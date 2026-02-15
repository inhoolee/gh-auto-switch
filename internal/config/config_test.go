package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_JSON(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "config.json")
	body := `{
  "rules": [
    { "prefix": "~/Documents", "user": "user-personal", "host": "github.com" },
    { "prefix": "~/Workspace", "user": "user-work", "host": "github.com" }
  ]
}`
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, used, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if used != p {
		t.Fatalf("expected used path %q, got %q", p, used)
	}
	if len(cfg.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %#v", cfg.Rules)
	}
	if cfg.Rules[1].User != "user-work" {
		t.Fatalf("expected user-work, got %#v", cfg.Rules[1])
	}
}

func TestLoad_YAML_Minimal(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "config.yaml")
	body := `rules:
  - prefix: ~/Documents
    user: user-personal
    host: github.com
  - prefix: ~/Workspace
    user: user-work
    host: github.com
`
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, used, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if used != p {
		t.Fatalf("expected used path %q, got %q", p, used)
	}
	if len(cfg.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %#v", cfg.Rules)
	}
	if cfg.Rules[0].Host != "github.com" {
		t.Fatalf("expected host github.com, got %#v", cfg.Rules[0])
	}
}

func TestLoad_TOML_Minimal(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "config.toml")
	body := `[[rules]]
prefix = "~/Documents"
user = "user-personal"
host = "github.com"

[[rules]]
prefix = "~/Workspace"
user = "user-work"
host = "github.com"
`
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, used, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if used != p {
		t.Fatalf("expected used path %q, got %q", p, used)
	}
	if len(cfg.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %#v", cfg.Rules)
	}
}
