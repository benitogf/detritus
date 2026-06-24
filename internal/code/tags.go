package code

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
)

// Tag is one symbol occurrence in a source file. Kind is "def" (a declaration)
// or "ref" (a use of an identifier defined elsewhere). Signature is populated
// for defs only. These tags are the input the PageRank structural map consumes.
type Tag struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"` // "def" | "ref"
	Line      int    `json:"line"`
	Signature string `json:"sig,omitempty"`
}

// FileTags is the cached parse result for one source file. mtime+size key the
// cache: an entry is valid only while both match the file on disk.
type FileTags struct {
	Path      string `json:"path"`
	MTimeNano int64  `json:"mtime"`
	Size      int64  `json:"size"`
	Tags      []Tag  `json:"tags"`
}

// CodeTagsDir is the per-file parse cache root: ~/.detritus/code-tags/.
// Derived, disposable, outside any project repo, never committed.
func CodeTagsDir() string {
	return filepath.Join(DataDir(), "code-tags")
}

// TagsForFile returns the def/ref tags for a Go file, re-parsing only when the
// file's mtime or size has changed since the last parse. Unchanged files are
// served from the on-disk cache, so the first call over a project pays the
// parse cost once and later calls (and later sessions) are incremental.
func TagsForFile(absPath string) ([]Tag, error) {
	fi, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}
	mtime, size := fi.ModTime().UnixNano(), fi.Size()
	if cached, ok := loadCachedTags(absPath); ok && cached.MTimeNano == mtime && cached.Size == size {
		return cached.Tags, nil
	}
	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	tags := extractGoTags(content)
	storeCachedTags(FileTags{Path: absPath, MTimeNano: mtime, Size: size, Tags: tags})
	return tags, nil
}

// extractGoTags parses Go source and returns its def tags (top-level decls and
// methods, with signatures) and ref tags (identifiers used via selectors or
// bare calls). Non-Go or unparseable content yields no tags.
func extractGoTags(content []byte) []Tag {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", content, parser.SkipObjectResolution)
	if err != nil {
		return nil
	}
	line := func(p token.Pos) int { return fset.Position(p).Line }

	var tags []Tag
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			tags = append(tags, Tag{Name: d.Name.Name, Kind: "def", Line: line(d.Pos()), Signature: formatFuncDecl(d, fset, content)})
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					tags = append(tags, Tag{Name: s.Name.Name, Kind: "def", Line: line(s.Pos()), Signature: formatTypeSpec(s)})
				case *ast.ValueSpec:
					kind := "var"
					if d.Tok == token.CONST {
						kind = "const"
					}
					for _, n := range s.Names {
						tags = append(tags, Tag{Name: n.Name, Kind: "def", Line: line(n.Pos()), Signature: kind + " " + n.Name})
					}
				}
			}
		}
	}

	// Refs: selector targets (pkg.Foo / x.Method → "Foo"/"Method") and bare
	// call identifiers (Foo() → "Foo"). First-seen line per name; this is the
	// edge source for the file→file reference graph.
	refLine := map[string]int{}
	addRef := func(name string, p token.Pos) {
		if _, seen := refLine[name]; !seen {
			refLine[name] = line(p)
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			addRef(x.Sel.Name, x.Sel.Pos())
		case *ast.CallExpr:
			if id, ok := x.Fun.(*ast.Ident); ok {
				addRef(id.Name, id.Pos())
			}
		}
		return true
	})
	for name, ln := range refLine {
		tags = append(tags, Tag{Name: name, Kind: "ref", Line: ln})
	}
	return tags
}

// cacheFileFor maps an absolute source path to its cache file. The path is
// hashed so one source file = one cache file (no whole-cache rewrites, no
// concurrent-write corruption — a torn write just forces a re-parse).
func cacheFileFor(absPath string) string {
	sum := sha256.Sum256([]byte(absPath))
	return filepath.Join(CodeTagsDir(), hex.EncodeToString(sum[:])+".json")
}

func loadCachedTags(absPath string) (FileTags, bool) {
	data, err := os.ReadFile(cacheFileFor(absPath))
	if err != nil {
		return FileTags{}, false
	}
	var ft FileTags
	if err := json.Unmarshal(data, &ft); err != nil {
		return FileTags{}, false
	}
	return ft, true
}

// storeCachedTags writes the entry atomically (temp + rename) so a concurrent
// reader never sees a half-written file. Failures are silent: the cache is a
// derived optimization, and a miss just re-parses next time.
func storeCachedTags(ft FileTags) {
	if err := os.MkdirAll(CodeTagsDir(), 0o755); err != nil {
		return
	}
	data, err := json.Marshal(ft)
	if err != nil {
		return
	}
	dst := cacheFileFor(ft.Path)
	tmp, err := os.CreateTemp(CodeTagsDir(), "tags-*.tmp")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return
	}
	if err := os.Rename(tmpName, dst); err != nil {
		os.Remove(tmpName)
	}
}
