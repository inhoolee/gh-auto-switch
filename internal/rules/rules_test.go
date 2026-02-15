package rules

import "testing"

func TestMatch_Prefix(t *testing.T) {
	rs := []Rule{
		{Prefix: "/Users/testuser/Documents", User: "user-personal", Host: "github.com"},
		{Prefix: "/Users/testuser/Workspace", User: "user-work", Host: "github.com"},
	}

	if got := Match("/Users/testuser/Documents/foo", rs); got == nil || got.User != "user-personal" {
		t.Fatalf("expected user-personal, got %#v", got)
	}
	if got := Match("/Users/testuser/Workspace/a/b", rs); got == nil || got.User != "user-work" {
		t.Fatalf("expected user-work, got %#v", got)
	}
	if got := Match("/Users/testuser/Other", rs); got != nil {
		t.Fatalf("expected nil match, got %#v", got)
	}
}

func TestHasPathPrefix_Boundaries(t *testing.T) {
	if !hasPathPrefix("/a/b/c", "/a/b") {
		t.Fatal("expected prefix match")
	}
	if hasPathPrefix("/a/bc/d", "/a/b") {
		t.Fatal("expected boundary-safe non-match")
	}
}
