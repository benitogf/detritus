// Package core holds the one retrieval pipeline shared by the curated KB (kb_*)
// and the learned-memory store (memory_*): BM25 hits → score-normalize →
// MMR-rerank → threshold-filter. The two callers feed it distinct bleve indices
// and a per-result trust/source provenance field, but the engine is one — the
// trust boundary lives in the write path and the provenance field, not in a
// second engine.
package core

import "math"

// Result is one retrieval hit. Trust/Source carry provenance: they are empty
// for curated KB docs (trusted, no agent-write path) and populated for learned
// memory entries (e.g. Trust="verified", Source=run/task/outcome) so callers
// can filter on origin.
type Result struct {
	DocName  string
	Section  string
	Score    float64
	Snippet  string
	Position int
	Trust    string
	Source   string
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

// Truncate clips s to maxLen runes-ish (bytes), appending an ellipsis.
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
