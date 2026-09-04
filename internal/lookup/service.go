package lookup

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// Service resolves lookup requests against a known set of resolvers.
type Service struct {
	resolvers   []Resolver
	byID        map[string]Resolver
	defaults    []string
	timeout     time.Duration
	concurrency int
	ex          *exchanger
}

// Options configures a Service.
type Options struct {
	Resolvers []Resolver
	// Defaults are the resolver IDs used when a request names none.
	Defaults []string
	// Timeout bounds a single DNS exchange.
	Timeout time.Duration
	// Concurrency caps in-flight exchanges. Zero picks a sensible default.
	Concurrency int
}

// NewService builds a Service from the given options.
func NewService(opts Options) *Service {
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Second
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = 12
	}
	byID := make(map[string]Resolver, len(opts.Resolvers))
	for _, r := range opts.Resolvers {
		byID[r.ID] = r
	}
	defaults := opts.Defaults
	if len(defaults) == 0 {
		for _, r := range opts.Resolvers {
			defaults = append(defaults, r.ID)
		}
	}
	return &Service{
		resolvers:   opts.Resolvers,
		byID:        byID,
		defaults:    defaults,
		timeout:     opts.Timeout,
		concurrency: opts.Concurrency,
		ex:          newExchanger(opts.Timeout),
	}
}

// Resolvers returns the configured resolver set.
func (s *Service) Resolvers() []Resolver { return s.resolvers }

// Defaults returns the default resolver IDs.
func (s *Service) Defaults() []string { return s.defaults }

// resolveResolver turns a resolver reference (an ID or a literal address) into
// a concrete Resolver. Literal addresses default to UDP, or DoH when a URL.
func (s *Service) resolveResolver(ref string) (Resolver, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return Resolver{}, fmt.Errorf("empty resolver")
	}
	if r, ok := s.byID[ref]; ok {
		return r, nil
	}
	// Treat as a literal address. Ad-hoc custom resolvers are deliberately
	// limited to plain DNS servers (an IP or host, queried over UDP with TCP
	// fallback). Allowing an arbitrary DoH/HTTP(S) URL here would let an
	// unauthenticated caller make the server POST to any URL of their choosing
	// — a server-side request forgery (SSRF) primitive against internal
	// networks. DoH and DoT endpoints must therefore be defined in the
	// configured resolver set and referenced by id, never supplied ad hoc.
	if strings.Contains(ref, "://") || strings.ContainsAny(ref, "/?#") {
		return Resolver{}, fmt.Errorf("custom resolver %q is not allowed: ad-hoc resolvers must be a plain DNS server address (IP or host); DoH/DoT endpoints must be pre-configured and referenced by id", ref)
	}
	return Resolver{ID: ref, Name: ref, Address: ref, Protocol: "udp"}, nil
}

// job is one unit of work in the fan-out.
type job struct {
	hostname string
	rtype    string
	resolver Resolver
}

