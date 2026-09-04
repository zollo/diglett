package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/zollo/diglett/internal/lookup"
)

// resolversResponse is returned by GET /api/resolvers.
type resolversResponse struct {
	Resolvers []lookup.Resolver `json:"resolvers"`
	Defaults  []string          `json:"defaults"`
	Types     []string          `json:"types"`
}

func (s *Server) handleResolvers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, resolversResponse{
		Resolvers: s.svc.Resolvers(),
		Defaults:  s.svc.Defaults(),
		Types:     lookup.SupportedTypes(),
	})
}

func (s *Server) handleTypes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"types": lookup.SupportedTypes()})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": s.version, "name": "diglett"})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleLookup accepts both POST (JSON body) and GET (query parameters) so the
// API is easy to use both from the UI and from curl.
func (s *Server) handleLookup(w http.ResponseWriter, r *http.Request) {
	var req lookup.Request
	switch r.Method {
	case http.MethodPost:
		defer r.Body.Close()
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
			return
		}
	case http.MethodGet:
		req = requestFromQuery(r)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if err := s.validateRequest(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.requestBudget(len(req.Hostnames), len(req.Resolvers)))
	defer cancel()

	resp, err := s.svc.Do(ctx, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Allow a plain-text rendering for terminal-friendly consumption.
	if wantsText(r) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		for _, q := range resp.Queries {
			fmt.Fprintln(w, q.Raw)
		}
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// requestFromQuery builds a Request from URL query parameters.
//
// Supported params: name/host/q (repeatable or comma/newline separated),
// type, resolver/resolvers (repeatable or comma separated), reverse, trace,
// dnssec.
func requestFromQuery(r *http.Request) lookup.Request {
	q := r.URL.Query()
	var hostnames []string
	for _, key := range []string{"name", "host", "hostname", "q"} {
		for _, v := range q[key] {
			hostnames = append(hostnames, splitMulti(v)...)
		}
	}
	var resolvers []string
	for _, key := range []string{"resolver", "resolvers"} {
		for _, v := range q[key] {
			resolvers = append(resolvers, splitMulti(v)...)
		}
	}
	return lookup.Request{
		Hostnames: hostnames,
		Type:      q.Get("type"),
		Resolvers: resolvers,
		Reverse:   parseBoolParam(q.Get("reverse")),
		Trace:     parseBoolParam(q.Get("trace")),
		DNSSEC:    parseBoolParam(q.Get("dnssec")),
	}
}

func (s *Server) validateRequest(req *lookup.Request) error {
	// Normalise hostnames (support newline-delimited entries in a single field).
	var hosts []string
	for _, h := range req.Hostnames {
		hosts = append(hosts, splitMulti(h)...)
	}
	req.Hostnames = hosts

	if len(req.Hostnames) == 0 {
		return fmt.Errorf("at least one hostname is required")
	}
	if m := s.cfg.Query.MaxHostnames; m > 0 && len(req.Hostnames) > m {
		return fmt.Errorf("too many hostnames: %d (max %d)", len(req.Hostnames), m)
	}
	if m := s.cfg.Query.MaxResolvers; m > 0 && len(req.Resolvers) > m {
		return fmt.Errorf("too many resolvers: %d (max %d)", len(req.Resolvers), m)
	}
	if !s.cfg.Query.AllowCustomResolvers {
		known := map[string]bool{}
		for _, r := range s.svc.Resolvers() {
			known[r.ID] = true
		}
		for _, ref := range req.Resolvers {
			if !known[ref] {
				return fmt.Errorf("custom resolvers are disabled; unknown resolver %q", ref)
			}
		}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		log.Printf("diglett: failed to encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func wantsText(r *http.Request) bool {
	if parseBoolParam(r.URL.Query().Get("text")) {
		return true
	}
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "text/plain") && !strings.Contains(accept, "application/json")
}

func splitMulti(v string) []string {
	fields := strings.FieldsFunc(v, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t'
	})
	var out []string
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

func parseBoolParam(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// requestBudget derives an overall deadline that scales with the amount of
// work so large fan-outs are not cut off prematurely, while staying bounded.
func (s *Server) requestBudget(hosts, resolvers int) time.Duration {
	base := s.cfg.Query.Timeout
	if base <= 0 {
		base = 5 * time.Second
	}
	// The service runs exchanges concurrently, so the budget need only grow
	// modestly with the size of the fan-out.
	units := maxInt(hosts, 1) * maxInt(resolvers, 1)
	total := base + base/4*time.Duration(units)
	if total > 2*time.Minute {
		total = 2 * time.Minute
	}
	return total
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
