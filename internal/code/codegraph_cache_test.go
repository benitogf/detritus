package code

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadGraphCachesAndInvalidates proves code_graph does not repay the full
// packages.Load on repeated queries against an unchanged scope, and that any
// source change busts the cache. Pointer identity of the derived call graph is
// the observable: a cache hit returns the same *callEdges built on the first
// load, a reload returns a fresh one.
func TestLoadGraphCachesAndInvalidates(t *testing.T) {
	root := t.TempDir()
	writeGo(t, root, "go.mod", "module cachetest\n\ngo 1.25\n")
	writeGo(t, root, "core.go", "package cachetest\n\nfunc Compute() int { return 1 }\n")

	pkgs1, edges1, fail := loadGraph(root)
	if fail != "" {
		t.Fatalf("first load failed: %s", fail)
	}
	if pkgs1 == nil || edges1 == nil {
		t.Fatal("first load returned nil packages/edges")
	}

	_, edges2, fail := loadGraph(root)
	if fail != "" {
		t.Fatalf("cached load reported failure: %s", fail)
	}
	if edges2 != edges1 {
		t.Fatal("expected a cache hit (same *callEdges), got a reload")
	}

	// A source change must invalidate the cache and force a reload.
	writeGo(t, root, "core.go", "package cachetest\n\nfunc Compute() int { return 2 }\nfunc Extra() {}\n")

	_, edges3, fail := loadGraph(root)
	if fail != "" {
		t.Fatalf("reload after edit failed: %s", fail)
	}
	if edges3 == edges1 {
		t.Fatal("expected a reload after editing a source file, got a stale cache hit")
	}
}

// TestFingerprintScopeChangesWithSource confirms the fingerprint is stable while
// files are unchanged and shifts when one changes.
func TestFingerprintScopeChangesWithSource(t *testing.T) {
	root := t.TempDir()
	writeGo(t, root, "go.mod", "module fp\n\ngo 1.25\n")
	writeGo(t, root, "a.go", "package fp\n\nfunc A() {}\n")

	fp1 := fingerprintScope(root)
	if fp1 == "" {
		t.Fatal("fingerprint empty for a walkable scope")
	}
	if fp1 != fingerprintScope(root) {
		t.Fatal("fingerprint not stable across calls on unchanged sources")
	}

	writeGo(t, root, "a.go", "package fp\n\nfunc A() {}\nfunc B() {}\n")
	if fingerprintScope(root) == fp1 {
		t.Fatal("fingerprint did not change after editing a source file")
	}
}

// TestFingerprintScopeChangesWithManifest confirms a go.mod edit shifts the
// fingerprint even though no .go file changed. packages.Load resolves imports
// through the module manifests, so a manifest-only change (go get -u, a
// replace, a vendored-dep update) alters the loaded graph and must bust the
// cache — otherwise a tree broken by a go.mod-only edit keeps serving a healthy
// cached graph instead of the "package does not compile" fallback. The edit
// grows go.mod so the digest shifts regardless of mtime granularity.
func TestFingerprintScopeChangesWithManifest(t *testing.T) {
	root := t.TempDir()
	writeGo(t, root, "go.mod", "module fp\n\ngo 1.25\n")
	writeGo(t, root, "a.go", "package fp\n\nfunc A() {}\n")

	fp1 := fingerprintScope(root)
	if fp1 == "" {
		t.Fatal("fingerprint empty for a walkable scope")
	}

	writeGo(t, root, "go.mod", "module fp\n\ngo 1.25\n\nrequire example.com/dep v1.2.3\n")
	if fingerprintScope(root) == fp1 {
		t.Fatal("fingerprint did not change after editing go.mod")
	}
}

// TestFingerprintScopeChangesWithAncestorManifest confirms a manifest edit
// invalidates even when scope is a subdirectory and the go.mod governing it
// lives at an ancestor. The go toolchain resolves go.mod upward from the load
// Dir, so a code_graph scoped to a package subtree must still bust its cache
// when the module's manifest changes — otherwise a go get -u at the module root
// leaves every sub-scope serving a stale graph.
func TestFingerprintScopeChangesWithAncestorManifest(t *testing.T) {
	root := t.TempDir()
	writeGo(t, root, "go.mod", "module fp\n\ngo 1.25\n")
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeGo(t, sub, "a.go", "package sub\n\nfunc A() {}\n")

	fp1 := fingerprintScope(sub)
	if fp1 == "" {
		t.Fatal("fingerprint empty for a walkable sub-scope")
	}

	writeGo(t, root, "go.mod", "module fp\n\ngo 1.25\n\nrequire example.com/dep v1.2.3\n")
	if fingerprintScope(sub) == fp1 {
		t.Fatal("fingerprint did not change after editing an ancestor go.mod")
	}
}

// TestFingerprintScopeRelativeCatchesAncestorManifest guards the relative-scope
// path: packages.Load resolves go.mod upward through the absolute chain no
// matter how the scope Dir is spelled, so a relative scope must be normalized
// before the ancestor walk — otherwise the walk stops at the working directory
// (filepath.Dir(".") == ".") and a go.mod above cwd never enters the digest,
// serving a stale graph. cwd is the package subtree; the governing go.mod sits
// one level up, out of reach of a naive relative walk.
func TestFingerprintScopeRelativeCatchesAncestorManifest(t *testing.T) {
	root := t.TempDir()
	writeGo(t, root, "go.mod", "module fp\n\ngo 1.25\n")
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeGo(t, sub, "a.go", "package sub\n\nfunc A() {}\n")

	t.Chdir(sub)
	fp1 := fingerprintScope(".")
	if fp1 == "" {
		t.Fatal("fingerprint empty for a walkable relative scope")
	}

	writeGo(t, root, "go.mod", "module fp\n\ngo 1.25\n\nrequire example.com/dep v1.2.3\n")
	if fingerprintScope(".") == fp1 {
		t.Fatal("fingerprint did not change after editing a go.mod above cwd")
	}
}
