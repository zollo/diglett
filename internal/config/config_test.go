package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultValid(t *testing.T) {
	cfg := Default()
	if err := cfg.validate(); err != nil {
		t.Fatalf("default config invalid: %v", err)
	}
	if len(cfg.ValidDefaults()) == 0 {
		t.Fatal("expected default resolvers")
	}
}

func TestLoadFileOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
server:
  host: 127.0.0.1
  port: 9000
query:
  timeout: 2s
  max_hostnames: 5
resolvers:
  - id: local
    name: Local
    address: 127.0.0.1
    protocol: udp
default_resolvers:
  - local
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Host != "127.0.0.1" || cfg.Server.Port != 9000 {
		t.Errorf("server override failed: %+v", cfg.Server)
	}
	if cfg.Query.Timeout != 2*time.Second || cfg.Query.MaxHostnames != 5 {
		t.Errorf("query override failed: %+v", cfg.Query)
	}
	if len(cfg.Resolvers) != 1 || cfg.Resolvers[0].ID != "local" {
		t.Errorf("resolver override failed: %+v", cfg.Resolvers)
	}
}

func TestFileCanDisableCustomResolvers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	// AllowCustomResolvers defaults to true; a file must be able to turn it off.
	yaml := "query:\n  allow_custom_resolvers: false\n"
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Query.AllowCustomResolvers {
		t.Fatal("expected allow_custom_resolvers=false from file to disable custom resolvers")
	}
	// A field the file did not mention keeps its default.
	if cfg.Query.MaxHostnames != Default().Query.MaxHostnames {
		t.Fatalf("unmentioned field changed: MaxHostnames=%d", cfg.Query.MaxHostnames)
	}
}

func TestEnvOverrides(t *testing.T) {
	t.Setenv("DIGLETT_PORT", "7777")
	t.Setenv("DIGLETT_HOST", "1.2.3.4")
	t.Setenv("DIGLETT_TIMEOUT", "3s")
	t.Setenv("DIGLETT_DEFAULT_RESOLVERS", "google,quad9")
	t.Setenv("DIGLETT_RESOLVERS", "a|Alpha|8.8.8.8|udp|G, b|Beta|https://x/dns-query|https|E")

	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 7777 || cfg.Server.Host != "1.2.3.4" {
		t.Errorf("env server override failed: %+v", cfg.Server)
	}
	if cfg.Query.Timeout != 3*time.Second {
		t.Errorf("env timeout failed: %v", cfg.Query.Timeout)
	}
	if len(cfg.Resolvers) != 2 || cfg.Resolvers[0].ID != "a" || cfg.Resolvers[1].Protocol != "https" {
		t.Errorf("env resolver list failed: %+v", cfg.Resolvers)
	}
	// default_resolvers references google/quad9, but those aren't in the custom
	// list, so ValidDefaults falls back to the first configured resolver.
	vd := cfg.ValidDefaults()
	if len(vd) != 1 || vd[0] != "a" {
		t.Errorf("ValidDefaults fallback failed: %v", vd)
	}
}

func TestInvalidPort(t *testing.T) {
	cfg := Default()
	cfg.Server.Port = 0
	if err := cfg.validate(); err == nil {
		t.Fatal("expected error for invalid port")
	}
}

func TestDuplicateResolverID(t *testing.T) {
	cfg := Default()
	cfg.Resolvers = append(cfg.Resolvers, cfg.Resolvers[0])
	if err := cfg.validate(); err == nil {
		t.Fatal("expected error for duplicate resolver id")
	}
}
