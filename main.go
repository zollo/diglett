// Command diglett is a free, open-source web DNS lookup tool and API, similar
// in spirit to digwebinterface.com. It serves an embedded web UI backed by an
// unauthenticated JSON API for querying multiple hostnames and record types
// across well known public DNS resolvers.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/zollo/diglett/internal/config"
	"github.com/zollo/diglett/internal/lookup"
	"github.com/zollo/diglett/internal/server"
	"github.com/zollo/diglett/web"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	var (
		configPath  = flag.String("config", "", "path to a YAML config file (or set DIGLETT_CONFIG)")
		showVersion = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println("diglett", version)
		return
	}

	log.SetFlags(log.LstdFlags | log.LUTC)

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("diglett: configuration error: %v", err)
	}

	svc := lookup.NewService(lookup.Options{
		Resolvers:   cfg.Resolvers,
		Defaults:    cfg.ValidDefaults(),
		Timeout:     cfg.Query.Timeout,
		Concurrency: cfg.Query.Concurrency,
	})

	srv := server.New(cfg, svc, web.Static(), version)

	addr := net.JoinHostPort(cfg.Server.Host, strconv.Itoa(cfg.Server.Port))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("diglett %s listening on http://%s (%d resolvers configured)", version, addr, len(cfg.Resolvers))
	if err := srv.ListenAndServe(ctx, addr); err != nil {
		log.Fatalf("diglett: server error: %v", err)
	}
	log.Println("diglett: shut down cleanly")
}
