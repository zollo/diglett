// Package config loads Diglett configuration from an optional YAML file and
// environment variables. Environment variables always override file values.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/zollo/diglett/internal/lookup"
	"gopkg.in/yaml.v3"
)

// Config is the full runtime configuration.
type Config struct {
	Server    ServerConfig      `yaml:"server"`
	Query     QueryConfig       `yaml:"query"`
	Resolvers []lookup.Resolver `yaml:"resolvers"`
	// DefaultResolvers lists resolver IDs pre-selected in the UI and used when
	// an API request names none.
	DefaultResolvers []string `yaml:"default_resolvers"`
}

// ServerConfig controls the HTTP listener.
type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	// TrustedProxy, when true, honours X-Forwarded-For for client-IP logging.
	TrustedProxy bool `yaml:"trusted_proxy"`
}

// QueryConfig controls DNS query behaviour and limits.
type QueryConfig struct {
	// Timeout for a single DNS exchange.
	Timeout time.Duration `yaml:"timeout"`
	// Concurrency caps simultaneous in-flight exchanges per request.
	Concurrency int `yaml:"concurrency"`
	// MaxHostnames caps hostnames per request (0 = unlimited).
	MaxHostnames int `yaml:"max_hostnames"`
	// MaxResolvers caps resolvers per request (0 = unlimited).
	MaxResolvers int `yaml:"max_resolvers"`
	// AllowCustomResolvers permits API callers to query arbitrary resolver
	// addresses not in the configured set.
	AllowCustomResolvers bool `yaml:"allow_custom_resolvers"`
}

// Default returns a Config populated with sensible defaults and a curated set
// of well known public resolvers.
func Default() Config {
	return Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
		},
		Query: QueryConfig{
			Timeout:              5 * time.Second,
			Concurrency:          12,
			MaxHostnames:         100,
			MaxResolvers:         20,
			AllowCustomResolvers: true,
		},
		Resolvers:        DefaultResolvers(),
		DefaultResolvers: []string{"google", "cloudflare", "quad9"},
	}
}

// DefaultResolvers returns the built-in list of well known public resolvers.
func DefaultResolvers() []lookup.Resolver {
	return []lookup.Resolver{
		{ID: "google", Name: "Google", Address: "8.8.8.8", Protocol: "udp", Group: "Public"},
		{ID: "google-secondary", Name: "Google (secondary)", Address: "8.8.4.4", Protocol: "udp", Group: "Public"},
		{ID: "cloudflare", Name: "Cloudflare", Address: "1.1.1.1", Protocol: "udp", Group: "Public"},
		{ID: "cloudflare-secondary", Name: "Cloudflare (secondary)", Address: "1.0.0.1", Protocol: "udp", Group: "Public"},
		{ID: "quad9", Name: "Quad9", Address: "9.9.9.9", Protocol: "udp", Group: "Public"},
		{ID: "opendns", Name: "OpenDNS", Address: "208.67.222.222", Protocol: "udp", Group: "Public"},
		{ID: "adguard", Name: "AdGuard", Address: "94.140.14.14", Protocol: "udp", Group: "Public"},
		{ID: "quad9-doh", Name: "Quad9 (DoH)", Address: "https://dns.quad9.net/dns-query", Protocol: "https", Group: "Encrypted"},
		{ID: "google-doh", Name: "Google (DoH)", Address: "https://dns.google/dns-query", Protocol: "https", Group: "Encrypted"},
		{ID: "cloudflare-doh", Name: "Cloudflare (DoH)", Address: "https://cloudflare-dns.com/dns-query", Protocol: "https", Group: "Encrypted"},
	}
}

// Load builds a Config from defaults, then an optional YAML file, then
// environment overrides. path may be empty.
func Load(path string) (Config, error) {
	cfg := Default()

	if path == "" {
		path = os.Getenv("DIGLETT_CONFIG")
	}
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("reading config %s: %w", path, err)
		}
		// Unmarshal directly onto the defaults: yaml.v3 only overwrites keys
		// that are present in the document, so a partial file overrides just the
		// fields it names (including an explicit `allow_custom_resolvers: false`)
		// and leaves everything else at its default. A present `resolvers:` or
		// `default_resolvers:` list replaces the corresponding default slice.
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("parsing config %s: %w", path, err)
		}
	}

	applyEnv(&cfg)

	if err := cfg.validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// applyEnv overrides configuration from DIGLETT_* environment variables.
