package gh

import "testing"

func TestParseAuthList(t *testing.T) {
	out := `
github.com
  ✓ Logged in as user-personal (oauth_token)
  ✓ Logged in as user-work (oauth_token)
  Active account: user-work
`
	al := ParseAuthList(out)
	info := al.Hosts["github.com"]
	if info.Host != "github.com" {
		t.Fatalf("expected host github.com, got %#v", info)
	}
	if info.ActiveUser != "user-work" {
		t.Fatalf("expected active user-work, got %#v", info)
	}
	if len(info.Users) != 2 {
		t.Fatalf("expected 2 users, got %#v", info.Users)
	}
}

func TestParseActiveUserFromStatus(t *testing.T) {
	out := `
github.com
  ✓ Logged in to github.com as user-personal (keychain)
  ✓ Git operations for github.com configured to use https protocol.
`
	u := ParseActiveUserFromStatus(out, "github.com")
	if u != "user-personal" {
		t.Fatalf("expected user-personal, got %q", u)
	}
}

func TestParseActiveUserFromStatus_AccountFormat(t *testing.T) {
	out := `
github.com
  ✓ Logged in to github.com account user-personal (keyring)
  - Active account: true

  ✓ Logged in to github.com account user-work (keyring)
  - Active account: false
`
	u := ParseActiveUserFromStatus(out, "github.com")
	if u != "user-personal" {
		t.Fatalf("expected user-personal, got %q", u)
	}
}

func TestParseAuthStatus_Accounts(t *testing.T) {
	out := `
github.com
  ✓ Logged in to github.com account user-personal (keyring)
  - Active account: true

  ✓ Logged in to github.com account user-work (keyring)
  - Active account: false
`
	st := ParseAuthStatus(out)
	h := st.Hosts["github.com"]
	if h.Host != "github.com" {
		t.Fatalf("expected host github.com, got %#v", h)
	}
	if got := ActiveUserFromAuthStatus(st, "github.com"); got != "user-personal" {
		t.Fatalf("expected active user-personal, got %q", got)
	}
	if len(h.Accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %#v", h.Accounts)
	}
}
