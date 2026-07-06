package memory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// fixtureTree writes a tiny compiling Go module whose known symbols/files a
// staleness scan can be checked against, and returns its root. It defines
// BuildIndex, the type Store, and the method Store.Save, in store.go.
func fixtureTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module fix\n\ngo 1.25\n")
	writeFile(t, root, "store.go", `package fix

import "strings"

type Store struct{}

func (s Store) Save() {}

func BuildIndex(text string) int { return len(strings.Fields(text)) }
`)
	return root
}

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// seedLesson writes an active lesson directly to the store (bypassing Put's
// verified-green gate and conflict check, which are irrelevant to a read-only
// staleness scan).
func seedLesson(t *testing.T, id, title string, bullets []string) {
	t.Helper()
	if err := ensureStore(); err != nil {
		t.Fatal(err)
	}
	if err := write(Lesson{
		ID: id, Kind: "fact", Status: "active", Trust: "verified",
		Title: title, Bullets: bullets,
	}); err != nil {
		t.Fatal(err)
	}
}

// TestStalenessFlagsDeadRefsNotLive is the core table test: a lesson whose
// symbol/file references all resolve against the fixture must NOT be flagged, a
// lesson referencing a vanished symbol AND a missing file MUST be flagged with
// both dead refs, and a prose-only lesson (no backticked/path refs) must never be
// flagged — the false-positive guard.
func TestStalenessFlagsDeadRefsNotLive(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	tree := fixtureTree(t)

	seedLesson(t, "live-lesson", "how to build the index", []string{
		"call `BuildIndex()` then `Store.Save`",
		"defined in `store.go`",
		"tokenises with `strings.Fields`", // stdlib symbol must be recognised alive
	})
	seedLesson(t, "stale-lesson", "old index recipe", []string{
		"call `VanishedFunc()` which no longer exists",
		"see `internal/gone/missing.go`",
	})
	seedLesson(t, "prose-lesson", "general advice", []string{
		"always be careful and test thoroughly before shipping",
		"prefer robust over clever; measure before optimizing",
	})

	res := CheckStaleness(tree)

	if res.Checked != 3 {
		t.Errorf("checked = %d, want 3 active lessons", res.Checked)
	}
	if len(res.Unknown) != 0 {
		t.Errorf("compiling fixture should yield no unknowns, got %+v", res.Unknown)
	}

	stale := staleByID(res)
	if _, ok := stale["live-lesson"]; ok {
		t.Errorf("live-lesson has only live refs and must not be flagged; got %+v", stale["live-lesson"])
	}
	if _, ok := stale["prose-lesson"]; ok {
		t.Errorf("prose-lesson has no code refs and must not be flagged (false-positive guard); got %+v", stale["prose-lesson"])
	}

	sl, ok := stale["stale-lesson"]
	if !ok {
		t.Fatalf("stale-lesson references a vanished symbol + missing file and must be flagged; stale=%+v", stale)
	}
	dead := map[string]string{} // ref -> kind
	for _, r := range sl.DeadRefs {
		dead[r.Ref] = r.Kind
	}
	if dead["VanishedFunc()"] != "symbol" {
		t.Errorf("expected dead symbol VanishedFunc(); got dead refs %+v", sl.DeadRefs)
	}
	if dead["internal/gone/missing.go"] != "file" {
		t.Errorf("expected dead file internal/gone/missing.go; got dead refs %+v", sl.DeadRefs)
	}
}

// TestStalenessUnknownOnNonGoTree proves the tree-not-loadable guardrail: when
// dir is not a loadable Go tree, references are reported as UNKNOWN, never dead,
// and the scan does not crash.
func TestStalenessUnknownOnNonGoTree(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	seedLesson(t, "sym-lesson", "uses a symbol", []string{"call `SomeFunc()` in `pkg.go`"})

	res := CheckStaleness(t.TempDir()) // empty dir, no go.mod → not a Go tree

	if len(res.Stale) != 0 {
		t.Errorf("a non-loadable tree must not produce DEAD findings, got %+v", res.Stale)
	}
	if len(res.Unknown) != 1 {
		t.Fatalf("expected the lesson's refs reported as unknown, got %+v", res.Unknown)
	}
	if res.Note == "" {
		t.Error("expected an explanatory note when the tree does not load")
	}
}

