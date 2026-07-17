package code

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"golang.org/x/tools/go/packages"
)

// reachableCap bounds the reachable-from BFS and the impacted-by production
// dependent count; truncation is noted, never silent. It is a var (not const)
// only so tests can lower it to exercise the bound on small fixtures.
var reachableCap = 100

const impactDepth = 3 // bound the impacted-by (reverse-transitive) walk; truncation is noted, never silent

// GraphQuery configures a code_graph navigation.
type GraphQuery struct {
	Symbol string // a function or interface name to navigate
	Scope  string // dir to load; empty → the current project
}

// GraphRef is one symbol reference in a structured code_graph result.
type GraphRef struct {
	Name string `json:"name"`
	Pos  string `json:"pos"`
}

// GraphResult is the typed structured result of code_graph so the SDK emits an
// outputSchema + structuredContent (mirrors kb_search). Callers is
// who-calls (1-hop reverse); Reachable is reachable-from (transitive forward via
// callees); ImpactedBy is what transitively reaches the symbol (reverse walk via
// callers, bounded to impactDepth — "what breaks if I change this"); AffectedTests
// are the _test.go files that define the target or any dependent. Truncated is set
// when any bounded walk was cut short. The interface path leaves the walk fields
// empty.
type GraphResult struct {
	Symbol        string     `json:"symbol"`
	Callers       []GraphRef `json:"callers"`
	Reachable     []GraphRef `json:"reachable"`
	ImpactedBy    []GraphRef `json:"impacted_by"`
	AffectedTests []string   `json:"affected_tests"`
	Truncated     bool       `json:"truncated"`
}

// newGraphResult returns a result with non-nil slices so structuredContent
// renders JSON arrays (not null) even on the empty paths.
func newGraphResult(symbol string) *GraphResult {
	return &GraphResult{
		Symbol:        symbol,
		Callers:       []GraphRef{},
		Reachable:     []GraphRef{},
		ImpactedBy:    []GraphRef{},
		AffectedTests: []string{},
	}
}

// BuildCodeGraph returns precise, type-resolved navigation for a symbol:
// who-calls and reachable-from for a function, or implementers for an
// interface. It loads the package with full type info via go/packages; when
// the package does not load or compile it falls back to the structural map
// (with a note) rather than erroring. It is never auto-run by code_map.
func BuildCodeGraph(q GraphQuery) (string, *GraphResult, error) {
	if strings.TrimSpace(q.Symbol) == "" {
		return "", nil, fmt.Errorf("symbol required")
	}
	scope := q.Scope
	if scope == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", nil, err
		}
		scope = ResolveProjectRoot(wd)
	}

	pkgs, edges, failReason := loadGraph(scope)
	if failReason != "" {
		return graphFallback(scope, q.Symbol, failReason), newGraphResult(q.Symbol), nil
	}

	// Interface query first: if the symbol names an interface, list its implementers.
	if iface, name := findInterface(pkgs, q.Symbol); iface != nil {
		return renderImplementers(pkgs, name, iface), newGraphResult(q.Symbol), nil
	}

	targets := edges.byName[q.Symbol]
	if len(targets) == 0 {
		return fmt.Sprintf("symbol %q not found as a function or interface in %s", q.Symbol, scope), newGraphResult(q.Symbol), nil
	}
	text, res := renderFunctionGraph(edges, q.Symbol, targets)
	return text, res, nil
}

// graphCacheEntry is one memoized load for a scope: the fingerprint of the
// scope's Go sources at load time, plus the loaded packages and derived call
// graph. It is reused while the fingerprint still matches.
type graphCacheEntry struct {
	fingerprint string
	pkgs        []*packages.Package
	edges       *callEdges
}

// graphCache memoizes the packages.Load + buildCallEdges result per scope for
// the lifetime of the process. code_graph runs a full type-checking load over
// ./... on every call, which is multi-second on a large module; each detritus
// consumer spawns its own stdio child, so a process-lifetime memo keyed on the
// scope's source fingerprint turns repeated queries within one session into
// cache hits while still reloading the instant any .go file changes.
var graphCache = struct {
	mu      sync.Mutex
	entries map[string]*graphCacheEntry
}{entries: map[string]*graphCacheEntry{}}

