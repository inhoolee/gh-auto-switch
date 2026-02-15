package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSwitch_UsesMockGh(t *testing.T) {
	tmp := t.TempDir()

	// Create a fake repo path that matches our config rule.
	root := filepath.Join(tmp, "Workspace")
	cwd := filepath.Join(root, "proj")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}

	// Config: match tmp Workspace and choose desired user.
	cfgPath := filepath.Join(tmp, "config.json")
	cfg := `{
  "rules": [
    { "prefix": "` + escapeJSON(root) + `", "user": "user-work", "host": "github.com" }
  ]
}`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	recordPath := filepath.Join(tmp, "calls.txt")
	ghPath := filepath.Join(tmp, "gh")
	ghScript := `#!/bin/sh
set -eu
record="` + recordPath + `"

echo "$*" >> "$record"

if [ "$#" -ge 2 ] && [ "$1" = "auth" ] && [ "$2" = "status" ]; then
  cat <<'EOF'
github.com
  ✓ Logged in to github.com account user-personal (keyring)
  - Active account: true

  ✓ Logged in to github.com account user-work (keyring)
  - Active account: false
EOF
  exit 0
fi

if [ "$#" -ge 2 ] && [ "$1" = "auth" ] && [ "$2" = "switch" ]; then
  exit 0
fi

echo "unexpected args: $*" >&2
exit 2
`
	if err := os.WriteFile(ghPath, []byte(ghScript), 0o700); err != nil {
		t.Fatal(err)
	}

	oldwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(oldwd) })
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GH_AUTO_SWITCH_GH", ghPath)
	t.Setenv("GH_AUTO_SWITCH_CONFIG", cfgPath)

	// Should switch from user-personal -> user-work.
	code := Run([]string{"-q", "switch"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}

	b, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "auth switch") || !strings.Contains(string(b), "user-work") {
		t.Fatalf("expected auth switch to be invoked for user-work, got:\n%s", string(b))
	}
}

func escapeJSON(s string) string {
	// Minimal escape for backslashes and quotes.
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
