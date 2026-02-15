package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/inhoolee/gh-auto-switch/internal/rules"
)

type Config struct {
	Rules []rules.Rule `json:"rules" yaml:"rules" toml:"rules"`
}

const defaultHost = "github.com"

func (c *Config) Normalize() {
	for i := range c.Rules {
		if strings.TrimSpace(c.Rules[i].Host) == "" {
			c.Rules[i].Host = defaultHost
		}
	}
}

func (c Config) Validate() error {
	if len(c.Rules) == 0 {
		return errors.New("no rules configured")
	}
	for i, r := range c.Rules {
		if strings.TrimSpace(r.Prefix) == "" {
			return fmt.Errorf("rules[%d]: prefix is required", i)
		}
		if strings.TrimSpace(r.User) == "" {
			return fmt.Errorf("rules[%d]: user is required", i)
		}
	}
	return nil
}

var ErrConfigNotFound = errors.New("config not found")

type NotFoundError struct {
	Tried []string
}

func (e NotFoundError) Error() string {
	if len(e.Tried) == 0 {
		return "config not found"
	}
	return "config not found (tried: " + strings.Join(e.Tried, ", ") + ")"
}

func (e NotFoundError) Unwrap() error { return ErrConfigNotFound }

func Load(explicitPath string) (cfg Config, usedPath string, err error) {
	if explicitPath == "" {
		if env := strings.TrimSpace(os.Getenv("GH_AUTO_SWITCH_CONFIG")); env != "" {
			explicitPath = env
		}
	}

	paths := []string{}
	if explicitPath != "" {
		paths = append(paths, explicitPath)
	} else {
		// Prefer XDG config dir when available.
		if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
			paths = append(paths,
				filepath.Join(xdg, "gh-auto-switch", "config.yaml"),
				filepath.Join(xdg, "gh-auto-switch", "config.yml"),
				filepath.Join(xdg, "gh-auto-switch", "config.json"),
				filepath.Join(xdg, "gh-auto-switch", "config.toml"),
			)
		}
		if home, e := os.UserHomeDir(); e == nil && home != "" {
			paths = append(paths,
				filepath.Join(home, ".config", "gh-auto-switch", "config.yaml"),
				filepath.Join(home, ".config", "gh-auto-switch", "config.yml"),
				filepath.Join(home, ".config", "gh-auto-switch", "config.json"),
				filepath.Join(home, ".config", "gh-auto-switch", "config.toml"),
			)
			paths = append(paths,
				filepath.Join(home, ".gh-auto-switch.yaml"),
				filepath.Join(home, ".gh-auto-switch.yml"),
				filepath.Join(home, ".gh-auto-switch.json"),
				filepath.Join(home, ".gh-auto-switch.toml"),
			)
		}
	}

	for _, p := range paths {
		b, readErr := os.ReadFile(p)
		if readErr != nil {
			if explicitPath != "" {
				return Config{}, p, fmt.Errorf("read config: %w", readErr)
			}
			continue
		}
		c, parseErr := parseConfig(p, b)
		if parseErr != nil {
			return Config{}, p, parseErr
		}
		c.Normalize()
		return c, p, nil
	}

	return Config{}, "", NotFoundError{Tried: paths}
}

func parseConfig(path string, b []byte) (Config, error) {
	ext := strings.ToLower(filepath.Ext(path))
	var cfg Config
	switch ext {
	case ".yaml", ".yml":
		c, err := parseYAML(b)
		if err != nil {
			return Config{}, fmt.Errorf("parse yaml: %w", err)
		}
		cfg = c
	case ".json":
		if err := json.Unmarshal(b, &cfg); err != nil {
			return Config{}, fmt.Errorf("parse json: %w", err)
		}
	case ".toml":
		c, err := parseTOML(b)
		if err != nil {
			return Config{}, fmt.Errorf("parse toml: %w", err)
		}
		cfg = c
	default:
		// Best-effort: try yaml then json then toml.
		if c, err := parseYAML(b); err == nil {
			return c, nil
		}
		if err := json.Unmarshal(b, &cfg); err == nil {
			return cfg, nil
		}
		if c, err := parseTOML(b); err == nil {
			return c, nil
		}
		return Config{}, fmt.Errorf("unsupported config extension: %s", ext)
	}
	return cfg, nil
}

// parseYAML parses a minimal subset of YAML required for this tool:
//
// rules:
//   - prefix: ~/Documents
//     user: user-personal
//     host: github.com
//
// It intentionally does not support anchors, complex types, multi-line strings, etc.
func parseYAML(b []byte) (Config, error) {
	lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	var (
		inRules bool
		cur     *rules.Rule
		out     Config
	)

	for i, raw := range lines {
		line := stripYAMLComment(raw)
		if strings.TrimSpace(line) == "" {
			continue
		}
		trim := strings.TrimSpace(line)

		if !inRules {
			if trim == "rules:" {
				inRules = true
			}
			continue
		}

		if strings.HasPrefix(trim, "-") {
			// Start a new rule.
			r := rules.Rule{}
			out.Rules = append(out.Rules, r)
			cur = &out.Rules[len(out.Rules)-1]

			rest := strings.TrimSpace(strings.TrimPrefix(trim, "-"))
			if rest == "" {
				continue
			}
			// Support "- key: value" on the same line.
			k, v, ok := splitKV(rest)
			if !ok {
				return Config{}, fmt.Errorf("line %d: expected `- key: value` or `-`", i+1)
			}
			assignRuleField(cur, k, v)
			continue
		}

		if cur == nil {
			return Config{}, fmt.Errorf("line %d: expected list item under rules", i+1)
		}
		k, v, ok := splitKV(trim)
		if !ok {
			return Config{}, fmt.Errorf("line %d: expected `key: value`", i+1)
		}
		assignRuleField(cur, k, v)
	}
	return out, nil
}

func stripYAMLComment(line string) string {
	// This is a minimal implementation: it removes '#' comments unless inside quotes
	// (we only support single-line simple scalar values).
	inSingle := false
	inDouble := false
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble {
				return line[:i]
			}
		}
	}
	return line
}

func splitKV(s string) (k, v string, ok bool) {
	idx := strings.IndexByte(s, ':')
	if idx < 0 {
		return "", "", false
	}
	k = strings.TrimSpace(s[:idx])
	v = strings.TrimSpace(s[idx+1:])
	v = trimQuotes(v)
	return k, v, true
}

func trimQuotes(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func assignRuleField(r *rules.Rule, k, v string) {
	switch strings.ToLower(k) {
	case "prefix":
		r.Prefix = v
	case "user":
		r.User = v
	case "host":
		r.Host = v
	}
}

// parseTOML parses a minimal TOML subset:
//
// [[rules]]
// prefix = "~/Documents"
// user = "user-personal"
// host = "github.com"
func parseTOML(b []byte) (Config, error) {
	lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	var (
		cur *rules.Rule
		out Config
	)

	reKV := regexp.MustCompile(`^([A-Za-z0-9_]+)\s*=\s*(.+)\s*$`)
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		if line == "[[rules]]" {
			out.Rules = append(out.Rules, rules.Rule{})
			cur = &out.Rules[len(out.Rules)-1]
			continue
		}
		if cur == nil {
			continue
		}
		m := reKV.FindStringSubmatch(line)
		if len(m) != 3 {
			return Config{}, fmt.Errorf("line %d: expected `key = value`", i+1)
		}
		k := m[1]
		v := strings.TrimSpace(m[2])
		// Minimal string handling: strip quotes if present.
		v = trimQuotes(v)
		assignRuleField(cur, k, v)
	}
	return out, nil
}
