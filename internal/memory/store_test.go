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

// TestPutConfirmationCounter checks P3-2: a re-Put of the same id increments
// Confirmed and stamps LastConfirmed, even when every bullet dedups away.
func TestPutConfirmationCounter(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	if _, err := Put("c", "fact", []string{"a durable fact"}, greenSource()); err != nil {
		t.Fatal(err)
	}
	first, _ := Get("c")
	if first.Confirmed != 0 || first.LastConfirmed != "" {
		t.Fatalf("first put should have Confirmed=0, LastConfirmed=\"\", got %d / %q", first.Confirmed, first.LastConfirmed)
	}
	// Re-put the same insight (dedups away) — still a confirmation.
	if _, err := Put("c", "fact", []string{"a durable fact"}, greenSource()); err != nil {
		t.Fatal(err)
	}
	if _, err := Put("c", "fact", []string{"a durable fact"}, greenSource()); err != nil {
		t.Fatal(err)
	}
	got, _ := Get("c")
	if got.Confirmed != 2 {
		t.Errorf("Confirmed = %d, want 2 after two re-puts", got.Confirmed)
	}
	if got.LastConfirmed == "" {
		t.Error("LastConfirmed not stamped on re-put")
	}
	if len(got.Bullets) != 1 {
		t.Errorf("bullets should still dedup to 1, got %d", len(got.Bullets))
	}
}

// TestConfirmedBackCompatOldRecord checks that a lesson file written before
// P3-2 (no confirmed/last_confirmed frontmatter) parses with Confirmed=0 and
// that a subsequent re-Put upgrades it in place.
func TestConfirmedBackCompatOldRecord(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	if err := ensureStore(); err != nil {
		t.Fatal(err)
	}
	old := "---\nid: legacy\nkind: fact\nstatus: active\ntrust: verified\nsource_outcome: green\nlast_used: 2026-01-01T00:00:00Z\nvalidity: \n---\n# legacy\n- an old belief\n"
	if err := os.WriteFile(lessonPath("legacy"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Get("legacy")
	if err != nil {
		t.Fatal(err)
	}
	if got.Confirmed != 0 || got.LastConfirmed != "" {
		t.Fatalf("old record must parse to Confirmed=0/LastConfirmed=\"\", got %d / %q", got.Confirmed, got.LastConfirmed)
	}
	if len(got.Bullets) != 1 || got.Bullets[0] != "an old belief" {
		t.Fatalf("old record body lost in parse: %v", got.Bullets)
	}
	// Re-put upgrades the record: counter starts from 0.
	if _, err := Put("legacy", "fact", []string{"a corroborating detail"}, greenSource()); err != nil {
		t.Fatal(err)
	}
	up, _ := Get("legacy")
	if up.Confirmed != 1 {
		t.Errorf("Confirmed = %d, want 1 after re-put of legacy record", up.Confirmed)
	}
}

// TestPutDetectsContradiction checks P3-4: a lesson with a near-identical topic
// (title) but disjoint content as an existing active lesson is refused with an
// error naming the conflicting id, rather than silently appended.
func TestPutDetectsContradiction(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	if _, err := Put("race-on-shared-counter", "procedure",
		[]string{"guard the counter with a mutex"}, greenSource()); err != nil {
		t.Fatal(err)
	}
	// Same topic (title tokens ⊃ the original), opposite advice.
	_, err := Put("race-on-shared-counter-map", "procedure",
		[]string{"use sync/atomic for the counter, never a mutex"}, greenSource())
	if err == nil {
		t.Fatal("expected a contradiction to be refused")
	}
	if !strings.Contains(err.Error(), "race-on-shared-counter") || !strings.Contains(err.Error(), "supersedes") {
		t.Errorf("conflict error must name the id and point at supersedes: %v", err)
	}
	if _, statErr := os.Stat(lessonPath("race-on-shared-counter-map")); statErr == nil {
		t.Error("a refused conflicting put must not write a file")
	}
}

// TestNoConflictOnDistinctTopics guards against false positives: same kind but
// different topics (titles diverge) must never be flagged.
func TestNoConflictOnDistinctTopics(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	if _, err := Put("bounded-retry-with-backoff", "procedure",
		[]string{"wrap the call in a bounded retry with jittered backoff"}, greenSource()); err != nil {
		t.Fatal(err)
	}
	if _, err := Put("graceful-shutdown-on-sigterm", "procedure",
		[]string{"trap SIGTERM and drain in-flight work before exiting"}, greenSource()); err != nil {
		t.Fatalf("distinct-topic lesson must not be flagged as a conflict: %v", err)
	}
}

// TestNoConflictOnDuplicateContent: a near-identical title WITH near-identical
// content is a duplicate, not a contradiction — it must not be flagged.
func TestNoConflictOnDuplicateContent(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	if _, err := Put("handle-nil-map-write", "procedure",
		[]string{"initialize the map before writing"}, greenSource()); err != nil {
		t.Fatal(err)
	}
	if _, err := Put("handle-nil-map-write-safe", "procedure",
		[]string{"initialize the map before writing"}, greenSource()); err != nil {
		t.Fatalf("duplicate content must not be flagged as a conflict: %v", err)
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
