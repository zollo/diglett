package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/zollo/diglett/internal/config"
	"github.com/zollo/diglett/internal/lookup"
)

func testServer() *Server {
	cfg := config.Default()
	cfg.Query.MaxHostnames = 3
	svc := lookup.NewService(lookup.Options{
		Resolvers: cfg.Resolvers,
		Defaults:  cfg.ValidDefaults(),
	})
	static := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html>diglett</html>")}}
	return New(cfg, svc, static, "test")
}

func TestHealthAndResolvers(t *testing.T) {
	h := testServer().Handler()

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "ok") {
		t.Fatalf("healthz: %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/resolvers", nil))
	var resp resolversResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Resolvers) == 0 || len(resp.Types) == 0 || len(resp.Defaults) == 0 {
		t.Fatalf("resolvers response incomplete: %+v", resp)
	}
}

func TestStaticIndex(t *testing.T) {
	rr := httptest.NewRecorder()
	testServer().Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "diglett") {
		t.Fatalf("index: %d %s", rr.Code, rr.Body.String())
	}
}

func TestLookupValidation(t *testing.T) {
	h := testServer().Handler()

	// No hostnames -> 400.
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/lookup", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty request, got %d", rr.Code)
	}

	// Too many hostnames -> 400 (MaxHostnames = 3).
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/lookup?name=a.com,b.com,c.com,d.com", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for too many hostnames, got %d", rr.Code)
	}

	// Bad JSON -> 400.
	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/lookup", strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad JSON, got %d", rr.Code)
	}

	// Unsupported method -> 405.
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "/api/lookup", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func TestRequestFromQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/lookup?name=a.com,b.com&type=MX&resolvers=google,cloudflare&trace=1", nil)
	r := requestFromQuery(req)
	if len(r.Hostnames) != 2 || r.Type != "MX" || len(r.Resolvers) != 2 || !r.Trace {
		t.Fatalf("unexpected parsed request: %+v", r)
	}
}

func TestCORSPreflight(t *testing.T) {
	rr := httptest.NewRecorder()
	testServer().Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodOptions, "/api/lookup", nil))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for preflight, got %d", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("missing CORS header")
	}
}

func TestSplitMulti(t *testing.T) {
	got := splitMulti("a.com, b.com\nc.com\td.com")
	if len(got) != 4 {
		t.Fatalf("splitMulti = %v", got)
	}
}
