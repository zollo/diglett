// Package lookup performs DNS queries against one or more configurable
// resolvers and renders the results in both structured and dig-style forms.
package lookup

// Resolver describes a single upstream DNS server that Diglett can query.
type Resolver struct {
	// ID is a short, stable identifier used by the API and UI (e.g. "google").
	ID string `json:"id" yaml:"id"`
	// Name is a human friendly label (e.g. "Google Public DNS").
	Name string `json:"name" yaml:"name"`
	// Address is the server location. For udp/tcp/tls this is "host" or
	// "host:port"; for https (DoH) it is a full URL, e.g.
	// "https://cloudflare-dns.com/dns-query".
	Address string `json:"address" yaml:"address"`
	// Protocol is one of udp, tcp, tls (DoT) or https (DoH). Empty means udp.
	Protocol string `json:"protocol" yaml:"protocol"`
	// Group is an optional label used to visually group resolvers in the UI
	// (e.g. "Public", "Regional").
	Group string `json:"group,omitempty" yaml:"group,omitempty"`
}

// Record is a single resource record returned in a DNS response.
type Record struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Class string `json:"class"`
	TTL   uint32 `json:"ttl"`
	Data  string `json:"data"`
}

// Request is a single lookup requested by an API caller. One Request may fan
// out into many individual Query results (hostnames x resolvers x types).
type Request struct {
	// Hostnames to look up. Each entry is a domain name (or an IP address when
	// Reverse is true).
	Hostnames []string `json:"hostnames"`
	// Type is the record type to query (A, AAAA, MX, ...). Empty or "AUTO"
	// triggers automatic record discovery across a curated set of types.
	Type string `json:"type"`
	// Resolvers is a list of resolver IDs (from the configured set) and/or
	// literal resolver addresses (e.g. "8.8.8.8" or
	// "https://dns.google/dns-query"). Empty falls back to the configured
	// defaults.
	Resolvers []string `json:"resolvers"`
	// Reverse performs a reverse (PTR) lookup, treating each hostname as an IP.
	Reverse bool `json:"reverse"`
	// Trace performs an iterative resolution from the root servers, mirroring
	// "dig +trace".
	Trace bool `json:"trace"`
	// DNSSEC sets the DO bit and requests DNSSEC records.
	DNSSEC bool `json:"dnssec"`
}

// Query is the result of a single hostname/type/resolver combination.
type Query struct {
	Hostname    string   `json:"hostname"`
	Type        string   `json:"type"`
	Resolver    Resolver `json:"resolver"`
	Status      string   `json:"status"`
	Answers     []Record `json:"answers"`
	Authority   []Record `json:"authority,omitempty"`
	Additional  []Record `json:"additional,omitempty"`
	QueryTimeMs int64    `json:"query_time_ms"`
	Server      string   `json:"server"`
	Protocol    string   `json:"protocol"`
	// Trace, when present, holds the ordered delegation steps of a +trace query.
	Trace []TraceStep `json:"trace,omitempty"`
	// Raw is a human readable, dig-style rendering of this query.
	Raw string `json:"raw"`
	// Error is set when the query could not be completed at all.
	Error string `json:"error,omitempty"`
}

// TraceStep is one referral hop when tracing delegation from the root.
type TraceStep struct {
	Server   string   `json:"server"`
	Zone     string   `json:"zone"`
	Records  []Record `json:"records"`
	Duration int64    `json:"query_time_ms"`
}

// Response is the full result set for a Request.
type Response struct {
	Queries    []Query `json:"queries"`
	DurationMs int64   `json:"duration_ms"`
}
