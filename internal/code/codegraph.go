package code

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

const reachableCap = 100 // bound the reachable-from BFS; truncation is noted, never silent
const impactDepth = 3    // bound the impacted-by (reverse-transitive) walk; truncation is noted, never silent

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
// outputSchema + structuredContent (mirrors kb_search/skill_search). Callers is
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
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return graphFallback(scope, q.Symbol, "package failed to load"), newGraphResult(q.Symbol), nil
	}
	if len(pkgs) == 0 {
		return graphFallback(scope, q.Symbol, "no Go packages in scope"), newGraphResult(q.Symbol), nil
	}
	if pkgErrors(pkgs) {
		return graphFallback(scope, q.Symbol, "package does not compile"), newGraphResult(q.Symbol), nil
	}

	// Interface query first: if the symbol names an interface, list its implementers.
	if iface, name := findInterface(pkgs, q.Symbol); iface != nil {
		return renderImplementers(pkgs, name, iface), newGraphResult(q.Symbol), nil
	}

	edges := buildCallEdges(pkgs)
	targets := edges.byName[q.Symbol]
	if len(targets) == 0 {
		return fmt.Sprintf("symbol %q not found as a function or interface in %s", q.Symbol, scope), newGraphResult(q.Symbol), nil
	}
	text, res := renderFunctionGraph(edges, q.Symbol, targets)
	return text, res, nil
}

func pkgErrors(pkgs []*packages.Package) bool {
	bad := false
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if len(p.Errors) > 0 || p.Types == nil || p.TypesInfo == nil {
			bad = true
		}
	})
	return bad
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

	callerSet := map[*types.Func]bool{}
	for _, t := range targets {
		for _, c := range e.callers[t] {
			callerSet[c] = true
		}
	}
	fmt.Fprintf(&b, "who-calls %s:\n", symbol)
	if len(callerSet) == 0 {
		b.WriteString("  (no callers in scope)\n")
	} else {
		for _, fn := range sortFuncs(callerSet) {
			fmt.Fprintf(&b, "  %s  (%s)\n", qualified(fn), shortPos(e.pos[fn]))
			res.Callers = append(res.Callers, graphRef(e, fn))
		}
	}

	reach, reachTrunc := reachableFrom(e, targets)
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
	impacted, impactTrunc := impactedBy(e, targets)
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
	// change to the symbol may break them.
	tests := affectedTests(e, targets, impacted)
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
			if len(out) >= reachableCap {
				truncated = true
				continue
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
