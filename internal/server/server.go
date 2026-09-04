// Package server exposes Diglett's HTTP API and embedded web UI.
package server

import (
	"context"
	"io/fs"
	"net/http"
	"time"

	"github.com/zollo/diglett/internal/config"
	"github.com/zollo/diglett/internal/lookup"
)

// Server wires the DNS lookup service to HTTP handlers.
type Server struct {
	cfg     config.Config
	svc     *lookup.Service
	static  fs.FS
	version string
}

// New constructs a Server from configuration, a lookup service and the static
// asset filesystem for the web UI.
func New(cfg config.Config, svc *lookup.Service, static fs.FS, version string) *Server {
	return &Server{cfg: cfg, svc: svc, static: static, version: version}
}

// Handler returns the root HTTP handler with all routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/lookup", s.handleLookup)
	mux.HandleFunc("/api/resolvers", s.handleResolvers)
	mux.HandleFunc("/api/types", s.handleTypes)
	mux.HandleFunc("/api/version", s.handleVersion)
	mux.HandleFunc("/healthz", s.handleHealth)

	// Serve the embedded web UI at the root.
	fileServer := http.FileServer(http.FS(s.static))
	mux.Handle("/", fileServer)

	return logRequests(withCORS(mux))
}

// ListenAndServe starts the HTTP server and blocks until ctx is cancelled or an
// error occurs.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
