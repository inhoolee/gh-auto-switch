package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/inhoolee/gh-auto-switch/internal/config"
	"github.com/inhoolee/gh-auto-switch/internal/gh"
	"github.com/inhoolee/gh-auto-switch/internal/rules"
)

const (
	exitOK      = 0
	exitUsage   = 2
	exitFailure = 1
)

type globalFlags struct {
	ConfigPath string
	Verbose    bool
	Quiet      bool
}

func Run(args []string) int {
	if len(args) == 0 {
		printRootUsage(os.Stderr)
		return exitUsage
	}

	var gf globalFlags
	fs := flag.NewFlagSet("gh-auto-switch", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&gf.ConfigPath, "config", "", "config file path (optional)")
	fs.BoolVar(&gf.Verbose, "v", false, "verbose output")
	fs.BoolVar(&gf.Quiet, "q", false, "quiet output (errors only)")

	// Parse global flags until first non-flag, then dispatch.
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	rest := fs.Args()
	if len(rest) == 0 {
		printRootUsage(os.Stderr)
		return exitUsage
	}

	cmd := rest[0]
	cmdArgs := rest[1:]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	switch cmd {
	case "status":
		return runStatus(ctx, gf, cmdArgs)
	case "switch":
		return runSwitch(ctx, gf, cmdArgs)
	case "doctor":
		return runDoctor(ctx, gf, cmdArgs)
	case "config":
		return runConfig(gf, cmdArgs)
	case "decide":
		return runDecide(ctx, gf, cmdArgs)
	case "-h", "--help", "help":
		printRootUsage(os.Stdout)
		return exitOK
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		printRootUsage(os.Stderr)
		return exitUsage
	}
}

func printRootUsage(w *os.File) {
	fmt.Fprintln(w, "gh-auto-switch: auto-switch gh auth context based on cwd")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  gh-auto-switch [--config path] [-v] [-q] <command> [args]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  status   Show rule match and current gh auth state")
	fmt.Fprintln(w, "  switch   Switch gh auth user for the matched rule (idempotent)")
	fmt.Fprintln(w, "  doctor   Check prerequisites and logged-in accounts")
	fmt.Fprintln(w, "  config   Manage config (path/show/init)")
	fmt.Fprintln(w, "  decide   Print the desired auth key for cwd (used for shell hooks)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Exit codes:")
	fmt.Fprintln(w, "  0 success")
	fmt.Fprintln(w, "  1 failure")
	fmt.Fprintln(w, "  2 usage/config error")
}

func loadConfigRequired(cfgPath string) (config.Config, string, error) {
	cfg, usedPath, err := config.Load(cfgPath)
	if err != nil {
		return config.Config{}, usedPath, err
	}
	if err := cfg.Validate(); err != nil {
		return config.Config{}, usedPath, err
	}
	return cfg, usedPath, nil
}

func loadConfigOptional(cfgPath string) (config.Config, string, error) {
	cfg, usedPath, err := config.Load(cfgPath)
	if err != nil {
		return config.Config{}, usedPath, err
	}
	if err := cfg.Validate(); err != nil {
		return config.Config{}, usedPath, err
	}
	return cfg, usedPath, nil
}

func runDecide(_ context.Context, gf globalFlags, args []string) int {
	var wantKey bool
	fs := flag.NewFlagSet("decide", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.BoolVar(&wantKey, "key", true, "print key as host|user (default true)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	cfg, _, err := loadConfigOptional(gf.ConfigPath)
	if err != nil {
		if errors.Is(err, config.ErrConfigNotFound) {
			// For shell hooks: unconfigured should be a no-op.
			return exitOK
		}
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		return exitUsage
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "getwd: %v\n", err)
		return exitFailure
	}
	cwd, _ = filepath.Abs(cwd)

	match := rules.Match(cwd, cfg.Rules)
	if match == nil {
		return exitOK
	}
	if !wantKey {
		fmt.Println(match.User)
		return exitOK
	}
	fmt.Printf("%s|%s\n", match.Host, match.User)
	return exitOK
}

func runStatus(ctx context.Context, gf globalFlags, args []string) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	cfg, cfgPath, err := loadConfigRequired(gf.ConfigPath)
	if err != nil {
		if errors.Is(err, config.ErrConfigNotFound) {
			fmt.Fprintln(os.Stderr, "config not found")
			fmt.Fprintln(os.Stderr, "fix: run `gh-auto-switch config init` then edit the generated file")
			return exitUsage
		}
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		return exitUsage
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "getwd: %v\n", err)
		return exitFailure
	}
	cwd, _ = filepath.Abs(cwd)

	match := rules.Match(cwd, cfg.Rules)
	if match == nil {
		fmt.Printf("cwd: %s\n", cwd)
		fmt.Printf("rule: (none)\n")
		fmt.Printf("config: %s\n", cfgPath)
		return exitOK
	}

	client := gh.NewClient(gh.NewExecRunner(), gh.Options{
		Verbose: gf.Verbose && !gf.Quiet,
		Quiet:   gf.Quiet,
	})

	fmt.Printf("cwd: %s\n", cwd)
	fmt.Printf("rule: prefix=%s host=%s user=%s\n", match.Prefix, match.Host, match.User)
	if cfgPath != "" {
		fmt.Printf("config: %s\n", cfgPath)
	} else {
		fmt.Printf("config: (unknown)\n")
	}

	st, err := client.AuthStatus(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gh auth status: %v\n", err)
		return exitFailure
	}

	hostInfo := st.Hosts[match.Host]
	if hostInfo.Host == "" {
		fmt.Printf("gh: host=%s not found in `gh auth status`\n", match.Host)
		return exitOK
	}

	active := gh.ActiveUserFromAuthStatus(st, match.Host)
	if active != "" {
		fmt.Printf("gh: active=%s@%s\n", active, match.Host)
	} else {
		fmt.Printf("gh: active=(unknown) host=%s\n", match.Host)
	}
	if len(hostInfo.Accounts) > 0 {
		users := make([]string, 0, len(hostInfo.Accounts))
		for _, a := range hostInfo.Accounts {
			users = append(users, a.User)
		}
		fmt.Printf("gh: logged-in=%s\n", strings.Join(users, ", "))
	} else {
		fmt.Printf("gh: logged-in=(none parsed)\n")
	}
	return exitOK
}

