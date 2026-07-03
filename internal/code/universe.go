package code

import (
	"go/types"
	"os"
	"path"
	"strings"

	"golang.org/x/tools/go/packages"
)

// SymbolUniverse is the ground truth a staleness check queries: every identifier
// name and every source file that exists in a loaded Go tree. It is built by
// reusing the same go/packages load code_graph uses, so it inherits the same
// type-resolution — no separate AST loader. It never carries an error: an
// unloadable / non-Go dir simply yields Loaded=false, so the caller treats every
// reference as "unknown" rather than crashing (a load failure must never be read
// as "the symbol is dead").
type SymbolUniverse struct {
	// Loaded is true when at least one Go package was loaded at the dir — i.e.
	// this really is a Go tree, so a missing source file is a meaningful signal.
	Loaded bool
	// Typed is true when the tree loaded AND type-checked clean (no package
	// errors). Only then are symbol judgments trustworthy; a tree that doesn't
	// compile makes every "not found" ambiguous, so symbols stay "unknown".
	Typed bool

	names map[string]struct{} // defined identifier names: module + deps + stdlib + predeclared universe
	files map[string]struct{} // slash-separated relative paths of every source file under the tree
	bases map[string]struct{} // basenames of those files (for bare `foo.go` refs with no directory)
}

// LoadSymbolUniverse loads the Go tree at dir and returns the set of identifiers
// and files that exist in it. dir is resolved to its project root (nearest
// go.mod/.git/marker) exactly like code_graph; an empty dir resolves from the
// current working directory. It never returns an error — a non-Go or unloadable
// dir yields a universe with Loaded=false.
func LoadSymbolUniverse(dir string) *SymbolUniverse {
	u := &SymbolUniverse{
		names: map[string]struct{}{},
		files: map[string]struct{}{},
		bases: map[string]struct{}{},
	}
	scope := dir
	if strings.TrimSpace(scope) == "" {
		wd, err := os.Getwd()
		if err != nil {
			return u
		}
		scope = wd
	}
	scope = ResolveProjectRoot(scope)

	// File index is filesystem-only — independent of whether the tree compiles —
	// so a missing source file can be judged unambiguously.
	if wr, err := Walk([]string{scope}); err == nil {
		for _, f := range wr.Files {
			u.files[f.PathRel] = struct{}{}
			u.bases[path.Base(f.PathRel)] = struct{}{}
		}
	}

	cfg := &packages.Config{
		// Same load as code_graph: full type info + deps so pkgErrors is
		// meaningful and identifiers from imported and standard-library packages
		// (os.Getwd, strings.Fields, …) are known-alive too, not just the
		// module's own.
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedTypes |
			packages.NeedSyntax | packages.NeedTypesInfo,
		Tests: true,
		Dir:   scope,
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil || !hasRealGoPackage(pkgs) {
		return u // not a loadable Go tree → Loaded stays false → refs are "unknown"
	}
	u.Loaded = true
	u.Typed = !pkgErrors(pkgs)

	// Seed the predeclared universe (builtins like make/len/append/new + nil,
	// true, error, int, …) so a backticked `make()` in a lesson is never flagged
	// dead. Then collect every package-level name and method across the whole
	// loaded universe.
	for _, n := range types.Universe.Names() {
		u.names[n] = struct{}{}
	}
	seen := map[*types.Package]bool{}
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if p.Types == nil || seen[p.Types] {
			return
		}
		seen[p.Types] = true
		scope := p.Types.Scope()
		for _, n := range scope.Names() {
			u.names[n] = struct{}{}
			collectMethods(u.names, scope.Lookup(n))
		}
	})
	return u
}

// hasRealGoPackage reports whether at least one loaded top-level package
// actually has Go source. packages.Load returns a synthetic error-package (with
// no GoFiles) for a dir that isn't a Go module, which must NOT count as a loaded
// tree — otherwise a non-Go dir would be judged instead of reported "unknown".
func hasRealGoPackage(pkgs []*packages.Package) bool {
	for _, p := range pkgs {
		if len(p.GoFiles) > 0 || len(p.CompiledGoFiles) > 0 {
			return true
		}
	}
	return false
}

// collectMethods adds the method names of a named type (concrete methods and,
// for an interface, its interface methods) so a `T.Method` reference resolves.
func collectMethods(names map[string]struct{}, obj types.Object) {
	tn, ok := obj.(*types.TypeName)
	if !ok {
		return
	}
	named, ok := tn.Type().(*types.Named)
	if !ok {
		return
	}
	for i := 0; i < named.NumMethods(); i++ {
		names[named.Method(i).Name()] = struct{}{}
	}
	if iface, ok := named.Underlying().(*types.Interface); ok {
		for i := 0; i < iface.NumMethods(); i++ {
			names[iface.Method(i).Name()] = struct{}{}
		}
	}
}

// HasSymbol reports whether a symbol reference plausibly still exists. It is
// deliberately lenient to avoid false positives: the ref is split on "." and any
// component that is a known identifier counts as alive (so `pkg.Foo` survives if
// either the package name or Foo exists anywhere in the loaded universe). Only a
// ref whose every dotted component is defined nowhere is reported dead. A trailing
// "()" is ignored. Call only when Typed is true; otherwise the result is unsafe.
func (u *SymbolUniverse) HasSymbol(ref string) bool {
	ref = strings.TrimSuffix(strings.TrimSpace(ref), "()")
	if ref == "" {
		return true // nothing to judge → do not flag
	}
	for _, seg := range strings.Split(ref, ".") {
		if seg == "" {
			continue
		}
		if _, ok := u.names[seg]; ok {
			return true
		}
	}
	return false
}

// HasFile reports whether a source-file reference still resolves under the tree.
// A ref containing a directory is matched as a path suffix (so
// `internal/code/store.go` matches the real file wherever it sits under the
// root); a bare basename is matched against any file's basename. Only a ref that
// matches nothing is reported dead. Call only when Loaded is true.
func (u *SymbolUniverse) HasFile(ref string) bool {
	ref = strings.TrimPrefix(strings.ReplaceAll(strings.TrimSpace(ref), "\\", "/"), "./")
	if ref == "" {
		return true
	}
	if _, ok := u.files[ref]; ok {
		return true
	}
	if !strings.Contains(ref, "/") {
		_, ok := u.bases[ref]
		return ok
	}
	suffix := "/" + ref
	for f := range u.files {
		if strings.HasSuffix(f, suffix) {
			return true
		}
	}
	return false
}
