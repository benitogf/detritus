package memory

import (
	"regexp"
	"strings"

	"github.com/benitogf/detritus/internal/code"
)

// P3-5 code-ref staleness: because detritus co-locates the lesson store AND live
// code intelligence, it can flag lessons whose code references have gone stale
// against the live tree. This is a CONSERVATIVE, ADVISORY scan — it reports only,
// never mutates/supersedes a lesson, and it prefers false negatives (a stale ref
// missed) over false positives (a live ref wrongly flagged).

const (
	// maxLessonsScanned bounds a single scan so a huge corpus can't wedge the
	// tool; truncation is surfaced, never silent.
	maxLessonsScanned = 5000
	// maxRefsPerLesson bounds extraction per lesson for the same reason.
	maxRefsPerLesson = 200
)

// sourceExts are the file suffixes we treat as a source-file reference. A ref
// ending in one of these is classified as a file; anything else is either a
// symbol (if backticked and identifier-shaped) or ignored as prose.
var sourceExts = []string{
	".go", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs",
	".gd", ".py", ".rs", ".java", ".kt", ".rb",
	".c", ".h", ".cc", ".cpp", ".hpp", ".cs",
}

// backtickRe captures the content of each `...` span in a lesson's text. Only
// backticked spans are considered for symbol references — bare prose words are
// never treated as symbols, which is the primary false-positive guard.
var backtickRe = regexp.MustCompile("`([^`]+)`")

// identRe matches a Go identifier or a dotted qualified name (Foo, pkg.Foo,
// T.Method, pkg.T.Method) with NO trailing parens — callers strip "()" first.
var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)

// pathRe matches a filename or repo-relative path built only of path-safe
// characters — used to reject prose before the source-extension test.
var pathRe = regexp.MustCompile(`^[A-Za-z0-9_./-]+$`)

// codeRef is one extracted candidate reference and its kind.
type codeRef struct {
	Ref  string
	Kind string // "symbol" | "file"
}

// StaleRef is one dead-or-unverifiable reference in a result.
type StaleRef struct {
	Ref  string `json:"ref"`
	Kind string `json:"kind"` // "symbol" | "file"
}

// StaleLesson is an active lesson with one or more DEAD references.
type StaleLesson struct {
	ID       string     `json:"id"`
	Title    string     `json:"title"`
	DeadRefs []StaleRef `json:"dead_refs"`
}

// UnknownLesson is an active lesson with references we could not verify (the tree
// did not load / did not compile) — reported honestly as unknown, never as dead.
type UnknownLesson struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	UnknownRefs []StaleRef `json:"unknown_refs"`
}

// StalenessResult is the typed, structured result of a staleness scan so the SDK
// emits an outputSchema + structuredContent (mirrors skill_search's shape).
type StalenessResult struct {
	Checked   int             `json:"checked"` // active lessons scanned
	Stale     []StaleLesson   `json:"stale"`
	Unknown   []UnknownLesson `json:"unknown"`
	Truncated bool            `json:"truncated"`
	Note      string          `json:"note,omitempty"`
}

// CheckStaleness scans every ACTIVE lesson for code references that have gone
// dead against the live Go tree at dir (empty → the current project). For each
// lesson it extracts candidate references from title + bullets and checks each
// against internal/code's loaded symbol/file universe. A reference is:
//   - dead    — the tree loaded and the reference resolves to nothing;
//   - unknown — the tree could not be loaded/compiled, so no judgment is safe;
//   - live    — resolves (not reported).
//
// It is purely advisory: it reports, and never modifies, supersedes, or deletes
// any lesson.
func CheckStaleness(dir string) StalenessResult {
	res := StalenessResult{Stale: []StaleLesson{}, Unknown: []UnknownLesson{}}
	lessons, err := listLessons()
	if err != nil {
		res.Note = "could not read the lesson store: " + err.Error()
		return res
	}

	u := code.LoadSymbolUniverse(dir)
	switch {
	case !u.Loaded:
		res.Note = "dir is not a loadable Go tree — every code reference is reported as unknown, not dead"
	case !u.Typed:
		res.Note = "the Go tree did not type-check clean — symbol references are reported as unknown (file references are still checked)"
	}

	for _, l := range lessons {
		if l.Status != "active" {
			continue
		}
		if res.Checked >= maxLessonsScanned {
			res.Truncated = true
			break
		}
		res.Checked++

		var dead, unknown []StaleRef
		for _, r := range extractRefs(l) {
			known, alive := classifyRef(u, r)
			switch {
			case !known:
				unknown = append(unknown, StaleRef{Ref: r.Ref, Kind: r.Kind})
			case !alive:
				dead = append(dead, StaleRef{Ref: r.Ref, Kind: r.Kind})
			}
		}
		if len(dead) > 0 {
			res.Stale = append(res.Stale, StaleLesson{ID: l.ID, Title: l.Title, DeadRefs: dead})
		}
		if len(unknown) > 0 {
			res.Unknown = append(res.Unknown, UnknownLesson{ID: l.ID, Title: l.Title, UnknownRefs: unknown})
		}
	}
	return res
}

