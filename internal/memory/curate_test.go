package memory

import (
	"testing"
	"time"
)

// setLastUsed rewrites a lesson's last_used to simulate the passage of time.
func setLastUsed(t *testing.T, id string, when time.Time) {
	t.Helper()
	l, err := Get(id)
	if err != nil {
		t.Fatal(err)
	}
	l.LastUsed = when.UTC().Format(time.RFC3339)
	if err := write(l); err != nil {
		t.Fatal(err)
	}
}

func TestDedupAtWriteNormalized(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	if _, err := Put("d", "fact", []string{"Use a Bounded Retry"}, greenSource()); err != nil {
		t.Fatal(err)
	}
	// Same insight, different case/spacing — must not duplicate.
	if _, err := Put("d", "fact", []string{"use a   bounded retry", "a genuinely new bullet"}, greenSource()); err != nil {
		t.Fatal(err)
	}
	got, _ := Get("d")
	if len(got.Bullets) != 2 {
		t.Fatalf("normalized dedup failed: expected 2 bullets, got %d: %v", len(got.Bullets), got.Bullets)
	}
}

func TestCurateAgesByLastUsed(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	now := time.Now()
	for _, id := range []string{"fresh", "mid", "old"} {
		if _, err := Put(id, "fact", []string{"content " + id}, greenSource()); err != nil {
			t.Fatal(err)
		}
	}
	setLastUsed(t, "fresh", now)
	setLastUsed(t, "mid", now.Add(-100*24*time.Hour)) // > staleAfter(90d), < archiveAfter(180d)
	setLastUsed(t, "old", now.Add(-200*24*time.Hour)) // > archiveAfter

	if _, err := Curate(CurateOptions{Now: now}); err != nil {
		t.Fatal(err)
	}
	assertStatus(t, "fresh", "active")
	assertStatus(t, "mid", "stale")
	assertStatus(t, "old", "archived")
}

func TestCurateHardCapArchivesLeastRecentlyUsed(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	now := time.Now()
	for i, id := range []string{"a", "b", "c"} {
		if _, err := Put(id, "fact", []string{"x " + id}, greenSource()); err != nil {
			t.Fatal(err)
		}
		setLastUsed(t, id, now.Add(time.Duration(-i)*time.Hour)) // a newest, c oldest
	}
	// Cap active at 2 → the least-recently-used (c) is archived.
	if _, err := Curate(CurateOptions{MaxActive: 2, Now: now}); err != nil {
		t.Fatal(err)
	}
	assertStatus(t, "a", "active")
	assertStatus(t, "b", "active")
	assertStatus(t, "c", "archived")
}

// TestPutRunsCurationPass proves the curation pass actually runs on the write
// path — the cap fires from a plain Put, with no explicit Curate call.
func TestPutRunsCurationPass(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	old := defaultMaxActive
	defaultMaxActive = 1
	defer func() { defaultMaxActive = old }()

	if _, err := Put("older", "fact", []string{"x"}, greenSource()); err != nil {
		t.Fatal(err)
	}
	setLastUsed(t, "older", time.Now().Add(-time.Hour)) // make it the LRU
	// This Put's curation pass (cap=1) must archive the least-recently-used.
	if _, err := Put("newer", "fact", []string{"y"}, greenSource()); err != nil {
		t.Fatal(err)
	}
	assertStatus(t, "older", "archived")
	assertStatus(t, "newer", "active")
}

func TestSupersedeMarksStaleNotDeleted(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	if _, err := Put("contradicted", "fact", []string{"old belief"}, greenSource()); err != nil {
		t.Fatal(err)
	}
	if err := Supersede("contradicted"); err != nil {
		t.Fatal(err)
	}
	assertStatus(t, "contradicted", "stale")
	if _, err := Get("contradicted"); err != nil {
		t.Error("superseded lesson must remain on disk for audit, not be deleted")
	}
}

func TestTouchRefreshesAndReactivates(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	if _, err := Put("p", "fact", []string{"x"}, greenSource()); err != nil {
		t.Fatal(err)
	}
	if err := Supersede("p"); err != nil { // → stale
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	setLastUsed(t, "p", old)
	if err := Touch("p"); err != nil {
		t.Fatal(err)
	}
	l, _ := Get("p")
	if l.Status != "active" {
		t.Errorf("touch should reactivate a stale-but-useful lesson, got %q", l.Status)
	}
	if lastUsedTime(l).Before(old.Add(time.Minute)) {
		t.Error("touch should refresh last_used")
	}
}

func TestArchivedNotRetrieved(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	if _, err := Put("gone", "fact", []string{"obsolete zephyr knowledge"}, greenSource()); err != nil {
		t.Fatal(err)
	}
	if r, _ := Search("zephyr", 5); len(r) == 0 {
		t.Fatal("precondition: lesson should be retrievable before archiving")
	}
	// Age it past the archive threshold, then it must drop out of retrieval.
	now := time.Now()
	setLastUsed(t, "gone", now.Add(-200*24*time.Hour))
	if _, err := Curate(CurateOptions{Now: now}); err != nil {
		t.Fatal(err)
	}
	assertStatus(t, "gone", "archived")
	if r, _ := Search("zephyr", 5); len(r) != 0 {
		t.Errorf("archived lesson must not be retrieved, got %v", r)
	}
}

func assertStatus(t *testing.T, id, want string) {
	t.Helper()
	l, err := Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if l.Status != want {
		t.Errorf("lesson %q status = %q, want %q", id, l.Status, want)
	}
}
