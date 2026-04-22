package code

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"sort"
	"strings"
)

// Outline returns a compact, signature-only view of a source file.
// Language dispatch uses the info's Language field (set by detectLanguage).
func Outline(language string, content []byte) string {
	switch language {
	case "go":
		return outlineGo(content)
	case "typescript", "javascript":
		return outlineByRegex(content, regexJS)
	case "python":
		return outlineByRegex(content, regexPy)
	case "rust":
		return outlineByRegex(content, regexRust)
	case "java", "csharp":
		return outlineByRegex(content, regexJavaC)
	case "c", "cpp":
		return outlineByRegex(content, regexCFamily)
	default:
		return ""
	}
}

func outlineGo(content []byte) string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", content, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "package %s\n", f.Name.Name)
	if len(f.Imports) > 0 {
		b.WriteString("\nimports:\n")
		for _, imp := range f.Imports {
			fmt.Fprintf(&b, "  %s\n", imp.Path.Value)
		}
	}
	var types []string
	var funcs []string
	var vars []string
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					types = append(types, formatTypeSpec(s))
				case *ast.ValueSpec:
					for _, n := range s.Names {
						if !n.IsExported() {
							continue
						}
						kind := "var"
						if d.Tok == token.CONST {
							kind = "const"
						}
						vars = append(vars, fmt.Sprintf("%s %s", kind, n.Name))
					}
				}
			}
		case *ast.FuncDecl:
			funcs = append(funcs, formatFuncDecl(d, fset, content))
		}
	}
	if len(types) > 0 {
		b.WriteString("\ntypes:\n")
		for _, t := range types {
			fmt.Fprintf(&b, "  %s\n", t)
		}
	}
	if len(vars) > 0 {
		b.WriteString("\nvalues:\n")
		for _, v := range vars {
			fmt.Fprintf(&b, "  %s\n", v)
		}
	}
	if len(funcs) > 0 {
		b.WriteString("\nfuncs:\n")
		for _, fn := range funcs {
			fmt.Fprintf(&b, "  %s\n", fn)
		}
	}
	return b.String()
}

func formatTypeSpec(s *ast.TypeSpec) string {
	switch t := s.Type.(type) {
	case *ast.StructType:
		return fmt.Sprintf("type %s struct { /* %d fields */ }", s.Name.Name, len(t.Fields.List))
	case *ast.InterfaceType:
		return fmt.Sprintf("type %s interface { /* %d methods */ }", s.Name.Name, len(t.Methods.List))
	default:
		return fmt.Sprintf("type %s", s.Name.Name)
	}
}

func formatFuncDecl(d *ast.FuncDecl, fset *token.FileSet, src []byte) string {
	start := fset.Position(d.Pos()).Offset
	var end int
	if d.Body != nil {
		end = fset.Position(d.Body.Lbrace).Offset
	} else {
		end = fset.Position(d.End()).Offset
	}
	if start < 0 || end > len(src) || start >= end {
		if d.Name != nil {
			return d.Name.Name
		}
		return "?"
	}
	sig := string(src[start:end])
	sig = strings.TrimSpace(sig)
	return sig
}

// regex-based signature extraction for non-Go languages.
// Patterns capture common declaration forms; not AST-perfect, but cheap.

var (
	regexJS = []*regexp.Regexp{
		regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s+\w+\s*\([^)]*\)`),
		regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:abstract\s+)?class\s+\w+(?:\s+extends\s+\S+)?(?:\s+implements\s+[^{]+)?`),
		regexp.MustCompile(`(?m)^\s*(?:export\s+)?interface\s+\w+(?:\s+extends\s+[^{]+)?`),
		regexp.MustCompile(`(?m)^\s*(?:export\s+)?type\s+\w+\s*=`),
		regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:const|let|var)\s+\w+\s*(?::|=\s*(?:async\s+)?(?:function|\([^)]*\)\s*=>))`),
	}
	regexPy = []*regexp.Regexp{
		regexp.MustCompile(`(?m)^\s*(?:async\s+)?def\s+\w+\s*\([^)]*\)\s*(?:->\s*[^:]+)?:`),
		regexp.MustCompile(`(?m)^\s*class\s+\w+(?:\([^)]*\))?:`),
	}
	regexRust = []*regexp.Regexp{
		regexp.MustCompile(`(?m)^\s*(?:pub\s+)?(?:async\s+)?fn\s+\w+[^{;]*`),
		regexp.MustCompile(`(?m)^\s*(?:pub\s+)?struct\s+\w+[^{;]*`),
		regexp.MustCompile(`(?m)^\s*(?:pub\s+)?enum\s+\w+[^{;]*`),
		regexp.MustCompile(`(?m)^\s*(?:pub\s+)?trait\s+\w+[^{;]*`),
		regexp.MustCompile(`(?m)^\s*impl(?:\s*<[^>]+>)?\s+[^{;]+`),
	}
	regexJavaC = []*regexp.Regexp{
		regexp.MustCompile(`(?m)^\s*(?:public|private|protected|internal)?\s*(?:static\s+)?(?:abstract\s+)?(?:class|interface|enum|record)\s+\w+[^{;]*`),
		regexp.MustCompile(`(?m)^\s*(?:public|private|protected|internal)\s+(?:static\s+)?(?:async\s+)?[\w<>\[\],\s]+\s+\w+\s*\([^)]*\)`),
	}
	regexCFamily = []*regexp.Regexp{
		regexp.MustCompile(`(?m)^\s*(?:static\s+|inline\s+|extern\s+)*[\w*&<>:\s]+\s+\w+\s*\([^)]*\)\s*(?:const\s*)?(?:override\s*)?(?:\{|;)`),
		regexp.MustCompile(`(?m)^\s*(?:class|struct|union|enum)\s+\w+[^;]*`),
		regexp.MustCompile(`(?m)^\s*typedef\s+[^;]+;`),
	}
)

func outlineByRegex(content []byte, patterns []*regexp.Regexp) string {
	seen := map[int]bool{}
	type match struct {
		start int
		text  string
	}
	var matches []match
	for _, pat := range patterns {
		for _, idx := range pat.FindAllIndex(content, -1) {
			if seen[idx[0]] {
				continue
			}
			seen[idx[0]] = true
			text := string(bytes.TrimSpace(content[idx[0]:idx[1]]))
			text = strings.TrimSuffix(text, "{")
			text = strings.TrimSuffix(text, "=")
			text = strings.TrimRight(text, " \t")
			matches = append(matches, match{start: idx[0], text: text})
		}
	}
	if len(matches) == 0 {
		return ""
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].start < matches[j].start })
	var b strings.Builder
	for _, m := range matches {
		b.WriteString(m.text)
		b.WriteByte('\n')
	}
	return b.String()
}
