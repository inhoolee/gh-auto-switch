package gh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

type Options struct {
	Verbose bool
	Quiet   bool
}

type Client struct {
	runner Runner
	opts   Options
}

func NewClient(r Runner, opts Options) *Client {
	return &Client{runner: r, opts: opts}
}

var ErrAuthListUnsupported = errors.New("`gh auth list` is not supported by this gh version")

func (c *Client) CheckInstalled() error {
	_, err := exec.LookPath(c.runner.GhPath())
	if err != nil {
		return fmt.Errorf("not found in PATH (%w)", err)
	}
	return nil
}

type AuthList struct {
	Hosts map[string]HostAuthInfo
}

type HostAuthInfo struct {
	Host       string
	Users      []string
	ActiveUser string
}

func (c *Client) AuthList(ctx context.Context) (AuthList, error) {
	out, err := c.runner.Run(ctx, c.opts, "auth", "list")
	if err != nil {
		if isAuthListUnsupported(err) {
			return AuthList{}, ErrAuthListUnsupported
		}
		return AuthList{}, err
	}
	return ParseAuthList(out), nil
}

type AuthStatus struct {
	Hosts map[string]HostStatusInfo
}

type HostStatusInfo struct {
	Host     string
	Accounts []AccountStatusInfo
}

type AccountStatusInfo struct {
	User   string
	Active bool
}

func (c *Client) AuthStatus(ctx context.Context) (AuthStatus, error) {
	out, err := c.runner.Run(ctx, c.opts, "auth", "status")
	if err != nil {
		return AuthStatus{}, err
	}
	return ParseAuthStatus(out), nil
}

func (c *Client) ActiveUserFromStatus(ctx context.Context, host string) (string, error) {
	// Prefer parsing the full `gh auth status` output, since some gh versions don't
	// support `--hostname` and some formats mark active per account lines.
	st, err := c.AuthStatus(ctx)
	if err != nil {
		return "", err
	}

	u := ActiveUserFromAuthStatus(st, host)
	if u == "" {
		return "", errors.New("unable to parse active user from `gh auth status` output")
	}
	return u, nil
}

func (c *Client) AuthSwitch(ctx context.Context, host, user string) error {
	// Prefer long flags; fall back to short flags for older gh versions.
	if _, err := c.runner.Run(ctx, c.opts, "auth", "switch", "--hostname", host, "--user", user); err == nil {
		return nil
	}
	_, err := c.runner.Run(ctx, c.opts, "auth", "switch", "-h", host, "-u", user)
	return err
}

type Runner interface {
	GhPath() string
	Run(ctx context.Context, opts Options, args ...string) (string, error)
}

type ExecRunner struct {
	ghPath string
}

func NewExecRunner() *ExecRunner {
	p := strings.TrimSpace(os.Getenv("GH_AUTO_SWITCH_GH"))
	if p == "" {
		p = "gh"
	}
	return &ExecRunner{ghPath: p}
}

func (r *ExecRunner) GhPath() string { return r.ghPath }

func (r *ExecRunner) Run(ctx context.Context, opts Options, args ...string) (string, error) {
	if opts.Verbose && !opts.Quiet {
		fmt.Fprintf(os.Stderr, "+ %s %s\n", r.ghPath, strings.Join(args, " "))
	}
	cmd := exec.CommandContext(ctx, r.ghPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Avoid printing secrets; gh auth commands should not print tokens by default.
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s: %s", r.ghPath, msg)
	}
	return stdout.String(), nil
}

var (
	reAuthListHostHeader = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	reLoggedInAs         = regexp.MustCompile(`Logged in (?:to [^ ]+ )?as ([A-Za-z0-9_-]+)`)
	reLoggedInAccount    = regexp.MustCompile(`Logged in to ([A-Za-z0-9_.-]+) account ([A-Za-z0-9_-]+)`)
	reActiveAccount      = regexp.MustCompile(`Active account:\s*([A-Za-z0-9_-]+)`)
	reUserParenActive    = regexp.MustCompile(`\b([A-Za-z0-9_-]+)\b.*\bactive\b`)
)

func ParseAuthList(out string) AuthList {
	al := AuthList{Hosts: map[string]HostAuthInfo{}}
	lines := splitLines(out)
	var curHost string

	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}

		// Host header lines are typically unindented single tokens like "github.com".
		if !strings.HasPrefix(line, " ") && reAuthListHostHeader.MatchString(trim) {
			curHost = trim
			al.Hosts[curHost] = HostAuthInfo{Host: curHost}
			continue
		}
		if curHost == "" {
			continue
		}

		info := al.Hosts[curHost]

		// "Active account: USER"
		if m := reActiveAccount.FindStringSubmatch(trim); len(m) == 2 {
			info.ActiveUser = m[1]
			al.Hosts[curHost] = info
			continue
		}

		// "✓ Logged in as USER ..."
		if m := reLoggedInAs.FindStringSubmatch(trim); len(m) == 2 {
			u := m[1]
			if !contains(info.Users, u) {
				info.Users = append(info.Users, u)
			}
			al.Hosts[curHost] = info
			continue
		}

		// Some versions may render users like: "  USER (active)".
		if m := reUserParenActive.FindStringSubmatch(trim); len(m) == 2 {
			u := m[1]
			if !contains(info.Users, u) {
				info.Users = append(info.Users, u)
			}
			if info.ActiveUser == "" {
				info.ActiveUser = u
			}
			al.Hosts[curHost] = info
			continue
		}

		al.Hosts[curHost] = info
	}

	return al
}

