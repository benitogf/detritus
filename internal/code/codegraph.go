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

// GraphQuery configures a code_graph navigation.
type GraphQuery struct {
	Symbol string // a function or interface name to navigate
	Scope  string // dir to load; empty → the current project
}

// BuildCodeGraph returns precise, type-resolved navigation for a symbol:
// who-calls and reachable-from for a function, or implementers for an
// interface. It loads the package with full type info via go/packages; when
// the package does not load or compile it falls back to the structural map
// (with a note) rather than erroring. It is never auto-run by code_map.
func BuildCodeGraph(q GraphQuery) (string, error) {
	if strings.TrimSpace(q.Symbol) == "" {
		return "", fmt.Errorf("symbol required")
	}
	scope := q.Scope
	if scope == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		scope = ResolveProjectRoot(wd)
	}

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedTypes |
			packages.NeedSyntax | packages.NeedTypesInfo,
		Dir: scope,
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return graphFallback(scope, q.Symbol, "package failed to load"), nil
	}
	if len(pkgs) == 0 {
		return graphFallback(scope, q.Symbol, "no Go packages in scope"), nil
	}
	if pkgErrors(pkgs) {
		return graphFallback(scope, q.Symbol, "package does not compile"), nil
	}

	// Interface query first: if the symbol names an interface, list its implementers.
	if iface, name := findInterface(pkgs, q.Symbol); iface != nil {
		return renderImplementers(pkgs, name, iface), nil
	}

	edges := buildCallEdges(pkgs)
	targets := edges.byName[q.Symbol]
	if len(targets) == 0 {
		return fmt.Sprintf("symbol %q not found as a function or interface in %s", q.Symbol, scope), nil
	}
	return renderFunctionGraph(edges, q.Symbol, targets), nil
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
	seenEdge := map[[2]*types.Func]bool{}
	for _, pkg := range pkgs {
		info := pkg.TypesInfo
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

func renderFunctionGraph(e *callEdges, symbol string, targets []*types.Func) string {
	var b strings.Builder

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
		}
	}

	reach, truncated := reachableFrom(e, targets)
	fmt.Fprintf(&b, "\nreachable from %s (≤%d):\n", symbol, reachableCap)
	if len(reach) == 0 {
		b.WriteString("  (calls nothing in scope)\n")
	} else {
		for _, fn := range reach {
			fmt.Fprintf(&b, "  %s  (%s)\n", qualified(fn), shortPos(e.pos[fn]))
		}
		if truncated {
			fmt.Fprintf(&b, "  … truncated at %d (scope is large)\n", reachableCap)
		}
	}
	return b.String()
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
