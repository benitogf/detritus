package code

import "testing"

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