// loadGraph returns the loaded packages and the type-resolved call graph for
// scope, reusing a cached load while the scope's Go sources are unchanged.
// failReason is non-empty (and pkgs/edges nil) when the load should fall back
// to the structural map — those transient/broken states are never cached, so a
// later query re-checks once the package loads or compiles again.
func loadGraph(scope string) (pkgs []*packages.Package, edges *callEdges, failReason string) {
	fp := fingerprintScope(scope)

	graphCache.mu.Lock()
	defer graphCache.mu.Unlock()

	if fp != "" {
		if e, ok := graphCache.entries[scope]; ok && e.fingerprint == fp {
			return e.pkgs, e.edges, ""
		}
	}

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedTypes |
			packages.NeedSyntax | packages.NeedTypesInfo,
		// Tests:true loads _test.go files so test functions enter the call graph
		// (needed to compute affected tests). It yields package/test-variant
		// duplicates of the same decl, which buildCallEdges collapses by position.
		Tests: true,
		Dir:   scope,
	}
	loaded, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, nil, "package failed to load"
	}
	if len(loaded) == 0 {
		return nil, nil, "no Go packages in scope"
	}
	if pkgErrors(loaded) {
		return nil, nil, "package does not compile"
	}

	built := buildCallEdges(loaded)
	if fp != "" {
		graphCache.entries[scope] = &graphCacheEntry{fingerprint: fp, pkgs: loaded, edges: built}
	}
	return loaded, built, ""
}

// fingerprintScope hashes every Go source under scope (relative path + mtime +
// size) plus the module manifests into a stable digest, so the cache
// invalidates the moment any file is added, removed, or edited. It returns ""
// if the scope cannot be walked, which disables caching for that call rather
// than risking a stale hit.
//
// The manifests (go.mod/go.sum/go.work/go.work.sum and vendor/modules.txt) are
// folded in because packages.Load resolves imports through them: a go.mod edit
// (go get -u, a replace) or a vendored-dep update changes the loaded graph
// without touching any .go file under scope, and vendor/ is pruned by the walk
// so vendor/modules.txt would otherwise never be seen. Residual staleness
// bound: an edit *inside* a locally-replaced module's own sources lives outside
// scope and does not shift any manifest, so it still won't invalidate — a
// caller editing a replace target must restart the process.
func fingerprintScope(scope string) string {
	h := sha256.New()
	err := filepath.WalkDir(scope, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if path != scope && (name == ".git" || name == "node_modules" || name == "vendor" || name == "testdata" || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(scope, path)
		if err != nil {
			rel = path
		}
		fmt.Fprintf(h, "%s\x00%d\x00%d\n", rel, info.ModTime().UnixNano(), info.Size())
		return nil
	})
	if err != nil {
		return ""
	}
	for _, rel := range []string{"go.mod", "go.sum", "go.work", "go.work.sum", filepath.Join("vendor", "modules.txt")} {
		info, err := os.Stat(filepath.Join(scope, rel))
		if err != nil {
			continue
		}
		fmt.Fprintf(h, "%s\x00%d\x00%d\n", rel, info.ModTime().UnixNano(), info.Size())
	}
	return hex.EncodeToString(h.Sum(nil))
}

// pkgErrors reports whether any PRODUCTION package failed to load or type-check.
// Test-variant packages are deliberately excluded: with Tests:true a single
// non-compiling _test.go produces a broken test variant, and gating the whole
// graph on that would degrade who-calls/reachable/impacted-by to the structural
// fallback exactly during mid-edit scoping (plan/forge/smith), when a broken
// test file is common. affectedTests is computed best-effort from whatever test
// packages did load. The fallback fires only when production code is broken.
func pkgErrors(pkgs []*packages.Package) bool {
	bad := false
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if isTestVariant(p) {
			return
		}
		if len(p.Errors) > 0 || p.Types == nil || p.TypesInfo == nil {
			bad = true
		}
	})
	return bad
}

// isTestVariant reports whether p is a test-build variant produced by
// packages.Load(Tests:true) rather than the production package. Load tags every
// variant with a ".test" FileSet in its ID: the in-package test build
// (`foo [foo.test]`), the external test package (`foo_test [foo.test]`), and the
// synthetic test main (`foo.test`). The production package's ID is its bare
// PkgPath, so ".test]" (bracketed variants) and a ".test" suffix (synthetic
// main) uniquely mark the variants.
func isTestVariant(p *packages.Package) bool {
	return strings.HasSuffix(p.ID, ".test") || strings.Contains(p.ID, ".test]")
}

// callEdges is the type-resolved direct-call graph over the loaded packages.
// Keys are *types.Func identities (stable within one Load). Dynamic dispatch
// through interface values is not traced — a documented v1 boundary.
type callEdges struct {
	callees map[*types.Func][]*types.Func // caller → callees
	callers map[*types.Func][]*types.Func // callee → callers
	pos     map[*types.Func]token.Position
	byName  map[string][]*types.Func
}

