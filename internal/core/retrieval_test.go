package core

import "testing"

func TestRankNormalizesToTopHit(t *testing.T) {
	raw := []Result{
		{DocName: "a", Score: 4.0},
		{DocName: "b", Score: 2.0},
	}
	got := Rank(raw, DefaultMMRLambda, 10, DefaultMinScore)
	if len(got) == 0 || got[0].Score != 1.0 {
		t.Fatalf("top hit should normalize to 1.0, got %+v", got)
	}
}

func TestRankFiltersBelowMinScore(t *testing.T) {
	raw := []Result{
		{DocName: "a", Score: 10.0}, // → 1.0
		{DocName: "b", Score: 0.5},  // → 0.05, below default 0.1
	}
	got := Rank(raw, DefaultMMRLambda, 10, DefaultMinScore)
	for _, r := range got {
		if r.Score < DefaultMinScore {
			t.Errorf("result %s score %.3f below threshold survived", r.DocName, r.Score)
		}
	}
}

func TestRankDiversifiesAcrossDocs(t *testing.T) {
	// Two docs, interleaved scores. MMR should not return only the first doc.
	raw := []Result{
		{DocName: "a", Section: "1", Score: 10},
		{DocName: "a", Section: "2", Score: 9},
		{DocName: "b", Section: "1", Score: 8},
		{DocName: "a", Section: "3", Score: 7},
	}
	got := Rank(raw, DefaultMMRLambda, 4, DefaultMinScore)
	docs := map[string]bool{}
	for _, r := range got {
		docs[r.DocName] = true
	}
	if len(docs) < 2 {
		t.Errorf("MMR should surface more than one doc, got %v", docs)
	}
}

func TestRankEmpty(t *testing.T) {
	if got := Rank(nil, DefaultMMRLambda, 10, DefaultMinScore); got != nil {
		t.Errorf("empty input should return nil, got %v", got)
	}
}

func TestTruncate(t *testing.T) {
	if got := Truncate("hello", 10); got != "hello" {
		t.Errorf("short string should be unchanged, got %q", got)
	}
	if got := Truncate("hello world", 5); got != "hello..." {
		t.Errorf("long string should clip+ellipsis, got %q", got)
	}
}