// classifyRef decides whether a reference is verifiable against the universe and,
// if so, whether it is alive. Returns (known, alive). Symbols are verifiable only
// when the tree type-checked (u.Typed); files only when it loaded (u.Loaded) — a
// missing file is unambiguous once we know we're looking at the right tree, while
// a missing symbol on a broken tree is not.
func classifyRef(u *code.SymbolUniverse, r codeRef) (known, alive bool) {
	switch r.Kind {
	case "file":
		if !u.Loaded {
			return false, false
		}
		return true, u.HasFile(r.Ref)
	case "symbol":
		if !u.Typed {
			return false, false
		}
		return true, u.HasSymbol(r.Ref)
	}
	return false, false
}

// extractRefs pulls conservative code-reference candidates out of a lesson's
// title + bullets. The heuristic:
//
//   - Backticked tokens: a `...` span whose content ends in a source extension
//     and is path-shaped is a FILE ref; otherwise, stripped of a trailing "()",
//     a token that is a Go identifier or dotted qualified name is a SYMBOL ref
//     ONLY when it (a) had trailing "()" (clearly a call), (b) is a dotted name
//     (clearly qualified), or (c) is a bare name starting with an uppercase
//     letter (the exported-name convention). Bare lowercase words (err, ctx, ok,
//     nil, id) are ignored — they are almost always locals/builtins/prose.
//   - Bare (non-backticked) tokens are considered ONLY as FILE refs: a
//     whitespace token, trimmed of surrounding punctuation, that is path-shaped
//     and ends in a source extension. Bare symbols are never extracted, since a
//     capitalised word in prose is common and would produce false positives.
//
// Results are de-duplicated by (kind, ref) and capped at maxRefsPerLesson.
func extractRefs(l Lesson) []codeRef {
	seen := map[string]bool{}
	var out []codeRef
	add := func(ref, kind string) bool {
		if len(out) >= maxRefsPerLesson {
			return false
		}
		key := kind + "\x00" + ref
		if seen[key] {
			return true
		}
		seen[key] = true
		out = append(out, codeRef{Ref: ref, Kind: kind})
		return true
	}

	texts := make([]string, 0, len(l.Bullets)+1)
	texts = append(texts, l.Title)
	texts = append(texts, l.Bullets...)

	for _, text := range texts {
		for _, m := range backtickRe.FindAllStringSubmatch(text, -1) {
			tok := strings.TrimSpace(m[1])
			if kind, ok := classifyBacktick(tok); ok {
				if !add(tok, kind) {
					return out
				}
			}
		}
		for _, field := range strings.Fields(text) {
			if tok, ok := bareFileToken(field); ok {
				if !add(tok, "file") {
					return out
				}
			}
		}
	}
	return out
}

// classifyBacktick classifies a single backticked token. Returns ("file"|"symbol", true)
// or ("", false) when the token is prose (not a code reference).
func classifyBacktick(tok string) (string, bool) {
	if tok == "" {
		return "", false
	}
	// File first: `store.go` also matches the dotted-identifier pattern, so the
	// source-extension test must win.
	if isFilePath(tok) {
		return "file", true
	}
	base := strings.TrimSuffix(tok, "()")
	hadParens := base != tok
	if !identRe.MatchString(base) {
		return "", false
	}
	switch {
	case hadParens: // Foo()
		return "symbol", true
	case strings.Contains(base, "."): // pkg.Foo, T.Method
		return "symbol", true
	case base[0] >= 'A' && base[0] <= 'Z': // exported bare name: Config, BuildIndex
		return "symbol", true
	default: // bare lowercase word — ignore (err, ctx, ok, nil, …)
		return "", false
	}
}

// bareFileToken extracts a file reference from a bare (non-backticked) word: it
// trims surrounding punctuation and returns the token if it is a path-shaped
// source filename. Only files are extracted from bare text (a capitalised word
// in prose is too common to treat as a symbol). isFilePath requires the trailing
// extension, so a trimmed word like "sentence." (→ "sentence") never qualifies.
func bareFileToken(field string) (string, bool) {
	tok := strings.Trim(field, "()[]{},.;:'\"")
	if isFilePath(tok) {
		return tok, true
	}
	return "", false
}

// isFilePath reports whether tok is a path-shaped token ending in a known source
// extension (so `internal/code/store.go` and `main.go` qualify; `v3.33.0`,
// `10.0.1.44`, and `pkg.Method` do not).
func isFilePath(tok string) bool {
	if tok == "" || !pathRe.MatchString(tok) {
		return false
	}
	lower := strings.ToLower(tok)
	for _, ext := range sourceExts {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}