// Do executes a request, fanning out across hostnames, types and resolvers.
func (s *Service) Do(ctx context.Context, req Request) (*Response, error) {
	start := time.Now()

	// Resolve the resolver references.
	refs := req.Resolvers
	if len(refs) == 0 {
		refs = s.defaults
	}
	var resolvers []Resolver
	for _, ref := range refs {
		r, err := s.resolveResolver(ref)
		if err != nil {
			return nil, err
		}
		resolvers = append(resolvers, r)
	}
	if len(resolvers) == 0 {
		return nil, fmt.Errorf("no resolvers specified or configured")
	}

	// Clean up hostnames.
	var hosts []string
	for _, h := range req.Hostnames {
		h = strings.TrimSpace(h)
		if h != "" {
			hosts = append(hosts, h)
		}
	}
	if len(hosts) == 0 {
		return nil, fmt.Errorf("no hostnames provided")
	}

	// Determine which record types to query per hostname.
	var types []string
	auto := IsAuto(req.Type)
	switch {
	case req.Reverse:
		types = []string{"PTR"}
	case auto:
		types = append(types, discoveryTypes...)
	default:
		norm, _, err := normalizeType(req.Type)
		if err != nil {
			return nil, err
		}
		types = []string{norm}
	}

	// Build the job list.
	var jobs []job
	var invalid []Query
	for _, h := range hosts {
		qhost := h
		if req.Reverse {
			arpa, err := reverseName(h)
			if err != nil {
				// Surface the validation error to the caller rather than
				// silently querying the invalid input as an ordinary name.
				for _, r := range resolvers {
					q := Query{
						Hostname: h,
						Type:     "PTR",
						Resolver: r,
						Protocol: protoOrDefault(r.Protocol),
						Server:   r.Address,
						Error:    err.Error(),
					}
					q.Raw = renderRaw(&q)
					invalid = append(invalid, q)
				}
				continue
			}
			qhost = arpa
		}
		for _, r := range resolvers {
			for _, t := range types {
				jobs = append(jobs, job{hostname: qhost, rtype: t, resolver: r})
			}
		}
	}

	results := make([]Query, len(jobs))
	sem := make(chan struct{}, s.concurrency)
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, j job) {
			defer wg.Done()
			defer func() { <-sem }()
			var q Query
			if req.Trace {
				q = s.trace(ctx, j.hostname, j.rtype, j.resolver, req.DNSSEC)
			} else {
				q = s.one(ctx, j.hostname, j.rtype, j.resolver, req.DNSSEC)
			}
			results[i] = q
		}(i, j)
	}
	wg.Wait()

	// In auto-discovery mode, drop empty NOERROR results so only record types
	// that actually exist are shown — but always keep at least one entry per
	// hostname/resolver so callers see that the lookup ran.
	if auto && !req.Trace {
		results = pruneEmptyDiscovery(results)
	}

	// Include any validation errors (e.g. invalid IPs in reverse mode).
	results = append(results, invalid...)

	return &Response{
		Queries:    results,
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}

// one performs a single query against one resolver.
func (s *Service) one(ctx context.Context, hostname, rtype string, r Resolver, dnssec bool) Query {
	q := Query{
		Hostname: hostname,
		Type:     rtype,
		Resolver: r,
		Protocol: protoOrDefault(r.Protocol),
		Server:   r.Address,
	}
	_, code, err := normalizeType(rtype)
	if err != nil {
		q.Error = err.Error()
		q.Raw = renderRaw(&q)
		return q
	}

	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(hostname), code)
	m.RecursionDesired = true
	if dnssec {
		m.SetEdns0(4096, true)
	}

	reply, server, rtt, err := s.ex.exchange(ctx, r, m)
	q.Server = server
	q.QueryTimeMs = rtt.Milliseconds()
	if err != nil {
		q.Error = err.Error()
		q.Raw = renderRaw(&q)
		return q
	}
	q.Status = dns.RcodeToString[reply.Rcode]
	q.Answers = rrsToRecords(reply.Answer)
	q.Authority = rrsToRecords(reply.Ns)
	q.Additional = rrsToRecords(reply.Extra)
	q.Raw = renderRaw(&q)
	return q
}

// pruneEmptyDiscovery removes empty NOERROR results in auto mode, retaining a
// placeholder per hostname/resolver group when a group is entirely empty.
func pruneEmptyDiscovery(in []Query) []Query {
	type key struct{ host, server string }
	group := map[key][]Query{}
	var order []key
	for _, q := range in {
		k := key{q.Hostname, q.Server}
		if _, ok := group[k]; !ok {
			order = append(order, k)
		}
		group[k] = append(group[k], q)
	}
	var out []Query
	for _, k := range order {
		var kept []Query
		for _, q := range group[k] {
			if q.Error == "" && len(q.Answers) == 0 && q.Status == "NOERROR" {
				continue
			}
			kept = append(kept, q)
		}
		if len(kept) == 0 {
			// Keep the SOA/first entry so the caller sees the negative result.
			kept = group[k][:1]
		}
		out = append(out, kept...)
	}
	return out
}

func protoOrDefault(p string) string {
	if strings.TrimSpace(p) == "" {
		return "udp"
	}
	return strings.ToLower(p)
}

// reverseName converts an IP address into its reverse-lookup name.
func reverseName(ip string) (string, error) {
	arpa, err := dns.ReverseAddr(strings.TrimSpace(ip))
	if err != nil {
		return "", fmt.Errorf("%q is not a valid IP address for reverse lookup", ip)
	}
	return strings.TrimSuffix(arpa, "."), nil
}
