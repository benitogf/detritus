package code

import (
	"os"
	"path/filepath"
	"testing"
)

// mkdirs creates dir and writes an empty file `name` inside it.
func touch(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveProjectRootGoModWins(t *testing.T) {
	root := t.TempDir()
	mod := filepath.Join(root, "proj")
	sub := filepath.Join(mod, "internal", "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	touch(t, root, ".git")  // repo root higher up
	touch(t, mod, "go.mod") // module root in between
	touch(t, mod, "package.json")

	if got := ResolveProjectRoot(sub); got != mod {
		t.Errorf("ResolveProjectRoot(%q) = %q, want module root %q", sub, got, mod)
	}
}

func TestResolveProjectRootGitFallback(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	touch(t, root, ".git") // no go.mod anywhere

	if got := ResolveProjectRoot(sub); got != root {
		t.Errorf("ResolveProjectRoot(%q) = %q, want git root %q", sub, got, root)
	}
}

func TestResolveProjectRootMarkerFallback(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "web")
	sub := filepath.Join(proj, "src")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	touch(t, proj, "package.json") // no go.mod, no .git

	if got := ResolveProjectRoot(sub); got != proj {
		t.Errorf("ResolveProjectRoot(%q) = %q, want marker root %q", sub, got, proj)
	}
}

func TestResolveProjectRootNoMarkerReturnsDir(t *testing.T) {
	// A not-yet-initialized folder (no go.mod, .git, or marker) resolves to
	// itself — mtime-based context must work before any project init.
	dir := t.TempDir()
	if got := ResolveProjectRoot(dir); got != dir {
		t.Errorf("ResolveProjectRoot(%q) = %q, want self %q", dir, got, dir)
	}
}

func TestResolveProjectRootNestedModuleReturnsNearest(t *testing.T) {
	root := t.TempDir()
	outer := root
	inner := filepath.Join(root, "tools", "gen")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	touch(t, outer, "go.mod")
	touch(t, inner, "go.mod")

	if got := ResolveProjectRoot(inner); got != inner {
		t.Errorf("ResolveProjectRoot(%q) = %q, want nearest module %q", inner, got, inner)
	}
}

func TestResolveProjectRootFileInputUsesDir(t *testing.T) {
	root := t.TempDir()
	touch(t, root, "go.mod")
	f := filepath.Join(root, "main.go")
	touch(t, root, "main.go")

	if got := ResolveProjectRoot(f); got != root {
		t.Errorf("ResolveProjectRoot(file %q) = %q, want dir %q", f, got, root)
	}
}
