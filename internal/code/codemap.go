package code

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dcadenas/pagerank"
)

const (
	// DefaultMapBudget is the default token ceiling for a code map.
	DefaultMapBudget = 1024

	focusPathWeight   = 50.0 // a file whose path matches a focus string
	focusIdentWeight  = 10.0 // a file that defines a focus identifier
	pagerankDamping   = 0.85
	pagerankTolerance = 1e-6
	charsPerToken     = 4 // rough token estimate: len(text)/4
)

// MapOptions configures a code map.
type MapOptions struct {
	Scope  string   // dir to map; empty → the current project (ResolveProjectRoot of cwd)
	Focus  []string // identifiers and/or repo-relative path fragments to bias toward
	Budget int      // token budget; <=0 → DefaultMapBudget
}

// mapFile is a Go file plus its extracted tags during map assembly.
type mapFile struct {
	rel  string
	abs  string
	defs []Tag
	refs []string
}

// BuildCodeMap returns a ranked, token-budgeted, plain-text structural overview
// of the Go files under the scope. Files are ranked by PageRank over the
// file→file symbol reference graph (a file links to every file that defines a
// symbol it references); `focus` biases the ranking toward named paths and
// identifiers. No setup, no stored index — tags come from the mtime-keyed parse
// cache, recomputed live per call.
func BuildCodeMap(opts MapOptions) (string, error) {
	budget := opts.Budget
	if budget <= 0 {
		budget = DefaultMapBudget
	}

	root := opts.Scope
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		root = ResolveProjectRoot(wd)
	} else if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}

	walkRes, err := Walk([]string{root})
	if err != nil {
		return "", err
	}

	var files []*mapFile
	defIndex := map[string][]int{} // def name → indices of files defining it
	for _, fi := range walkRes.Files {
		if fi.Language != "go" {
			continue
		}
		abs := filepath.Join(fi.Root, filepath.FromSlash(fi.PathRel))
		tags, terr := TagsForFile(abs)
		if terr != nil {
			continue
		}
		mf := &mapFile{rel: fi.PathRel, abs: abs}
		for _, t := range tags {
			switch t.Kind {
			case "def":
				mf.defs = append(mf.defs, t)
			case "ref":
				mf.refs = append(mf.refs, t.Name)
			}
		}
		idx := len(files)
		files = append(files, mf)
		for _, d := range mf.defs {
			defIndex[d.Name] = append(defIndex[d.Name], idx)
		}
	}
	if len(files) == 0 {
		return "", nil
	}

	ranks := rankFiles(files, defIndex)
	order := orderByFocusedScore(files, ranks, opts.Focus)
	return assembleMap(files, order, budget), nil
}

// rankFiles builds the reference graph and returns each file's PageRank. An
// edge A→B is added once per distinct symbol A references that B defines;
// repeated pagerank.Link calls accumulate as integer edge weight.
func rankFiles(files []*mapFile, defIndex map[string][]int) []float64 {
	graph := pagerank.New()
	linked := false
	for ai, f := range files {
		for _, refName := range f.refs {
			for _, bi := range defIndex[refName] {
				if bi != ai {
					graph.Link(ai, bi)
					linked = true
				}
			}
		}
	}
	ranks := make([]float64, len(files))
	if !linked {
		return ranks
	}
	graph.Rank(pagerankDamping, pagerankTolerance, func(id int, rank float64) {
		if id >= 0 && id < len(ranks) {
			ranks[id] = rank
		}
	})
	return ranks
}

// orderByFocusedScore returns file indices sorted by focused score, descending.
// A uniform base floor (1/N) ensures the focus multipliers always bite — even
// for isolated files PageRank left at zero — so focus measurably reorders.
func orderByFocusedScore(files []*mapFile, ranks []float64, focus []string) []int {
	base := 1.0 / float64(len(files))
	score := make([]float64, len(files))
	for i, f := range files {
		m := 1.0
		if f.matchesFocusPath(focus) {
			m *= focusPathWeight
		}
		if f.definesFocusIdent(focus) {
			m *= focusIdentWeight
		}
		score[i] = (ranks[i] + base) * m
	}
	order := make([]int, len(files))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		if score[order[a]] != score[order[b]] {
			return score[order[a]] > score[order[b]]
		}
		return files[order[a]].rel < files[order[b]].rel
	})
	return order
}

// assembleMap renders ranked files until the token budget is reached, always
// emitting at least the top file so a tiny budget still yields something.
func assembleMap(files []*mapFile, order []int, budget int) string {
	var b strings.Builder
	used := 0
	for _, i := range order {
		block := renderFile(files[i])
		cost := estimateMapTokens(block)
		if used+cost > budget && b.Len() > 0 {
			break
		}
		b.WriteString(block)
		used += cost
	}
	return b.String()
}

// renderFile emits a file's path and its def signatures (source order, one per
// line, whitespace-collapsed so multi-line signatures stay on one line).
func renderFile(f *mapFile) string {
	defs := append([]Tag(nil), f.defs...)
	sort.SliceStable(defs, func(i, j int) bool { return defs[i].Line < defs[j].Line })
	var b strings.Builder
	b.WriteString(f.rel)
	b.WriteByte('\n')
	for _, d := range defs {
		sig := d.Signature
		if sig == "" {
			sig = d.Name
		}
		b.WriteString("  ")
		b.WriteString(strings.Join(strings.Fields(sig), " "))
		b.WriteByte('\n')
	}
	return b.String()
}

func estimateMapTokens(s string) int {
	return len(s) / charsPerToken
}

func (f *mapFile) matchesFocusPath(focus []string) bool {
	for _, q := range focus {
		if q != "" && strings.Contains(f.rel, q) {
			return true
		}
	}
	return false
}

func (f *mapFile) definesFocusIdent(focus []string) bool {
	for _, d := range f.defs {
		for _, q := range focus {
			if d.Name == q {
				return true
			}
		}
	}
	return false
}