// TestStalenessSymbolsUnknownOnBrokenTree proves that on a tree that loads but
// does not compile, symbol refs are "unknown" (never dead) while file refs are
// still judged (a missing file is unambiguous).
func TestStalenessSymbolsUnknownOnBrokenTree(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module broken\n\ngo 1.25\n")
	writeFile(t, root, "broken.go", "package broken\n\nfunc Run() { doesNotExist() }\n") // type error

	seedLesson(t, "mixed-lesson", "mixed refs", []string{
		"call `Run()`",                   // symbol → unknown (tree doesn't compile)
		"see `internal/gone/missing.go`", // file → dead (filesystem is unambiguous)
	})

	res := CheckStaleness(root)

	stale := staleByID(res)
	if sl, ok := stale["mixed-lesson"]; ok {
		for _, r := range sl.DeadRefs {
			if r.Kind == "symbol" {
				t.Errorf("symbol on a non-compiling tree must be unknown, not dead: %+v", r)
			}
		}
		if !hasRef(sl.DeadRefs, "internal/gone/missing.go") {
			t.Errorf("missing file should still be dead on a broken tree; got %+v", sl.DeadRefs)
		}
	} else {
		t.Errorf("expected the missing file to be flagged dead; stale=%+v", stale)
	}
	if !anyUnknownSymbol(res) {
		t.Errorf("expected the symbol ref reported as unknown; unknown=%+v", res.Unknown)
	}
}

// TestStalenessToolStructuredOutput exercises skill_staleness as a real MCP
// consumer: it must advertise ReadOnlyHint + closed-world, infer an OutputSchema
// from the typed result, and populate structuredContent with the stale finding
// alongside the back-compat text block.
func TestStalenessToolStructuredOutput(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	tree := fixtureTree(t)
	seedLesson(t, "stale-lesson", "old recipe", []string{"call `VanishedFunc()`"})

	ctx, cs := connectToolsClient(t)

	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	var tool *mcp.Tool
	for _, tl := range tools.Tools {
		if tl.Name == "skill_staleness" {
			tool = tl
		}
	}
	if tool == nil {
		t.Fatal("skill_staleness not registered")
	}
	if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
		t.Error("skill_staleness: ReadOnlyHint not set")
	}
	if tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint {
		t.Error("skill_staleness: OpenWorldHint want *false")
	}
	if tool.OutputSchema == nil {
		t.Error("skill_staleness: OutputSchema not inferred from typed result")
	}

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "skill_staleness",
		Arguments: map[string]any{"dir": tree},
	})
	if err != nil {
		t.Fatalf("skill_staleness: %v", err)
	}
	if res.IsError {
		t.Fatalf("skill_staleness returned error: %v", res.Content)
	}
	if _, ok := res.Content[0].(*mcp.TextContent); !ok {
		t.Fatalf("first content not text, got %T", res.Content[0])
	}
	sc, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent not an object: %T", res.StructuredContent)
	}
	stale, ok := sc["stale"].([]any)
	if !ok || len(stale) == 0 {
		t.Fatalf("structuredContent.stale empty/wrong type: %v", sc["stale"])
	}
	entry := stale[0].(map[string]any)
	if entry["id"] != "stale-lesson" {
		t.Errorf("stale[0].id = %v, want stale-lesson", entry["id"])
	}
	if _, ok := entry["dead_refs"].([]any); !ok {
		t.Errorf("stale[0].dead_refs missing/wrong type: %v", entry["dead_refs"])
	}
}

