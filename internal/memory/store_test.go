package memory

import (
	"os"
	"strings"
	"testing"
)

func greenSource() Source {
	return Source{Run: "r1", Task: "t1", Outcome: "green", TS: "2026-06-24T00:00:00Z"}
}

func TestPutGateRejectsUnverified(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	_, err := Put("x", "fact", []string{"a learned thing"}, Source{Outcome: "red"})
	if err == nil {
		t.Fatal("expected the verification gate to reject a non-green outcome")
	}
	if _, err := os.Stat(lessonPath("x")); err == nil {
		t.Error("rejected put must write no file")
	}
}

func TestPutWritesFileTruthAndGet(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	id, err := Put("retry-pattern", "procedure", []string{"wrap the call in a bounded retry"}, greenSource())
	if err != nil {
		t.Fatal(err)
	}
	if id != "retry-pattern" {
		t.Fatalf("got id %q", id)
	}
	// File is the source of truth.
	raw, err := os.ReadFile(lessonPath("retry-pattern"))
	if err != nil {
		t.Fatalf("lesson file not written: %v", err)
	}
	for _, want := range []string{"kind: procedure", "trust: verified", "source_outcome: green", "- wrap the call in a bounded retry"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("lesson file missing %q\n---\n%s", want, raw)
		}
	}
	got, err := Get("retry-pattern")
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != "procedure" || got.Trust != "verified" || len(got.Bullets) != 1 {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestPutAppendsItemizedDeltaNeverRewrites(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	if _, err := Put("p", "procedure", []string{"first bullet"}, greenSource()); err != nil {
		t.Fatal(err)
	}
	if _, err := Put("p", "procedure", []string{"second bullet", "first bullet"}, greenSource()); err != nil {
		t.Fatal(err)
	}
	got, err := Get("p")
	if err != nil {
		t.Fatal(err)
	}
	// First bullet preserved (not collapsed), second appended, duplicate deduped.
	if len(got.Bullets) != 2 {
		t.Fatalf("expected 2 bullets after itemized append+dedup, got %d: %v", len(got.Bullets), got.Bullets)
	}
	if got.Bullets[0] != "first bullet" || got.Bullets[1] != "second bullet" {
		t.Errorf("append order/content wrong: %v", got.Bullets)
	}
}

func TestPutRejectsBadKind(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	if _, err := Put("p", "note", []string{"x"}, greenSource()); err == nil {
		t.Error("expected rejection of kind other than procedure/fact")
	}
}

func TestGetMissing(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	if _, err := Get("nope"); err == nil {
		t.Error("expected error for missing lesson")
	}
}

func TestEnsureStoreGitignoresIndex(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	if _, err := Put("p", "fact", []string{"x"}, greenSource()); err != nil {
		t.Fatal(err)
	}
	gi, err := os.ReadFile(MemoryDir() + "/.gitignore")
	if err != nil {
		t.Fatalf("no .gitignore: %v", err)
	}
	if !strings.Contains(string(gi), "index.bleve") {
		t.Errorf("derived index must be gitignored, got: %s", gi)
	}
}