func ParseActiveUserFromStatus(out, host string) string {
	lines := splitLines(out)
	inHost := false
	curUser := ""

	for _, raw := range lines {
		trim := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		if trim == "" {
			continue
		}

		// Common header format: a bare hostname line ("github.com").
		if trim == host {
			inHost = true
			curUser = ""
			continue
		}
		if !inHost {
			continue
		}

		// Newer/alternative format:
		// "✓ Logged in to github.com account user-personal (keyring)"
		if m := reLoggedInAccount.FindStringSubmatch(trim); len(m) == 3 {
			// Only trust matches for the host we're querying.
			if m[1] == host {
				curUser = m[2]
			}
			continue
		}

		// Another common format:
		// "✓ Logged in to github.com as user-personal (keychain)"
		if strings.Contains(trim, host) && strings.Contains(trim, "Logged in") {
			if m := reLoggedInAs.FindStringSubmatch(trim); len(m) == 2 {
				curUser = m[1]
			}
		}

		// Active marker line in some versions:
		// "- Active account: true"
		if curUser != "" && strings.Contains(trim, "Active account: true") {
			return curUser
		}
	}

	// Fallback: first user we can find for the host.
	for _, raw := range lines {
		trim := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		if trim == "" {
			continue
		}
		if m := reLoggedInAccount.FindStringSubmatch(trim); len(m) == 3 && m[1] == host {
			return m[2]
		}
		if strings.Contains(trim, host) && strings.Contains(trim, "Logged in") {
			if m := reLoggedInAs.FindStringSubmatch(trim); len(m) == 2 {
				return m[1]
			}
		}
	}

	return ""
}

func ParseAuthStatus(out string) AuthStatus {
	as := AuthStatus{Hosts: map[string]HostStatusInfo{}}
	lines := splitLines(out)

	curHost := ""
	inHost := false
	curAcctIdx := -1

	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}

		// Host header line: typically a bare token like "github.com".
		if !strings.HasPrefix(line, " ") && reAuthListHostHeader.MatchString(trim) {
			curHost = trim
			inHost = true
			curAcctIdx = -1
			as.Hosts[curHost] = HostStatusInfo{Host: curHost}
			continue
		}
		if !inHost || curHost == "" {
			continue
		}

		info := as.Hosts[curHost]

		// Account lines.
		// "✓ Logged in to github.com account user-personal (keyring)"
		if m := reLoggedInAccount.FindStringSubmatch(trim); len(m) == 3 {
			if m[1] == curHost {
				info.Accounts = append(info.Accounts, AccountStatusInfo{User: m[2]})
				curAcctIdx = len(info.Accounts) - 1
				as.Hosts[curHost] = info
			}
			continue
		}

		// "✓ Logged in to github.com as user-personal (keychain)"
		if strings.Contains(trim, curHost) && strings.Contains(trim, "Logged in") {
			if m := reLoggedInAs.FindStringSubmatch(trim); len(m) == 2 {
				info.Accounts = append(info.Accounts, AccountStatusInfo{User: m[1]})
				curAcctIdx = len(info.Accounts) - 1
				as.Hosts[curHost] = info
				continue
			}
		}

		// Per-account active marker:
		// "- Active account: true"
		if curAcctIdx >= 0 && strings.Contains(trim, "Active account:") {
			if strings.Contains(trim, "true") {
				info.Accounts[curAcctIdx].Active = true
			}
			as.Hosts[curHost] = info
			continue
		}

		as.Hosts[curHost] = info
	}

	return as
}

func ActiveUserFromAuthStatus(as AuthStatus, host string) string {
	h := as.Hosts[host]
	for _, a := range h.Accounts {
		if a.Active {
			return a.User
		}
	}
	// Fallback: if no active marker is present, return the first account for the host.
	if len(h.Accounts) > 0 {
		return h.Accounts[0].User
	}
	return ""
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Split(s, "\n")
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func isAuthListUnsupported(err error) bool {
	// Example seen in the wild:
	//   gh: unknown command "list" for "gh auth"
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, `unknown command "list"`) && strings.Contains(msg, `for "gh auth"`) {
		return true
	}
	if strings.Contains(msg, "unknown command") && strings.Contains(msg, "auth") && strings.Contains(msg, "list") {
		return true
	}
	return false
}