func applyEnv(cfg *Config) {
	if v := os.Getenv("DIGLETT_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv("DIGLETT_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = p
		}
	}
	if v := os.Getenv("DIGLETT_TRUSTED_PROXY"); v != "" {
		cfg.Server.TrustedProxy = parseBool(v)
	}
	if v := os.Getenv("DIGLETT_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Query.Timeout = d
		}
	}
	if v := os.Getenv("DIGLETT_CONCURRENCY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Query.Concurrency = n
		}
	}
	if v := os.Getenv("DIGLETT_MAX_HOSTNAMES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Query.MaxHostnames = n
		}
	}
	if v := os.Getenv("DIGLETT_MAX_RESOLVERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Query.MaxResolvers = n
		}
	}
	if v := os.Getenv("DIGLETT_ALLOW_CUSTOM_RESOLVERS"); v != "" {
		cfg.Query.AllowCustomResolvers = parseBool(v)
	}
	// DIGLETT_DEFAULT_RESOLVERS is a comma-separated list of resolver IDs.
	if v := os.Getenv("DIGLETT_DEFAULT_RESOLVERS"); v != "" {
		cfg.DefaultResolvers = splitList(v)
	}
	// DIGLETT_RESOLVERS provides inline resolver definitions in the compact form
	// "id|Name|address|protocol|group", entries separated by commas.
	if v := os.Getenv("DIGLETT_RESOLVERS"); v != "" {
		if rs := parseResolverList(v); len(rs) > 0 {
			cfg.Resolvers = rs
		}
	}
}

func (cfg Config) validate() error {
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return fmt.Errorf("invalid server port %d", cfg.Server.Port)
	}
	if len(cfg.Resolvers) == 0 {
		return fmt.Errorf("no resolvers configured")
	}
	ids := map[string]bool{}
	for _, r := range cfg.Resolvers {
		if r.ID == "" {
			return fmt.Errorf("resolver with empty id: %+v", r)
		}
		if r.Address == "" {
			return fmt.Errorf("resolver %q has empty address", r.ID)
		}
		if ids[r.ID] {
			return fmt.Errorf("duplicate resolver id %q", r.ID)
		}
		ids[r.ID] = true
	}
	// Drop any default resolver IDs that don't exist rather than erroring.
	var valid []string
	for _, id := range cfg.DefaultResolvers {
		if ids[id] {
			valid = append(valid, id)
		}
	}
	if len(valid) == 0 {
		// Fall back to the first configured resolver.
		valid = []string{cfg.Resolvers[0].ID}
	}
	return nil
}

// ValidDefaults returns default resolver IDs filtered to those that exist.
func (cfg Config) ValidDefaults() []string {
	ids := map[string]bool{}
	for _, r := range cfg.Resolvers {
		ids[r.ID] = true
	}
	var out []string
	for _, id := range cfg.DefaultResolvers {
		if ids[id] {
			out = append(out, id)
		}
	}
	if len(out) == 0 && len(cfg.Resolvers) > 0 {
		out = []string{cfg.Resolvers[0].ID}
	}
	return out
}

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func splitList(v string) []string {
	parts := strings.Split(v, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseResolverList(v string) []lookup.Resolver {
	var out []lookup.Resolver
	for _, entry := range strings.Split(v, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		fields := strings.Split(entry, "|")
		r := lookup.Resolver{}
		for i, f := range fields {
			f = strings.TrimSpace(f)
			switch i {
			case 0:
				r.ID = f
			case 1:
				r.Name = f
			case 2:
				r.Address = f
			case 3:
				r.Protocol = f
			case 4:
				r.Group = f
			}
		}
		if r.Name == "" {
			r.Name = r.ID
		}
		if r.ID != "" && r.Address != "" {
			out = append(out, r)
		}
	}
	return out
}
