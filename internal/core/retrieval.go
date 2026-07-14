// Package core holds the one retrieval pipeline behind the curated KB (kb_*):
// BM25 hits → score-normalize → MMR-rerank → threshold-filter.
package core

import (
	"math"
	"unicode/utf8"
)

// Result is one retrieval hit.
type Result struct {
	DocName  string
	Section  string
	Score    float64
	Snippet  string
	Position int
}

const (
	// DefaultMMRLambda balances relevance vs diversity in MMR reranking.
	DefaultMMRLambda = 0.7
	// DefaultMinScore drops matches below 10% of the best result.
	DefaultMinScore = 0.1
)

// Rank runs the shared pipeline over raw bleve hits (assumed sorted by score
// descending): normalize to [0,1] relative to the best hit, MMR-rerank for
// diversity, then drop matches below minScore. Returns nil for no input.
func Rank(raw []Result, lambda float64, topN int, minScore float64) []Result {
	if len(raw) == 0 {
		return nil
	}
	if max := raw[0].Score; max > 0 {
		for i := range raw {
			raw[i].Score /= max
		}
	}
	ranked := mmrRerank(raw, lambda, topN)
	var filtered []Result
	for _, r := range ranked {
		if r.Score >= minScore {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

func mmrRerank(results []Result, lambda float64, topN int) []Result {
	if len(results) <= 1 {
		return results
	}
	selected := []Result{results[0]}
	remaining := results[1:]

	for len(selected) < topN && len(remaining) > 0 {
		bestIdx := 0
		bestScore := -math.MaxFloat64
		for i, candidate := range remaining {
			var maxSim float64
			for _, sel := range selected {
				if sim := docSimilarity(candidate, sel); sim > maxSim {
					maxSim = sim
				}
			}
			mmrScore := lambda*candidate.Score - (1-lambda)*maxSim
			if mmrScore > bestScore {
				bestScore = mmrScore
				bestIdx = i
			}
		}
		selected = append(selected, remaining[bestIdx])
		remaining = append(remaining[:bestIdx], remaining[bestIdx+1:]...)
	}
	return selected
}

func docSimilarity(a, b Result) float64 {
	if a.DocName == b.DocName {
		if a.Section == b.Section {
			return 1.0
		}
		return 0.5
	}
	return 0.0
}

// Truncate clips s to at most maxLen bytes (backing off to a rune boundary so
// a multi-byte rune is never split), appending an ellipsis.
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	end := maxLen
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	return s[:end] + "..."
}