func runSwitch(ctx context.Context, gf globalFlags, args []string) int {
	fs := flag.NewFlagSet("switch", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var force bool
	fs.BoolVar(&force, "force", false, "switch even if gh already appears correct")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	cfg, _, err := loadConfigRequired(gf.ConfigPath)
	if err != nil {
		if errors.Is(err, config.ErrConfigNotFound) {
			fmt.Fprintln(os.Stderr, "config not found")
			fmt.Fprintln(os.Stderr, "fix: run `gh-auto-switch config init` then edit the generated file")
			return exitUsage
		}
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		return exitUsage
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "getwd: %v\n", err)
		return exitFailure
	}
	cwd, _ = filepath.Abs(cwd)

	match := rules.Match(cwd, cfg.Rules)
	if match == nil {
		return exitOK
	}

	client := gh.NewClient(gh.NewExecRunner(), gh.Options{
		Verbose: gf.Verbose && !gf.Quiet,
		Quiet:   gf.Quiet,
	})

	st, err := client.AuthStatus(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gh auth status: %v\n", err)
		return exitFailure
	}
	hostInfo := st.Hosts[match.Host]
	if hostInfo.Host == "" {
		fmt.Fprintf(os.Stderr, "not logged in on host %s (run `gh auth login --hostname %s`)\n", match.Host, match.Host)
		return exitFailure
	}

	// Ensure the desired user is actually logged in for this host.
	if len(hostInfo.Accounts) > 0 {
		found := false
		for _, a := range hostInfo.Accounts {
			if a.User == match.User {
				found = true
				break
			}
		}
		if !found {
			fmt.Fprintf(os.Stderr, "not logged in as %s on host %s (run `gh auth login --hostname %s`)\n", match.User, match.Host, match.Host)
			return exitFailure
		}
	}

	active := gh.ActiveUserFromAuthStatus(st, match.Host)
	if !force && active != "" && active == match.User {
		return exitOK
	}

	if err := client.AuthSwitch(ctx, match.Host, match.User); err != nil {
		fmt.Fprintf(os.Stderr, "gh auth switch: %v\n", err)
		return exitFailure
	}
	return exitOK
}

func runDoctor(ctx context.Context, gf globalFlags, args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}

	cfg, cfgPath, err := loadConfigRequired(gf.ConfigPath)
	if err != nil {
		if errors.Is(err, config.ErrConfigNotFound) {
			fmt.Fprintln(os.Stderr, "config not found")
			fmt.Fprintln(os.Stderr, "fix: run `gh-auto-switch config init` then edit the generated file")
			return exitUsage
		}
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		return exitUsage
	}

	client := gh.NewClient(gh.NewExecRunner(), gh.Options{
		Verbose: gf.Verbose && !gf.Quiet,
		Quiet:   gf.Quiet,
	})

	if err := client.CheckInstalled(); err != nil {
		fmt.Fprintf(os.Stderr, "gh: %v\n", err)
		fmt.Fprintln(os.Stderr, "fix: install GitHub CLI: https://cli.github.com/")
		return exitFailure
	}

	if !gf.Quiet {
		fmt.Printf("config: %s\n", cfgPath)
	}

	st, err := client.AuthStatus(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gh auth status: %v\n", err)
		fmt.Fprintln(os.Stderr, "fix: run `gh auth login` for required accounts/hosts")
		return exitFailure
	}

	// Validate each configured (host,user) pair is present.
	missing := 0
	seen := map[string]bool{}
	for _, r := range cfg.Rules {
		key := r.Host + "|" + r.User
		if seen[key] {
			continue
		}
		seen[key] = true

		hostInfo := st.Hosts[r.Host]
		if hostInfo.Host == "" {
			fmt.Fprintf(os.Stderr, "missing host in gh auth status: %s\n", r.Host)
			fmt.Fprintf(os.Stderr, "fix: run `gh auth login --hostname %s`\n", r.Host)
			missing++
			continue
		}
		if len(hostInfo.Accounts) > 0 {
			found := false
			for _, a := range hostInfo.Accounts {
				if a.User == r.User {
					found = true
					break
				}
			}
			if !found {
				fmt.Fprintf(os.Stderr, "missing user for host %s: %s\n", r.Host, r.User)
				fmt.Fprintf(os.Stderr, "fix: run `gh auth login --hostname %s` and login as %s\n", r.Host, r.User)
				missing++
			}
		}
	}
	if missing > 0 {
		return exitFailure
	}

	// Print a compact summary.
	for host := range st.Hosts {
		active := gh.ActiveUserFromAuthStatus(st, host)
		if active != "" {
			fmt.Printf("%s: active=%s\n", host, active)
		} else {
			fmt.Printf("%s: active=(unknown)\n", host)
		}
		info := st.Hosts[host]
		if len(info.Accounts) > 0 {
			users := make([]string, 0, len(info.Accounts))
			for _, a := range info.Accounts {
				users = append(users, a.User)
			}
			fmt.Printf("%s: users=%s\n", host, strings.Join(users, ", "))
		}
	}

	return exitOK
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

