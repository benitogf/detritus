package memory

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestSearchRetrievesDistilledLesson(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	if _, err := Put("flaky-waitgroup", "procedure",
		[]string{"use an exact-count sync.WaitGroup to make async delivery tests deterministic"},
		greenSource()); err != nil {
		t.Fatal(err)
	}
	results, err := Search("deterministic async test WaitGroup", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected the distilled lesson to be FTS-retrievable")
	}
	found := false
	for _, r := range results {
		if r.DocName == "flaky-waitgroup" {
			found = true
			if r.Trust != "verified" {
				t.Errorf("retrieved lesson should carry trust=verified, got %q", r.Trust)
			}
		}
	}
	if !found {
		t.Errorf("flaky-waitgroup not in results: %+v", results)
	}
}

func TestSearchEmptyMemory(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	results, err := Search("anything", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("empty memory should return no results, got %v", results)
	}
}

func TestSearchReflectsNewWrites(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	if _, err := Put("first", "fact", []string{"alpha concept about widgets"}, greenSource()); err != nil {
		t.Fatal(err)
	}
	if r, _ := Search("widgets", 5); len(r) == 0 {
		t.Fatal("first lesson not retrievable")
	}
	// A second write must be reflected on the next search (index rebuilt from disk).
	if _, err := Put("second", "fact", []string{"beta concept about gadgets"}, greenSource()); err != nil {
		t.Fatal(err)
	}
	r, err := Search("gadgets", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(r) == 0 || r[0].DocName != "second" {
		t.Errorf("rebuilt index should surface the new lesson, got %+v", r)
	}
}

// TestConcurrentWritesAndSearches exercises the flock guard: many sessions
// writing + searching at once must not corrupt the derived index (LM3).
func TestConcurrentWritesAndSearches(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	const n = 8
	var wg sync.WaitGroup
	wg.Add(n * 2)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("lesson-%d", i)
		go func() {
			defer wg.Done()
			_, _ = Put(id, "fact", []string{"content for " + id + " about topic" + id}, greenSource())
		}()
		go func() {
			defer wg.Done()
			_, _ = Search("topic", 5)
		}()
	}
	wg.Wait()

	// All writes are durable files; a final search rebuilds and finds them.
	results, err := Search("content topic", n)
	if err != nil {
		t.Fatalf("post-concurrency search errored (index corrupted?): %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected lessons retrievable after concurrent writes")
	}
	for i := 0; i < n; i++ {
		if _, err := Get(fmt.Sprintf("lesson-%d", i)); err != nil {
			t.Errorf("lesson-%d file missing after concurrent writes: %v", i, err)
		}
	}
}

func TestSearchSnippetIsNotWholeCorpus(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	big := make([]string, 50)
	for i := range big {
		big[i] = fmt.Sprintf("bullet %d with searchable keyword zebra and lots of extra text padding padding", i)
	}
	if _, err := Put("huge", "procedure", big, greenSource()); err != nil {
		t.Fatal(err)
	}
	results, err := Search("zebra", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected a hit")
	}
	// JIT: snippet is truncated, not the full lesson body.
	full, _ := Get("huge")
	fullLen := len(strings.Join(full.Bullets, "\n"))
	if len(results[0].Snippet) >= fullLen {
		t.Errorf("snippet (%d) should be much smaller than the full lesson (%d) — JIT retrieval", len(results[0].Snippet), fullLen)
	}
}
