package rules

import (
	"os"
	"path/filepath"
	"strings"
)

type Rule struct {
	Prefix string `json:"prefix" yaml:"prefix" toml:"prefix"`
	User   string `json:"user" yaml:"user" toml:"user"`
	Host   string `json:"host" yaml:"host" toml:"host"`
}

type MatchResult struct {
	Prefix string
	User   string
	Host   string
}

// Match returns the first rule whose expanded prefix matches cwd.
// Rules are evaluated in order.
func Match(cwd string, rules []Rule) *MatchResult {
	cwdVariants := pathVariants(cwd)
	for _, r := range rules {
		pfx := expandHome(r.Prefix)
		if pfx == "" {
			continue
		}
		pfxVariants := pathVariants(pfx)
		for _, cv := range cwdVariants {
			for _, pv := range pfxVariants {
				if hasPathPrefix(cv, pv) {
					return &MatchResult{
						Prefix: r.Prefix,
						User:   r.User,
						Host:   r.Host,
					}
				}
			}
		}
	}
	return nil
}

func expandHome(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "~"+string(os.PathSeparator)) || p == "~" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return p
		}
		if p == "~" {
			return home
		}
		return filepath.Join(home, p[2:])
	}
	return p
}

func hasPathPrefix(path, prefix string) bool {
	if path == prefix {
		return true
	}
	sep := string(os.PathSeparator)
	if !strings.HasSuffix(prefix, sep) {
		prefix = prefix + sep
	}
	return strings.HasPrefix(path+sep, prefix)
}

func pathVariants(p string) []string {
	clean := filepath.Clean(p)
	var out []string
	out = append(out, clean)
	if real, err := filepath.EvalSymlinks(clean); err == nil && real != "" && real != clean {
		out = append(out, filepath.Clean(real))
	}
	return out
}