var errNoConfig = errors.New("no config")

func runConfig(gf globalFlags, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: gh-auto-switch config <path|show|init> [flags]")
		return exitUsage
	}

	switch args[0] {
	case "path":
		_, cfgPath, err := loadConfigOptional(gf.ConfigPath)
		if err != nil {
			if errors.Is(err, config.ErrConfigNotFound) {
				fmt.Println("(not found)")
				return exitOK
			}
			fmt.Fprintf(os.Stderr, "config error: %v\n", err)
			return exitUsage
		}
		fmt.Println(cfgPath)
		return exitOK

	case "show":
		cfg, cfgPath, err := loadConfigOptional(gf.ConfigPath)
		if err != nil {
			if errors.Is(err, config.ErrConfigNotFound) {
				fmt.Fprintln(os.Stderr, "config not found")
				fmt.Fprintln(os.Stderr, "fix: run `gh-auto-switch config init` then edit the generated file")
				return exitUsage
			}
			fmt.Fprintf(os.Stderr, "config error: %v\n", err)
			return exitUsage
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "encode: %v\n", err)
			return exitFailure
		}
		if gf.Verbose && !gf.Quiet {
			fmt.Fprintf(os.Stderr, "config: %s\n", cfgPath)
		}
		return exitOK

	case "init":
		fs := flag.NewFlagSet("config init", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		var (
			outPath string
			format  string
			force   bool
		)
		fs.StringVar(&outPath, "path", "", "output path (default: user config dir)")
		fs.StringVar(&format, "format", "yaml", "format: yaml|json|toml")
		fs.BoolVar(&force, "force", false, "overwrite existing file")
		if err := fs.Parse(args[1:]); err != nil {
			return exitUsage
		}

		format = strings.ToLower(strings.TrimSpace(format))
		if format != "yaml" && format != "yml" && format != "json" && format != "toml" {
			fmt.Fprintf(os.Stderr, "unsupported format: %s\n", format)
			return exitUsage
		}
		if format == "yml" {
			format = "yaml"
		}

		if outPath == "" {
			dir := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
			if dir == "" {
				home, err := os.UserHomeDir()
				if err != nil || home == "" {
					fmt.Fprintf(os.Stderr, "user home dir: %v\n", err)
					return exitFailure
				}
				dir = filepath.Join(home, ".config")
			}
			ext := format
			outPath = filepath.Join(dir, "gh-auto-switch", "config."+ext)
		}

		body := renderDefaultConfig(format)
		if body == "" {
			fmt.Fprintf(os.Stderr, "failed to render config for format: %s\n", format)
			return exitFailure
		}

		if !force {
			if _, err := os.Stat(outPath); err == nil {
				fmt.Fprintf(os.Stderr, "config already exists: %s (use --force to overwrite)\n", outPath)
				return exitFailure
			}
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
			return exitFailure
		}
		if err := os.WriteFile(outPath, []byte(body), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write: %v\n", err)
			return exitFailure
		}

		if !gf.Quiet {
			fmt.Println(outPath)
		}
		return exitOK

	default:
		fmt.Fprintf(os.Stderr, "unknown config subcommand: %s\n", args[0])
		return exitUsage
	}
}

func renderDefaultConfig(format string) string {
	switch format {
	case "json":
		return `{
  "rules": [
    { "prefix": "~/Documents", "user": "user-personal", "host": "github.com" },
    { "prefix": "~/Workspace", "user": "user-work", "host": "github.com" }
  ]
}
`
	case "yaml":
		// Minimal YAML (supported by this project’s built-in YAML parser).
		return `rules:
  - prefix: ~/Documents
    user: user-personal
    host: github.com
  - prefix: ~/Workspace
    user: user-work
    host: github.com
`
	case "toml":
		return `[[rules]]
prefix = "~/Documents"
user = "user-personal"
host = "github.com"

[[rules]]
prefix = "~/Workspace"
user = "user-work"
host = "github.com"
`
	default:
		return ""
	}
}
