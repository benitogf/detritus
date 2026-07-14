package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// #149: the sidecar endpoints must be overridable so a non-default deployment or
// a second sidecar on other ports can be targeted without a rebuild.
func TestEnvURLOr(t *testing.T) {
	const key = "CANDYLAND_TEST_URL"
	t.Setenv(key, "")
	if got := envURLOr(key, "http://default:1"); got != "http://default:1" {
		t.Errorf("blank env should yield default, got %q", got)
	}
	t.Setenv(key, "  http://override:9  ")
	if got := envURLOr(key, "http://default:1"); got != "http://override:9" {
		t.Errorf("set env should win (trimmed), got %q", got)
	}
}

// #149: the inherited-MCP file carries secret env values, so it must live in a
// per-user cache dir (fixed name, no accumulation) — never a predictable path in
// the world-shared TempDir where concurrent users collide.
func TestOriginMCPConfigPathPerUser(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache) // platformCacheDir honors this on linux

	path, err := originMCPConfigPath()
	if err != nil {
		t.Fatalf("originMCPConfigPath: %v", err)
	}
	if !strings.HasPrefix(path, cache+string(os.PathSeparator)) {
		t.Errorf("path %q is not under the per-user cache dir %q", path, cache)
	}
	if filepath.Base(path) != "origin-mcp.json" {
		t.Errorf("path %q should end in the fixed name origin-mcp.json", path)
	}
	// The parent dir is created 0700 (owner-only) so the secret file is not
	// exposed to other users.
	info, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat parent dir: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("cache dir perm = %v, want 0700", info.Mode().Perm())
	}
}
