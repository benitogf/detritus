package code

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func tagsByKind(tags []Tag, kind string) map[string]Tag {
	m := map[string]Tag{}
	for _, t := range tags {
		if t.Kind == kind {
			m[t.Name] = t
		}
	}
	return m
}

func TestExtractGoTagsDefsAndRefs(t *testing.T) {
	src := []byte(`package p

import "fmt"

type Widget struct{ n int }

const Limit = 10

func (w Widget) Render() string { return fmt.Sprintf("%d", helper()) }

func helper() int { return Limit }
`)
	tags := extractGoTags(src)
	defs := tagsByKind(tags, "def")
	refs := tagsByKind(tags, "ref")

	for _, name := range []string{"Widget", "Limit", "Render", "helper"} {
		if _, ok := defs[name]; !ok {
			t.Errorf("missing def tag %q", name)
		}
	}
	if got := defs["Render"].Signature; got == "" {
		t.Error("def Render has empty signature")
	}
	// Sprintf is a selector ref; helper is a bare-call ref.
	for _, name := range []string{"Sprintf", "helper"} {
		if _, ok := refs[name]; !ok {
			t.Errorf("missing ref tag %q", name)
		}
	}
}

func TestExtractGoTagsUnparseableYieldsNone(t *testing.T) {
	if tags := extractGoTags([]byte("this is not go")); tags != nil {
		t.Errorf("expected nil tags for non-Go content, got %v", tags)
	}
}

func TestTagsForFileCacheHitAndInvalidate(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir()) // isolate the on-disk cache

	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	v1 := []byte("package p\n\nfunc Alpha() {}\n")
	if err := os.WriteFile(file, v1, 0o644); err != nil {
		t.Fatal(err)
	}

	tags, err := TagsForFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tagsByKind(tags, "def")["Alpha"]; !ok {
		t.Fatal("first parse missing def Alpha")
	}
	if _, err := os.Stat(cacheFileFor(file)); err != nil {
		t.Fatalf("cache file not written: %v", err)
	}

	// Cache HIT: replace the bytes with DIFFERENT content of the SAME size,
	// then restore the original mtime+size. TagsForFile must return the stale
	// cached tags (Alpha), proving it did not re-parse.
	fi, _ := os.Stat(file)
	v2 := []byte("package p\n\nfunc Betaa() {}\n") // same length as v1
	if len(v2) != len(v1) {
		t.Fatalf("test setup: v2 len %d != v1 len %d", len(v2), len(v1))
	}
	if err := os.WriteFile(file, v2, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(file, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}
	hit, err := TagsForFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tagsByKind(hit, "def")["Alpha"]; !ok {
		t.Error("expected stale cached def Alpha on mtime/size match (cache miss = re-parsed)")
	}

	// Cache INVALIDATE: bump mtime forward; TagsForFile must re-parse and now
	// reflect the new content (Betaa).
	later := fi.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(file, later, later); err != nil {
		t.Fatal(err)
	}
	fresh, err := TagsForFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tagsByKind(fresh, "def")["Betaa"]; !ok {
		t.Error("expected re-parsed def Betaa after mtime change")
	}
	if _, ok := tagsByKind(fresh, "def")["Alpha"]; ok {
		t.Error("stale def Alpha still present after invalidation")
	}
}
