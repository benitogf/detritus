package code

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"time"

	ignore "github.com/sabhiram/go-gitignore"
)

const (
	defaultMaxFileSize int64 = 1 << 20 // 1 MB
	binarySniffSize          = 8 << 10 // 8 KB
)

// FileInfo describes a single file found during a walk.
type FileInfo struct {
	Root     string    // absolute path to the pack root this file lives under
	PathRel  string    // slash-separated path inside the root
	Size     int64
	MTime    time.Time
	Language string
}

// ID returns the stable document ID: "<root>|<path_rel>".
// The `|` separator avoids confusion with forward slashes already inside roots/paths.
func (f FileInfo) ID() string {
	return f.Root + "|" + f.PathRel
}

// Skipped describes a file that was deliberately not indexed.
type Skipped struct {
	Path   string // root-relative for readability
	Reason string
}

// WalkResult is returned by Walk.
type WalkResult struct {
	Files   []FileInfo
	Skipped []Skipped
}

// defaultIgnores are directory or file patterns always excluded regardless of .gitignore.
var defaultIgnores = []string{
	".git",
	"node_modules",
	"vendor",
	".detritus",
	"package-lock.json",
	"yarn.lock",
	"pnpm-lock.yaml",
	"Cargo.lock",
	"go.sum",
	"*.min.js",
	"*.min.css",
}

// Walk enumerates files under each root, honouring .gitignore and defaults.
// Roots must be absolute paths.
func Walk(roots []string) (*WalkResult, error) {
	out := &WalkResult{}
	for _, root := range roots {
		abs, err := filepath.Abs(root)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(abs)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, &os.PathError{Op: "walk", Path: abs, Err: os.ErrInvalid}
		}
		ig := loadGitignore(abs)
		if err := walkRoot(abs, ig, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func walkRoot(root string, ig *ignore.GitIgnore, out *WalkResult) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		if relSlash == "." {
			return nil
		}
		if matchAnyDefault(relSlash, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if ig != nil && ig.MatchesPath(relSlash) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if info.Size() > defaultMaxFileSize {
			out.Skipped = append(out.Skipped, Skipped{Path: relSlash, Reason: "size cap"})
			return nil
		}
		isBin, bErr := isBinary(path)
		if bErr != nil || isBin {
			if isBin {
				out.Skipped = append(out.Skipped, Skipped{Path: relSlash, Reason: "binary"})
			}
			return nil
		}
		out.Files = append(out.Files, FileInfo{
			Root:     root,
			PathRel:  relSlash,
			Size:     info.Size(),
			MTime:    info.ModTime(),
			Language: detectLanguage(relSlash),
		})
		return nil
	})
}

// matchAnyDefault returns true if relSlash matches any default-ignore pattern.
// Directory matches are prefix-aware (e.g. "node_modules/..." matches "node_modules").
func matchAnyDefault(relSlash string, isDir bool) bool {
	base := filepath.Base(relSlash)
	for _, pat := range defaultIgnores {
		if strings.Contains(pat, "*") {
			if ok, _ := filepath.Match(pat, base); ok {
				return true
			}
			continue
		}
		if base == pat {
			return true
		}
	}
	_ = isDir
	return false
}

func loadGitignore(root string) *ignore.GitIgnore {
	p := filepath.Join(root, ".gitignore")
	if _, err := os.Stat(p); err != nil {
		return nil
	}
	ig, err := ignore.CompileIgnoreFile(p)
	if err != nil {
		return nil
	}
	return ig
}

// isBinary returns true if the first 8KB contain a NUL byte.
func isBinary(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	buf := make([]byte, binarySniffSize)
	n, _ := f.Read(buf)
	return bytes.IndexByte(buf[:n], 0) >= 0, nil
}

// detectLanguage returns a coarse language label from a file's extension.
func detectLanguage(relSlash string) string {
	ext := strings.ToLower(filepath.Ext(relSlash))
	switch ext {
	case ".go":
		return "go"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".hpp":
		return "cpp"
	case ".cs":
		return "csharp"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".swift":
		return "swift"
	case ".kt", ".kts":
		return "kotlin"
	case ".md", ".markdown":
		return "markdown"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	case ".sh", ".bash":
		return "shell"
	case ".html", ".htm":
		return "html"
	case ".css", ".scss", ".sass":
		return "css"
	case ".sql":
		return "sql"
	default:
		return ""
	}
}