func buildCallEdges(pkgs []*packages.Package) *callEdges {
	e := &callEdges{
		callees: map[*types.Func][]*types.Func{},
		callers: map[*types.Func][]*types.Func{},
		pos:     map[*types.Func]token.Position{},
		byName:  map[string][]*types.Func{},
	}
	// canon collapses the package/test-variant duplicates of one declaration to a
	// single representative *types.Func. packages.Load shares one FileSet across
	// all loaded packages, so a source position uniquely identifies a decl.
	canon := map[token.Position]*types.Func{}
	canonical := func(fn *types.Func, fset *token.FileSet) *types.Func {
		pos := fset.Position(fn.Pos())
		if c, ok := canon[pos]; ok {
			return c
		}
		canon[pos] = fn
		return fn
	}
	seenEdge := map[[2]*types.Func]bool{}
	for _, pkg := range pkgs {
		info := pkg.TypesInfo
		if info == nil {
			continue
		}
		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}
				self, _ := info.Defs[fd.Name].(*types.Func)
				if self == nil {
					continue
				}
				self = canonical(self, pkg.Fset)
				e.record(self, pkg.Fset)
				if fd.Body == nil {
					continue
				}
				ast.Inspect(fd.Body, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					callee := calleeFunc(info, call)
					if callee == nil {
						return true
					}
					callee = canonical(callee, pkg.Fset)
					e.record(callee, pkg.Fset)
					key := [2]*types.Func{self, callee}
					if !seenEdge[key] {
						seenEdge[key] = true
						e.callees[self] = append(e.callees[self], callee)
						e.callers[callee] = append(e.callers[callee], self)
					}
					return true
				})
			}
		}
	}
	return e
}

func (e *callEdges) record(fn *types.Func, fset *token.FileSet) {
	if _, ok := e.pos[fn]; ok {
		return
	}
	e.pos[fn] = fset.Position(fn.Pos())
	e.byName[fn.Name()] = append(e.byName[fn.Name()], fn)
}

// calleeFunc resolves a call expression's callee to a *types.Func via type
// info — handling bare calls (Foo()), package-qualified calls (pkg.Foo()), and
// method calls (x.Method()).
func calleeFunc(info *types.Info, call *ast.CallExpr) *types.Func {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		if fn, ok := info.Uses[fun].(*types.Func); ok {
			return fn
		}
	case *ast.SelectorExpr:
		if sel, ok := info.Selections[fun]; ok {
			if fn, ok := sel.Obj().(*types.Func); ok {
				return fn
			}
		}
		if fn, ok := info.Uses[fun.Sel].(*types.Func); ok {
			return fn
		}
	}
	return nil
}

func renderFunctionGraph(e *callEdges, symbol string, targets []*types.Func) (string, *GraphResult) {
	var b strings.Builder
	res := newGraphResult(symbol)

	// who-calls / reachable / impacted-by describe the PRODUCTION call graph, so
	// funcs defined in _test.go are filtered out of these lists (a test calling a
	// function is not a caller a change-impact analysis reasons about). The tests
	// that exercise the symbol are surfaced separately below as affected tests,
	// which is why the UNFILTERED impacted set is passed to affectedTests.
	callerSet := map[*types.Func]bool{}
	for _, t := range targets {
		for _, c := range e.callers[t] {
			callerSet[c] = true
		}
	}
	callers := filterOutTests(e, sortFuncs(callerSet))
	fmt.Fprintf(&b, "who-calls %s:\n", symbol)
	if len(callers) == 0 {
		b.WriteString("  (no callers in scope)\n")
	} else {
		for _, fn := range callers {
			fmt.Fprintf(&b, "  %s  (%s)\n", qualified(fn), shortPos(e.pos[fn]))
			res.Callers = append(res.Callers, graphRef(e, fn))
		}
	}

	reachAll, reachTrunc := reachableFrom(e, targets)
	reach := filterOutTests(e, reachAll)
	fmt.Fprintf(&b, "\nreachable from %s (≤%d):\n", symbol, reachableCap)
	if len(reach) == 0 {
		b.WriteString("  (calls nothing in scope)\n")
	} else {
		for _, fn := range reach {
			fmt.Fprintf(&b, "  %s  (%s)\n", qualified(fn), shortPos(e.pos[fn]))
			res.Reachable = append(res.Reachable, graphRef(e, fn))
		}
		if reachTrunc {
			fmt.Fprintf(&b, "  … truncated at %d (scope is large)\n", reachableCap)
		}
	}

	// impacted-by: who transitively REACHES the symbol — the "what breaks if I
	// change this" (blast radius) direction, walking e.callers up to impactDepth.
	impactedAll, impactTrunc := impactedBy(e, targets)
	impacted := filterOutTests(e, impactedAll)
	fmt.Fprintf(&b, "\nimpacted-by %s (≤%d):\n", symbol, impactDepth)
	if len(impacted) == 0 {
		b.WriteString("  (nothing depends on it in scope)\n")
	} else {
		for _, fn := range impacted {
			fmt.Fprintf(&b, "  %s  (%s)\n", qualified(fn), shortPos(e.pos[fn]))
			res.ImpactedBy = append(res.ImpactedBy, graphRef(e, fn))
		}
		if impactTrunc {
			fmt.Fprintf(&b, "  … truncated at depth %d (deeper dependents exist)\n", impactDepth)
		}
	}

	// affected tests: _test.go files that define the target or any dependent, so a
	// change to the symbol may break them. Uses the UNFILTERED impacted set — test
	// funcs are exactly how affected tests are discovered.
	tests := affectedTests(e, targets, impactedAll)
	res.AffectedTests = tests
	b.WriteString("\naffected tests:\n")
	if len(tests) == 0 {
		b.WriteString("  (none in scope)\n")
	} else {
		for _, f := range tests {
			fmt.Fprintf(&b, "  %s\n", f)
		}
	}

	res.Truncated = reachTrunc || impactTrunc
	return b.String(), res
}

