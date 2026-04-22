package code

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blevesearch/bleve/v2"
)

// redirectDataDir points the package's DataDir() at a temp location for this test.
func redirectDataDir(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("DETRITUS_HOME", tmp)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// countMatches runs a content-field match query and returns how many unique
// documents matched. It's a small helper for asserting index freshness.
func countMatches(t *testing.T, packName, term string) int {
	t.Helper()
	reg := NewRegistry()
	defer reg.Close()
	idx, err := reg.Open(packName)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	q := bleve.NewMatchQuery(term)
	q.SetField("content")
	req := bleve.NewSearchRequestOptions(q, 100, 0, false)
	res, err := idx.Search(req)
	if err != nil {
		t.Fatalf("search %q: %v", term, err)
	}
	return len(res.Hits)
}

func TestPackIncremental(t *testing.T) {
	redirectDataDir(t)

	src := t.TempDir()
	writeFile(t, filepath.Join(src, "main.go"), "package main\n\nfunc Hello() string { return \"oldmarker\" }\n")
	writeFile(t, filepath.Join(src, "util.py"), "def greet(name):\n    return f'hello {name}'\n")
	writeFile(t, filepath.Join(src, ".gitignore"), "secret.txt\n")
	writeFile(t, filepath.Join(src, "secret.txt"), "should-be-ignored\n")

	// Initial pack — main.go + util.py + .gitignore (secret.txt is gitignored)
	stats, err := Pack("test", []string{src}, Options{DetritusVersion: "test"})
	if err != nil {
		t.Fatalf("initial pack: %v", err)
	}
	if stats.New != 3 || stats.Modified != 0 || stats.Deleted != 0 {
		t.Fatalf("initial stats unexpected: new=%d mod=%d del=%d (wanted new=3)", stats.New, stats.Modified, stats.Deleted)
	}
	if stats.Files != 3 {
		t.Fatalf("file count: got %d want 3 (secret.txt should be gitignored)", stats.Files)
	}

	// No changes → all unchanged
	stats, err = Pack("test", nil, Options{DetritusVersion: "test"})
	if err != nil {
		t.Fatalf("noop pack: %v", err)
	}
	if stats.Unchanged != 3 || stats.New != 0 || stats.Modified != 0 || stats.Deleted != 0 {
		t.Fatalf("noop stats: new=%d mod=%d del=%d unchanged=%d", stats.New, stats.Modified, stats.Deleted, stats.Unchanged)
	}

	// Baseline: the old marker is indexed, the new one isn't.
	if n := countMatches(t, "test", "oldmarker"); n != 1 {
		t.Fatalf("oldmarker pre-modify: got %d hits want 1", n)
	}
	if n := countMatches(t, "test", "newmarker"); n != 0 {
		t.Fatalf("newmarker pre-modify: got %d hits want 0", n)
	}

	// Modify main.go → should show modified:1.
	// Set an explicit future mtime instead of sleeping: slow filesystems
	// and mtime granularity (1s on some ext4 configs, 2s on FAT) make a
	// real sleep flaky.
	mainPath := filepath.Join(src, "main.go")
	writeFile(t, mainPath, "package main\n\nfunc Hello() string { return \"newmarker\" }\n")
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(mainPath, future, future); err != nil {
		t.Fatal(err)
	}
	stats, err = Pack("test", nil, Options{DetritusVersion: "test"})
	if err != nil {
		t.Fatalf("modify pack: %v", err)
	}
	if stats.Modified != 1 || stats.New != 0 || stats.Deleted != 0 {
		t.Fatalf("modify stats: new=%d mod=%d del=%d unchanged=%d", stats.New, stats.Modified, stats.Deleted, stats.Unchanged)
	}
	// The counter said "modified", but verify the index itself was actually updated.
	if n := countMatches(t, "test", "oldmarker"); n != 0 {
		t.Fatalf("oldmarker post-modify: got %d hits want 0 (index stale)", n)
	}
	if n := countMatches(t, "test", "newmarker"); n != 1 {
		t.Fatalf("newmarker post-modify: got %d hits want 1 (index not updated)", n)
	}

	// Add a new file
	writeFile(t, filepath.Join(src, "extra.go"), "package main\nfunc Extra() {}\n")
	stats, err = Pack("test", nil, Options{DetritusVersion: "test"})
	if err != nil {
		t.Fatalf("add pack: %v", err)
	}
	if stats.New != 1 {
		t.Fatalf("add stats: new=%d want 1", stats.New)
	}

	// Delete a file
	if err := os.Remove(filepath.Join(src, "util.py")); err != nil {
		t.Fatal(err)
	}
	stats, err = Pack("test", nil, Options{DetritusVersion: "test"})
	if err != nil {
		t.Fatalf("delete pack: %v", err)
	}
	if stats.Deleted != 1 {
		t.Fatalf("delete stats: del=%d want 1", stats.Deleted)
	}
	if stats.Files != 3 {
		t.Fatalf("final file count: got %d want 3 (main.go, extra.go, .gitignore)", stats.Files)
	}
}

func TestPackManifestPersists(t *testing.T) {
	redirectDataDir(t)
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "a.go"), "package a\n")

	if _, err := Pack("persist", []string{src}, Options{DetritusVersion: "x"}); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest("persist")
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "persist" || m.FileCount != 1 || m.SchemaVersion != SchemaVersion {
		t.Fatalf("manifest round-trip: %+v", m)
	}
}

