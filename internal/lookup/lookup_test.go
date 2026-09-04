package lookup

import (
	"context"
	"strings"
	"testing"

	"github.com/miekg/dns"
)

func TestReverseInvalidIPReturnsError(t *testing.T) {
	s := NewService(Options{Resolvers: []Resolver{{ID: "r", Name: "R", Address: "192.0.2.1", Protocol: "udp"}}})
	// A non-IP in reverse mode is entirely invalid, so no network query runs;
	// the caller must still receive the validation error rather than a lookup
	// of the raw string as an ordinary name.
	resp, err := s.Do(context.Background(), Request{Hostnames: []string{"not-an-ip"}, Reverse: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Queries) != 1 {
		t.Fatalf("expected 1 query, got %d", len(resp.Queries))
	}
	q := resp.Queries[0]
	if q.Error == "" || q.Type != "PTR" || q.Hostname != "not-an-ip" {
		t.Fatalf("expected PTR validation error for the raw input, got %+v", q)
	}
}

func TestHostPort(t *testing.T) {
	cases := []struct {
		addr, def, want string
	}{
		{"8.8.8.8", "53", "8.8.8.8:53"},
		{"8.8.8.8:5353", "53", "8.8.8.8:5353"},
		{"1.1.1.1", "853", "1.1.1.1:853"},
		{"2001:4860:4860::8888", "53", "[2001:4860:4860::8888]:53"},
		{"[2001:4860:4860::8888]:53", "53", "[2001:4860:4860::8888]:53"},
	}
	for _, c := range cases {
		if got := hostPort(c.addr, c.def); got != c.want {
			t.Errorf("hostPort(%q,%q)=%q want %q", c.addr, c.def, got, c.want)
		}
	}
}

func TestIsAuto(t *testing.T) {
	for _, s := range []string{"", "auto", "AUTO", " discover ", "Discover"} {
		if !IsAuto(s) {
			t.Errorf("IsAuto(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"A", "MX", "aaaa"} {
		if IsAuto(s) {
			t.Errorf("IsAuto(%q) = true, want false", s)
		}
	}
}

func TestNormalizeType(t *testing.T) {
	name, code, err := normalizeType("mx")
	if err != nil || name != "MX" || code != dns.TypeMX {
		t.Fatalf("normalizeType(mx) = %q,%d,%v", name, code, err)
	}
	if _, _, err := normalizeType("nope"); err == nil {
		t.Fatal("expected error for unknown type")
	}
	// Empty normalises to A.
	if name, _, _ := normalizeType(""); name != "A" {
		t.Fatalf("empty type normalised to %q, want A", name)
	}
}

func TestReverseName(t *testing.T) {
	got, err := reverseName("8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}
	if got != "8.8.8.8.in-addr.arpa" {
		t.Fatalf("reverseName = %q", got)
	}
	if _, err := reverseName("not-an-ip"); err == nil {
		t.Fatal("expected error for invalid IP")
	}
}

func TestRRToRecord(t *testing.T) {
	rr, err := dns.NewRR("example.com. 300 IN A 93.184.216.34")
	if err != nil {
		t.Fatal(err)
	}
	rec := rrToRecord(rr)
	if rec.Name != "example.com." || rec.Type != "A" || rec.TTL != 300 || rec.Data != "93.184.216.34" {
		t.Fatalf("unexpected record: %+v", rec)
	}

	mx, _ := dns.NewRR("example.com. 300 IN MX 10 mail.example.com.")
	rec = rrToRecord(mx)
	if rec.Type != "MX" || rec.Data != "10 mail.example.com." {
		t.Fatalf("unexpected MX record data: %q", rec.Data)
	}
}

func TestRRsToRecordsSkipsOPT(t *testing.T) {
	opt := new(dns.OPT)
	opt.Hdr.Name = "."
	opt.Hdr.Rrtype = dns.TypeOPT
	a, _ := dns.NewRR("example.com. 300 IN A 1.2.3.4")
	recs := rrsToRecords([]dns.RR{opt, a})
	if len(recs) != 1 || recs[0].Type != "A" {
		t.Fatalf("OPT not skipped: %+v", recs)
	}
}

func TestPruneEmptyDiscovery(t *testing.T) {
	in := []Query{
		{Hostname: "x", Server: "s", Type: "A", Status: "NOERROR", Answers: []Record{{Data: "1.2.3.4"}}},
		{Hostname: "x", Server: "s", Type: "AAAA", Status: "NOERROR"},               // empty -> pruned
		{Hostname: "x", Server: "s", Type: "MX", Status: "NOERROR"},                 // empty -> pruned
		{Hostname: "y", Server: "s", Type: "A", Status: "NOERROR"},                  // all empty for y
		{Hostname: "y", Server: "s", Type: "AAAA", Status: "NOERROR"},               // all empty for y
		{Hostname: "z", Server: "s", Type: "TXT", Status: "NOERROR", Error: "boom"}, // error kept
	}
	out := pruneEmptyDiscovery(in)

	byHost := map[string]int{}
	for _, q := range out {
		byHost[q.Hostname]++
	}
	if byHost["x"] != 1 {
		t.Errorf("host x: got %d results, want 1 (only the A record)", byHost["x"])
	}
	if byHost["y"] != 1 {
		t.Errorf("host y: got %d results, want 1 placeholder", byHost["y"])
	}
	if byHost["z"] != 1 {
		t.Errorf("host z: got %d results, want 1 (error kept)", byHost["z"])
	}
}

func TestRenderRaw(t *testing.T) {
	q := &Query{
		Hostname: "example.com", Type: "A", Server: "8.8.8.8:53", Protocol: "udp",
		Status: "NOERROR", QueryTimeMs: 12,
		Answers: []Record{{Name: "example.com.", TTL: 300, Class: "IN", Type: "A", Data: "1.2.3.4"}},
	}
	out := renderRaw(q)
	for _, want := range []string{"example.com", "NOERROR", "ANSWER SECTION", "1.2.3.4"} {
		if !strings.Contains(out, want) {
			t.Errorf("renderRaw output missing %q:\n%s", want, out)
		}
	}
}

func TestResolveResolver(t *testing.T) {
	s := NewService(Options{Resolvers: []Resolver{{ID: "google", Name: "Google", Address: "8.8.8.8", Protocol: "udp"}}})
	// Known ID.
	r, err := s.resolveResolver("google")
	if err != nil || r.Address != "8.8.8.8" {
		t.Fatalf("known resolver: %+v %v", r, err)
	}
	// Literal IP -> udp.
	r, _ = s.resolveResolver("1.2.3.4")
	if r.Protocol != "udp" || r.Address != "1.2.3.4" {
		t.Fatalf("literal IP: %+v", r)
	}
	// Ad-hoc DoH/URL resolvers are rejected (SSRF hardening): they must be
	// pre-configured and referenced by id.
	if _, err := s.resolveResolver("https://dns.example/dns-query"); err == nil {
		t.Fatal("expected error for ad-hoc URL resolver")
	}
	if _, err := s.resolveResolver("evil.example/path?x=1"); err == nil {
		t.Fatal("expected error for ad-hoc resolver containing a path")
	}
}
