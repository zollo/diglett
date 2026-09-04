package lookup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// exchanger performs a single DNS exchange against one resolver. It abstracts
// over the wire protocol (plain UDP/TCP, DNS-over-TLS and DNS-over-HTTPS).
type exchanger struct {
	httpClient *http.Client
	timeout    time.Duration
}

func newExchanger(timeout time.Duration) *exchanger {
	return &exchanger{
		timeout: timeout,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// exchange sends msg to the resolver and returns the reply along with the
// concrete server endpoint that answered and the elapsed time.
func (e *exchanger) exchange(ctx context.Context, r Resolver, msg *dns.Msg) (*dns.Msg, string, time.Duration, error) {
	proto := strings.ToLower(strings.TrimSpace(r.Protocol))
	if proto == "" {
		proto = "udp"
	}
	switch proto {
	case "https", "doh":
		return e.exchangeDoH(ctx, r, msg)
	case "tls", "dot":
		return e.exchangeClassic(ctx, msg, hostPort(r.Address, "853"), "tcp-tls")
	case "tcp":
		return e.exchangeClassic(ctx, msg, hostPort(r.Address, "53"), "tcp")
	case "udp":
		reply, server, rtt, err := e.exchangeClassic(ctx, msg, hostPort(r.Address, "53"), "udp")
		// Fall back to TCP on truncation, matching standard resolver behaviour.
		if err == nil && reply != nil && reply.Truncated {
			return e.exchangeClassic(ctx, msg, hostPort(r.Address, "53"), "tcp")
		}
		return reply, server, rtt, err
	default:
		return nil, "", 0, fmt.Errorf("unsupported protocol %q", r.Protocol)
	}
}

func (e *exchanger) exchangeClassic(ctx context.Context, msg *dns.Msg, addr, net string) (*dns.Msg, string, time.Duration, error) {
	c := &dns.Client{Net: net, Timeout: e.timeout}
	reply, rtt, err := c.ExchangeContext(ctx, msg, addr)
	if err != nil {
		return nil, addr, rtt, err
	}
	return reply, addr, rtt, nil
}

func (e *exchanger) exchangeDoH(ctx context.Context, r Resolver, msg *dns.Msg) (*dns.Msg, string, time.Duration, error) {
	url := r.Address
	if !strings.Contains(url, "://") {
		url = "https://" + url
	}
	packed, err := msg.Pack()
	if err != nil {
		return nil, url, 0, err
	}
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(packed))
	if err != nil {
		return nil, url, 0, err
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, url, time.Since(start), err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, url, time.Since(start), err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, url, time.Since(start), fmt.Errorf("DoH server returned HTTP %d", resp.StatusCode)
	}
	reply := new(dns.Msg)
	if err := reply.Unpack(body); err != nil {
		return nil, url, time.Since(start), fmt.Errorf("invalid DoH response: %w", err)
	}
	return reply, url, time.Since(start), nil
}

// hostPort ensures addr has a port, appending defaultPort when absent. It
// correctly handles bare IPv6 addresses.
func hostPort(addr, defaultPort string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return addr
	}
	// Already host:port?
	if _, _, err := net.SplitHostPort(addr); err == nil {
		return addr
	}
	// Bare IPv6 needs bracketing before we can attach a port.
	if strings.Count(addr, ":") >= 2 && !strings.HasPrefix(addr, "[") {
		return "[" + addr + "]:" + defaultPort
	}
	return net.JoinHostPort(addr, defaultPort)
}
