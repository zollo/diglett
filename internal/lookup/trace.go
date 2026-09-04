package lookup

import (
	"context"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// rootServers are the IANA root name servers used as the starting point for a
// trace. Any reachable one is fine; we try them in order.
var rootServers = []string{
	"198.41.0.4",     // a.root-servers.net
	"170.247.170.2",  // b.root-servers.net
	"192.33.4.12",    // c.root-servers.net
	"199.7.91.13",    // d.root-servers.net
	"192.203.230.10", // e.root-servers.net
	"192.5.5.241",    // f.root-servers.net
	"192.112.36.4",   // g.root-servers.net
	"198.97.190.53",  // h.root-servers.net
	"192.36.148.17",  // i.root-servers.net
	"192.58.128.30",  // j.root-servers.net
	"193.0.14.129",   // k.root-servers.net
	"199.7.83.42",    // l.root-servers.net
	"202.12.27.33",   // m.root-servers.net
}

const maxTraceHops = 20

// trace performs an iterative resolution from the root servers, recording each
// referral as a TraceStep, mirroring "dig +trace".
func (s *Service) trace(ctx context.Context, hostname, rtype string, seed Resolver, dnssec bool) Query {
	q := Query{
		Hostname: hostname,
		Type:     rtype,
		Resolver: Resolver{ID: "trace", Name: "Iterative trace from root", Protocol: "udp"},
		Protocol: "udp",
		Server:   "root",
	}
	_, code, err := normalizeType(rtype)
	if err != nil {
		q.Error = err.Error()
		q.Raw = renderRaw(&q)
		return q
	}

	name := dns.Fqdn(hostname)
	// Start from a root server.
	nextServers := append([]string{}, rootServers...)
	zone := "."

	for hop := 0; hop < maxTraceHops; hop++ {
		reply, server, rtt, err := s.traceQuery(ctx, nextServers, name, code, dnssec)
		if err != nil {
			q.Error = "trace failed at " + zone + ": " + err.Error()
			break
		}
		step := TraceStep{
			Server:   server,
			Zone:     zone,
			Duration: rtt.Milliseconds(),
		}
		// Accumulate total elapsed time across hops so the result carries a
		// meaningful QueryTimeMs regardless of which branch returns below.
		q.QueryTimeMs += step.Duration

		// If we received answers, we've reached the authoritative data.
		if len(reply.Answer) > 0 {
			step.Records = rrsToRecords(reply.Answer)
			q.Trace = append(q.Trace, step)
			q.Status = dns.RcodeToString[reply.Rcode]
			q.Answers = rrsToRecords(reply.Answer)
			q.Authority = rrsToRecords(reply.Ns)
			q.Server = server
			q.Raw = renderTrace(&q)
			return q
		}

		// Otherwise follow the delegation (NS records in the authority section).
		nsSet := extractNS(reply.Ns)
		if len(nsSet) == 0 {
			// No delegation and no answer: negative/authoritative response.
			step.Records = rrsToRecords(reply.Ns)
			q.Trace = append(q.Trace, step)
			q.Status = dns.RcodeToString[reply.Rcode]
			q.Authority = rrsToRecords(reply.Ns)
			q.Server = server
			q.Raw = renderTrace(&q)
			return q
		}
		step.Records = rrsToRecords(reply.Ns)
		q.Trace = append(q.Trace, step)

		// Resolve the next hop's server addresses, preferring glue.
		glue := extractGlue(reply.Extra)
		var servers []string
		for _, ns := range nsSet {
			if addrs, ok := glue[strings.ToLower(ns.Ns)]; ok {
				servers = append(servers, addrs...)
			}
		}
		if len(servers) == 0 {
			// No glue: resolve one NS name using the seed resolver.
			for _, ns := range nsSet {
				if ip := s.resolveHostA(ctx, ns.Ns, seed); ip != "" {
					servers = append(servers, ip)
					break
				}
			}
		}
		if len(servers) == 0 {
			q.Error = "trace stalled: could not resolve name servers for " + nsSet[0].Header().Name
			break
		}
		nextServers = servers
		zone = nsSet[0].Header().Name
	}

	if q.Raw == "" {
		q.Raw = renderTrace(&q)
	}
	return q
}

// traceQuery sends a non-recursive query to the first server that answers.
func (s *Service) traceQuery(ctx context.Context, servers []string, name string, code uint16, dnssec bool) (*dns.Msg, string, time.Duration, error) {
	m := new(dns.Msg)
	m.SetQuestion(name, code)
	m.RecursionDesired = false
	if dnssec {
		m.SetEdns0(4096, true)
	}
	var lastErr error
	for _, srv := range servers {
		r := Resolver{Address: srv, Protocol: "udp"}
		reply, server, rtt, err := s.ex.exchange(ctx, r, m)
		if err == nil && reply != nil {
			return reply, server, rtt, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = context.DeadlineExceeded
	}
	return nil, "", 0, lastErr
}

func (s *Service) resolveHostA(ctx context.Context, host string, seed Resolver) string {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(host), dns.TypeA)
	m.RecursionDesired = true
	reply, _, _, err := s.ex.exchange(ctx, seed, m)
	if err != nil || reply == nil {
		return ""
	}
	for _, rr := range reply.Answer {
		if a, ok := rr.(*dns.A); ok {
			return a.A.String()
		}
	}
	return ""
}

func extractNS(rrs []dns.RR) []*dns.NS {
	var out []*dns.NS
	for _, rr := range rrs {
		if ns, ok := rr.(*dns.NS); ok {
			out = append(out, ns)
		}
	}
	return out
}

// extractGlue maps NS hostnames to their A/AAAA glue addresses.
func extractGlue(rrs []dns.RR) map[string][]string {
	glue := map[string][]string{}
	for _, rr := range rrs {
		switch v := rr.(type) {
		case *dns.A:
			k := strings.ToLower(v.Hdr.Name)
			glue[k] = append(glue[k], v.A.String())
		case *dns.AAAA:
			k := strings.ToLower(v.Hdr.Name)
			glue[k] = append(glue[k], v.AAAA.String())
		}
	}
	return glue
}

func renderTrace(q *Query) string {
	var b strings.Builder
	b.WriteString(";; Trace: " + q.Hostname + "  Type: " + q.Type + "\n")
	for _, step := range q.Trace {
		b.WriteString(";; " + step.Zone + " -> " + step.Server + " (" +
			itoa(step.Duration) + " ms)\n")
		for _, r := range step.Records {
			b.WriteString("  " + r.Name + "  " + r.Type + "  " + r.Data + "\n")
		}
	}
	if q.Error != "" {
		b.WriteString(";; ERROR: " + q.Error + "\n")
	}
	return b.String()
}

func itoa(n int64) string {
	// Small helper to avoid importing strconv just for traces.
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
