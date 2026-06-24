package memory

import "testing"

func TestRecallStatsCountsMisses(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	if err := LogSearch("found something", 3); err != nil {
		t.Fatal(err)
	}
	if err := LogSearch("found nothing", 0); err != nil {
		t.Fatal(err)
	}
	if err := LogSearch("also nothing", 0); err != nil {
		t.Fatal(err)
	}
	r, err := RecallStats()
	if err != nil {
		t.Fatal(err)
	}
	if r.Total != 3 || r.Misses != 2 {
		t.Fatalf("expected total=3 misses=2, got %+v", r)
	}
	if got := r.MissFraction(); got < 0.66 || got > 0.67 {
		t.Errorf("miss fraction = %.3f, want ~0.667", got)
	}
}

func TestRecallStatsEmpty(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	r, err := RecallStats()
	if err != nil {
		t.Fatal(err)
	}
	if r.Total != 0 || r.MissFraction() != 0 {
		t.Errorf("empty log should be zero, got %+v", r)
	}
}

func TestSearchAndLogRecordsHitAndMiss(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	if _, err := Put("widget-lesson", "fact", []string{"widgets need a flux capacitor"}, greenSource()); err != nil {
		t.Fatal(err)
	}
	// A hit: query matches the lesson.
	hit, err := SearchAndLog("flux capacitor widgets", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hit) == 0 {
		t.Fatal("expected a hit for the matching query")
	}
	// A miss: nothing matches.
	miss, err := SearchAndLog("zzzz nonexistent quux", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(miss) != 0 {
		t.Fatalf("expected a miss, got %d results", len(miss))
	}
	r, err := RecallStats()
	if err != nil {
		t.Fatal(err)
	}
	if r.Total != 2 || r.Misses != 1 {
		t.Errorf("expected total=2 misses=1 after one hit + one miss, got %+v", r)
	}
}
