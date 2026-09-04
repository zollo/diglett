package lookup

import (
	"fmt"
	"strings"

	"github.com/miekg/dns"
)

// supportedTypes lists the record types the API and UI expose explicitly.
var supportedTypes = []string{
	"A", "AAAA", "CNAME", "MX", "NS", "TXT", "SOA",
	"PTR", "SRV", "CAA", "NAPTR", "DS", "DNSKEY", "TLSA", "ANY",
}

// discoveryTypes is the curated set queried when the caller does not specify a
// record type ("auto discovery"). Only types that yield answers are shown.
var discoveryTypes = []string{
	"A", "AAAA", "CNAME", "MX", "NS", "TXT", "SOA", "CAA", "SRV",
}

// SupportedTypes returns the record types the tool can query directly.
func SupportedTypes() []string {
	out := make([]string, len(supportedTypes))
	copy(out, supportedTypes)
	return out
}

// IsAuto reports whether the given type string requests automatic discovery.
func IsAuto(t string) bool {
	t = strings.ToUpper(strings.TrimSpace(t))
	return t == "" || t == "AUTO" || t == "DISCOVER"
}

// normalizeType upper-cases and validates a requested record type, returning
// the dns package's numeric type code.
func normalizeType(t string) (string, uint16, error) {
	t = strings.ToUpper(strings.TrimSpace(t))
	if t == "" {
		t = "A"
	}
	code, ok := dns.StringToType[t]
	if !ok {
		return "", 0, fmt.Errorf("unknown record type %q", t)
	}
	return t, code, nil
}

// rrToRecord converts a miekg/dns resource record into our transport form.
func rrToRecord(rr dns.RR) Record {
	h := rr.Header()
	// The record's string form is "<header>\t<rdata>"; keep only the rdata.
	full := rr.String()
	data := full
	if idx := strings.Index(full, "\t"+dns.TypeToString[h.Rrtype]+"\t"); idx >= 0 {
		parts := strings.SplitN(full, "\t"+dns.TypeToString[h.Rrtype]+"\t", 2)
		if len(parts) == 2 {
			data = parts[1]
		}
	} else {
		// Fallback: strip the header prefix produced by rr.Header().String().
		data = strings.TrimPrefix(full, h.String())
	}
	return Record{
		Name:  h.Name,
		Type:  dns.TypeToString[h.Rrtype],
		Class: dns.ClassToString[h.Class],
		TTL:   h.Ttl,
		Data:  strings.TrimSpace(data),
	}
}

func rrsToRecords(rrs []dns.RR) []Record {
	if len(rrs) == 0 {
		return nil
	}
	out := make([]Record, 0, len(rrs))
	for _, rr := range rrs {
		// OPT pseudo-records (EDNS) are transport-level; skip them in output.
		if _, ok := rr.(*dns.OPT); ok {
			continue
		}
		out = append(out, rrToRecord(rr))
	}
	return out
}

// renderRaw produces a compact, dig-style textual rendering of a query result.
func renderRaw(q *Query) string {
	var b strings.Builder
	fmt.Fprintf(&b, ";; Query: %s  Type: %s\n", q.Hostname, q.Type)
	fmt.Fprintf(&b, ";; Server: %s (%s)\n", q.Server, strings.ToUpper(q.Protocol))
	if q.Error != "" {
		fmt.Fprintf(&b, ";; ERROR: %s\n", q.Error)
		return b.String()
	}
	fmt.Fprintf(&b, ";; Status: %s, Query time: %d ms\n", q.Status, q.QueryTimeMs)
	writeSection(&b, "ANSWER", q.Answers)
	writeSection(&b, "AUTHORITY", q.Authority)
	writeSection(&b, "ADDITIONAL", q.Additional)
	if len(q.Answers) == 0 && q.Error == "" && q.Status == "NOERROR" {
		b.WriteString(";; (no records of this type)\n")
	}
	return b.String()
}

func writeSection(b *strings.Builder, title string, recs []Record) {
	if len(recs) == 0 {
		return
	}
	fmt.Fprintf(b, ";; %s SECTION:\n", title)
	// Compute column widths for alignment.
	var nameW, ttlW int
	for _, r := range recs {
		if len(r.Name) > nameW {
			nameW = len(r.Name)
		}
		ttl := fmt.Sprintf("%d", r.TTL)
		if len(ttl) > ttlW {
			ttlW = len(ttl)
		}
	}
	for _, r := range recs {
		fmt.Fprintf(b, "%-*s  %*d  %-3s  %-6s  %s\n",
			nameW, r.Name, ttlW, r.TTL, r.Class, r.Type, r.Data)
	}
}