// TestExtractRefsHeuristic pins the conservative extraction contract: exported
// bare names / dotted / parenthesised backticked tokens and source-file paths are
// extracted; bare lowercase words, version-like tokens, IPs, and plain prose are
// NOT (the false-positive guards).
func TestExtractRefsHeuristic(t *testing.T) {
	l := Lesson{
		Title: "notes",
		Bullets: []string{
			"call `BuildIndex()` and `pkg.Foo` and `T.Method`",    // symbols
			"CamelCase `DocMetadata` and `resolveDocName` names",  // bare CamelCase → symbol
			"backticked lowercase `err` `ctx` `ok` `nil` ignored", // NOT symbols
			"builtins `make()` `len()` are calls",                 // symbols (parens)
			"acronyms `JSON` `API` `Config` are not symbols",      // NOT symbols (no internal caps)
			"version `v3.33.0` and ip `10.0.1.44` are not refs",   // NOT refs
			"file `internal/code/store.go` and bare main.go here", // files (backticked + bare)
			"pure prose with no code references at all",           // nothing
		},
	}
	got := map[string]string{} // ref -> kind
	for _, r := range extractRefs(l) {
		got[r.Ref] = r.Kind
	}

	wantSymbol := []string{"BuildIndex()", "pkg.Foo", "T.Method", "DocMetadata", "resolveDocName", "make()", "len()"}
	for _, s := range wantSymbol {
		if got[s] != "symbol" {
			t.Errorf("want %q extracted as symbol; got kind %q", s, got[s])
		}
	}
	wantFile := []string{"internal/code/store.go", "main.go"}
	for _, f := range wantFile {
		if got[f] != "file" {
			t.Errorf("want %q extracted as file; got kind %q", f, got[f])
		}
	}
	// Acronyms / single-cap words with no internal lower→upper transition are the
	// intended false-negative: never extracted as symbols (the FP guard for prose
	// acronyms), alongside bare lowercase words, versions, and IPs.
	for _, no := range []string{"err", "ctx", "ok", "nil", "v3.33.0", "10.0.1.44", "JSON", "API", "Config"} {
		if _, ok := got[no]; ok {
			t.Errorf("%q must NOT be extracted as a code ref (false-positive guard); got kind %q", no, got[no])
		}
	}
}

// TestStalenessAcronymsNotFlagged proves the CamelCase-only bare-symbol rule: a
// lesson whose bullets contain backticked acronyms / all-caps / single-cap words
// (JSON, API, TLS, README, SIGTERM, Makefile) yields ZERO symbol dead-refs — they
// are not even extracted as symbol refs — while a backticked real CamelCase symbol
// that does not exist in the tree IS still flagged dead.
func TestStalenessAcronymsNotFlagged(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	tree := fixtureTree(t)

	seedLesson(t, "acronym-lesson", "config notes", []string{
		"emit `JSON` over the `API` guarded by `TLS`",
		"see the `README` and the `Makefile`; handle `SIGTERM`",
	})
	seedLesson(t, "camel-lesson", "vanished camel symbol", []string{
		"call `LoadSymbolUniverse` which is not defined in this tree",
	})

	res := CheckStaleness(tree)

	stale := staleByID(res)
	if sl, ok := stale["acronym-lesson"]; ok {
		for _, r := range sl.DeadRefs {
			if r.Kind == "symbol" {
				t.Errorf("acronym %q must not be extracted/flagged as a symbol; got %+v", r.Ref, r)
			}
		}
	}
	// And they must not surface as unknown symbol refs either (never extracted).
	for _, l := range res.Unknown {
		if l.ID != "acronym-lesson" {
			continue
		}
		for _, r := range l.UnknownRefs {
			if r.Kind == "symbol" {
				t.Errorf("acronym %q must not be extracted as a symbol ref at all; got %+v", r.Ref, r)
			}
		}
	}

	sl, ok := stale["camel-lesson"]
	if !ok {
		t.Fatalf("a real CamelCase symbol absent from the tree must be flagged dead; stale=%+v", stale)
	}
	if !hasRef(sl.DeadRefs, "LoadSymbolUniverse") {
		t.Errorf("expected dead symbol LoadSymbolUniverse; got %+v", sl.DeadRefs)
	}
}

func staleByID(res StalenessResult) map[string]StaleLesson {
	m := map[string]StaleLesson{}
	for _, l := range res.Stale {
		m[l.ID] = l
	}
	return m
}

func hasRef(refs []StaleRef, ref string) bool {
	for _, r := range refs {
		if r.Ref == ref {
			return true
		}
	}
	return false
}

func anyUnknownSymbol(res StalenessResult) bool {
	for _, l := range res.Unknown {
		for _, r := range l.UnknownRefs {
			if r.Kind == "symbol" {
				return true
			}
		}
	}
	return false
}