// graphRef renders a func into a structured reference (qualified name + short pos).
func graphRef(e *callEdges, fn *types.Func) GraphRef {
	return GraphRef{Name: qualified(fn), Pos: shortPos(e.pos[fn])}
}

// isTestFunc reports whether fn is defined in a _test.go file (using the position
// already recorded in e.pos) — such funcs are kept out of the production
// who-calls/reachable/impacted-by lists but still drive affected-test discovery.
func isTestFunc(e *callEdges, fn *types.Func) bool {
	return strings.HasSuffix(filepathToSlash(e.pos[fn].Filename), "_test.go")
}

// filterOutTests returns fns with every _test.go-defined func removed, preserving
// order. Used to keep test functions out of the rendered + structured
// who-calls/reachable/impacted-by lists.
func filterOutTests(e *callEdges, fns []*types.Func) []*types.Func {
	out := make([]*types.Func, 0, len(fns))
	for _, fn := range fns {
		if isTestFunc(e, fn) {
			continue
		}
		out = append(out, fn)
	}
	return out
}

func reachableFrom(e *callEdges, targets []*types.Func) ([]*types.Func, bool) {
	visited := map[*types.Func]bool{}
	for _, t := range targets {
		visited[t] = true
	}
	queue := append([]*types.Func{}, targets...)
	var out []*types.Func
	truncated := false
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, callee := range e.callees[cur] {
			if visited[callee] {
				continue
			}
			visited[callee] = true
			if len(out) >= reachableCap {
				truncated = true
				continue
			}
			out = append(out, callee)
			queue = append(queue, callee)
		}
	}
	sort.Slice(out, func(i, j int) bool { return qualified(out[i]) < qualified(out[j]) })
	return out, truncated
}

// impactedBy walks the REVERSE call graph (e.callers) transitively from targets,
// bounded to impactDepth hops, to answer "what breaks if I change this". It
// mirrors reachableFrom but follows callers instead of callees. Returns the
// dependent set (excluding the targets) and whether the walk was cut short by
// depth or the reachableCap item bound — truncation is always surfaced, never
// silent.
func impactedBy(e *callEdges, targets []*types.Func) ([]*types.Func, bool) {
	visited := map[*types.Func]bool{}
	for _, t := range targets {
		visited[t] = true
	}
	type item struct {
		fn    *types.Func
		depth int
	}
	queue := make([]item, 0, len(targets))
	for _, t := range targets {
		queue = append(queue, item{fn: t, depth: 0})
	}
	var out []*types.Func
	prodCount := 0 // only production dependents count toward reachableCap
	truncated := false
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.depth >= impactDepth {
			// A caller beyond the depth bound exists but is not expanded.
			for _, caller := range e.callers[cur.fn] {
				if !visited[caller] {
					truncated = true
					break
				}
			}
			continue
		}
		for _, caller := range e.callers[cur.fn] {
			if visited[caller] {
				continue
			}
			visited[caller] = true
			// Only production dependents count toward the cap. Test funcs are
			// still collected (affectedTests needs them) but never consume the
			// budget, so they can't crowd real dependents out of the blast radius.
			if !isTestFunc(e, caller) {
				if prodCount >= reachableCap {
					truncated = true
					continue
				}
				prodCount++
			}
			out = append(out, caller)
			queue = append(queue, item{fn: caller, depth: cur.depth + 1})
		}
	}
	sort.Slice(out, func(i, j int) bool { return qualified(out[i]) < qualified(out[j]) })
	return out, truncated
}