func TestPackSearchAndDocIDLookup(t *testing.T) {
	redirectDataDir(t)
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "auth.go"), "package auth\n\nfunc ValidateJWT(token string) error { return nil }\n")
	writeFile(t, filepath.Join(src, "main.go"), "package main\n\nfunc main() {}\n")

	if _, err := Pack("q", []string{src}, Options{DetritusVersion: "x"}); err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry()
	defer reg.Close()
	idx, err := reg.Open("q")
	if err != nil {
		t.Fatal(err)
	}

	// Search for "ValidateJWT"
	qq := bleve.NewMatchQuery("ValidateJWT")
	qq.SetField("content")
	req := bleve.NewSearchRequestOptions(qq, 5, 0, false)
	req.Fields = []string{"path_rel"}
	res, err := idx.Search(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) == 0 {
		t.Fatal("search returned zero hits for ValidateJWT")
	}
	p, _ := res.Hits[0].Fields["path_rel"].(string)
	if p != "auth.go" {
		t.Fatalf("top hit path_rel=%q want auth.go", p)
	}

	// Doc ID lookup via resolveDocument
	hit, err := resolveDocument(idx, "q", "auth.go", []string{"content"})
	if err != nil {
		t.Fatal(err)
	}
	content, _ := hit.Fields["content"].(string)
	if !strings.Contains(content, "ValidateJWT") {
		t.Fatalf("resolved doc missing ValidateJWT: %q", content)
	}
}

func TestOutlineGo(t *testing.T) {
	src := []byte(`package demo

import "fmt"

type Widget struct {
	Name string
	Count int
}

type Greeter interface {
	Greet() string
}

func Hello(name string) string {
	return fmt.Sprintf("hello %s", name)
}

func (w *Widget) Describe() string {
	return w.Name
}
`)
	out := Outline("go", src)
	for _, want := range []string{"package demo", "type Widget struct", "type Greeter interface", "func Hello(name string) string", "func (w *Widget) Describe() string"} {
		if !strings.Contains(out, want) {
			t.Errorf("outline missing %q; got:\n%s", want, out)
		}
	}
}

func TestOutlinePython(t *testing.T) {
	src := []byte(`class Animal:
    def __init__(self, name):
        self.name = name

def greet(who):
    return f"hi {who}"

async def fetch(url):
    pass
`)
	out := Outline("python", src)
	for _, want := range []string{"class Animal:", "def greet(who):", "async def fetch(url):"} {
		if !strings.Contains(out, want) {
			t.Errorf("python outline missing %q; got:\n%s", want, out)
		}
	}
}

func TestSliceLines(t *testing.T) {
	content := "one\ntwo\nthree\nfour\nfive\n"
	if got := SliceLines(content, 2, 4); got != "two\nthree\nfour" {
		t.Fatalf("slice 2-4: %q", got)
	}
	if got := SliceLines(content, 0, 0); got == "" {
		t.Fatal("slice 0-0 should fall through to whole content")
	}
}

func TestPackNameValidation(t *testing.T) {
	redirectDataDir(t)
	for _, bad := range []string{"", ".", "..", "../etc", "foo/bar", "abs/olute", "has space", "with|pipe"} {
		if _, err := Pack(bad, []string{t.TempDir()}, Options{}); err == nil {
			t.Errorf("Pack(%q) accepted, wanted rejection", bad)
		}
		if err := Unpack(bad); err == nil {
			t.Errorf("Unpack(%q) accepted, wanted rejection", bad)
		}
		if _, err := LoadManifest(bad); err == nil {
			t.Errorf("LoadManifest(%q) accepted, wanted rejection", bad)
		}
	}
	for _, ok := range []string{"detritus", "work", "pack-1", "pack.v2", "pack_name", "ABCabc123"} {
		if err := ValidatePackName(ok); err != nil {
			t.Errorf("ValidatePackName(%q) rejected: %v", ok, err)
		}
	}
}

func TestPackForCWD(t *testing.T) {
	redirectDataDir(t)
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "x.go"), "package x\n")
	if _, err := Pack("cwdtest", []string{src}, Options{DetritusVersion: "x"}); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(src, "deeper")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	name, err := PackForCWD(sub)
	if err != nil {
		t.Fatal(err)
	}
	if name != "cwdtest" {
		t.Fatalf("PackForCWD(%s) = %q want cwdtest", sub, name)
	}
}
