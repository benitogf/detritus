package memory

import (
	"fmt"
	"strings"
	"testing"
)

// TestMemoryRetrievalIsTokenLean records and guards LM11's token claim:
// skill_search returns ranked keys + truncated snippets, so retrieving from
// memory costs far fewer tokens than reading every lesson in full. JIT
// retrieval is neutral-or-better: it never loads the whole corpus into context.
func TestMemoryRetrievalIsTokenLean(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())

	// A sizeable corpus: many lessons, each with a long body.
	const lessons = 40
	for i := 0; i < lessons; i++ {
		body := make([]string, 12)
		for j := range body {
			body[j] = fmt.Sprintf("lesson %d bullet %d: detailed guidance about topic alpha with padding padding padding", i, j)
		}
		if _, err := Put(fmt.Sprintf("lesson-%d", i), "procedure", body, greenSource()); err != nil {
			t.Fatal(err)
		}
	}

	results, err := Search("topic alpha guidance", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected hits")
	}

	// Cost of the search result (keys + snippets).
	retrieved := 0
	for _, r := range results {
		retrieved += len(r.DocName) + len(r.Snippet)
	}
	retrievedTokens := retrieved / 4

	// Cost of reading every lesson in full.
	all, err := listLessons()
	if err != nil {
		t.Fatal(err)
	}
	full := 0
	for _, l := range all {
		full += len(l.Title) + len(strings.Join(l.Bullets, "\n"))
	}
	fullTokens := full / 4

	t.Logf("skill_search ~%d tokens vs reading all %d lessons ~%d tokens", retrievedTokens, lessons, fullTokens)
	if retrievedTokens >= fullTokens {
		t.Errorf("JIT retrieval (%d tokens) must be leaner than the whole corpus (%d tokens)", retrievedTokens, fullTokens)
	}
	// Bounded: top-5 truncated snippets stay well under a kilo-token budget.
	if retrievedTokens > 1024 {
		t.Errorf("retrieval should be budget-bounded, got ~%d tokens", retrievedTokens)
	}
}