// affectedTests returns the _test.go files that define the target or any of its
// (bounded) dependents — tests that transitively exercise the symbol and so may
// break when it changes. Uses the definition positions already recorded in e.pos.
func affectedTests(e *callEdges, targets, impacted []*types.Func) []string {
	seen := map[string]bool{}
	files := []string{}
	add := func(fn *types.Func) {
		p := e.pos[fn]
		if p.Filename == "" {
			return
		}
		f := filepathToSlash(p.Filename)
		if !strings.HasSuffix(f, "_test.go") || seen[f] {
			return
		}
		seen[f] = true
		files = append(files, f)
	}
	for _, t := range targets {
		add(t)
	}
	for _, fn := range impacted {
		add(fn)
	}
	sort.Strings(files)
	return files
}

func findInterface(pkgs []*packages.Package, symbol string) (*types.Interface, string) {
	for _, pkg := range pkgs {
		if pkg.Types == nil {
			continue
		}
		obj := pkg.Types.Scope().Lookup(symbol)
		tn, ok := obj.(*types.TypeName)
		if !ok {
			continue
		}
		if iface, ok := tn.Type().Underlying().(*types.Interface); ok {
			return iface, pkg.Types.Name() + "." + symbol
		}
	}
	return nil, ""
}

func renderImplementers(pkgs []*packages.Package, name string, iface *types.Interface) string {
	type impl struct {
		name string
		pos  token.Position
	}
	var impls []impl
	seen := map[string]bool{}
	for _, pkg := range pkgs {
		if pkg.Types == nil {
			continue
		}
		scope := pkg.Types.Scope()
		for _, n := range scope.Names() {
			tn, ok := scope.Lookup(n).(*types.TypeName)
			if !ok {
				continue
			}
			named, ok := tn.Type().(*types.Named)
			if !ok {
				continue
			}
			if _, isIface := named.Underlying().(*types.Interface); isIface {
				continue
			}
			if !types.Implements(named, iface) && !types.Implements(types.NewPointer(named), iface) {
				continue
			}
			full := pkg.Types.Name() + "." + n
			if seen[full] {
				continue
			}
			seen[full] = true
			impls = append(impls, impl{name: full, pos: pkg.Fset.Position(tn.Pos())})
		}
	}
	sort.Slice(impls, func(i, j int) bool { return impls[i].name < impls[j].name })

	var b strings.Builder
	fmt.Fprintf(&b, "implementers of %s:\n", name)
	if len(impls) == 0 {
		b.WriteString("  (none in scope)\n")
		return b.String()
	}
	for _, im := range impls {
		fmt.Fprintf(&b, "  %s  (%s)\n", im.name, shortPos(im.pos))
	}
	return b.String()
}

func graphFallback(scope, symbol, reason string) string {
	m, _ := BuildCodeMap(MapOptions{Scope: scope, Focus: []string{symbol}})
	return fmt.Sprintf("(code_graph: %s — falling back to the structural map)\n\n%s", reason, m)
}

func sortFuncs(set map[*types.Func]bool) []*types.Func {
	out := make([]*types.Func, 0, len(set))
	for fn := range set {
		out = append(out, fn)
	}
	sort.Slice(out, func(i, j int) bool { return qualified(out[i]) < qualified(out[j]) })
	return out
}

// qualified renders a func as pkg.Name or pkg.(Recv).Method for stable output.
func qualified(fn *types.Func) string {
	pkgName := ""
	if fn.Pkg() != nil {
		pkgName = fn.Pkg().Name() + "."
	}
	if recv := fn.Type().(*types.Signature).Recv(); recv != nil {
		return fmt.Sprintf("%s(%s).%s", pkgName, recvTypeName(recv.Type()), fn.Name())
	}
	return pkgName + fn.Name()
}

func recvTypeName(t types.Type) string {
	if p, ok := t.(*types.Pointer); ok {
		return "*" + recvTypeName(p.Elem())
	}
	if n, ok := t.(*types.Named); ok {
		return n.Obj().Name()
	}
	return t.String()
}

func shortPos(p token.Position) string {
	if p.Filename == "" {
		return "?"
	}
	return fmt.Sprintf("%s:%d", lastSegment(filepathToSlash(p.Filename)), p.Line)
}

func filepathToSlash(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}
