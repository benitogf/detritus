package code

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
)

// SchemaVersion changes when the index field schema or analyzer changes in a
// backwards-incompatible way. On mismatch the pack is fully rebuilt.
const SchemaVersion = 1

// Manifest is the pack's metadata record.
type Manifest struct {
	SchemaVersion int       `json:"schema_version"`
	Version       string    `json:"version"`
	Name          string    `json:"name"`
	Roots         []string  `json:"roots"`
	PackedAt      time.Time `json:"packed_at"`
	FileCount     int       `json:"file_count"`
	TotalBytes    int64     `json:"total_bytes"`
	TotalTokens   int       `json:"total_tokens"`
	Skipped       []Skipped `json:"skipped,omitempty"`
}

// PackStats summarizes a pack/refresh run.
type PackStats struct {
	New       int
	Modified  int
	Deleted   int
	Unchanged int
	Files     int
	Bytes     int64
	Tokens    int
	Duration  time.Duration
}

// Options controls pack behaviour.
type Options struct {
	// Rebuild forces a full re-index even if the manifest matches.
	Rebuild bool
	// DetritusVersion is recorded in the manifest.
	DetritusVersion string
}

// Pack creates or incrementally refreshes a named pack.
// roots must be absolute paths. If the pack exists and roots is nil,
// the previous manifest's roots are reused.
func Pack(name string, roots []string, opts Options) (*PackStats, error) {
	if err := ValidatePackName(name); err != nil {
		return nil, err
	}
	if err := EnsurePacksDir(); err != nil {
		return nil, fmt.Errorf("ensure packs dir: %w", err)
	}

	existing, err := LoadManifest(name)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load manifest: %w", err)
	}

	if len(roots) == 0 {
		if existing == nil {
			return nil, fmt.Errorf("pack %q does not exist and no roots given", name)
		}
		roots = existing.Roots
	}

	absRoots, err := absolutizeAll(roots)
	if err != nil {
		return nil, err
	}

	schemaMismatch := existing != nil && existing.SchemaVersion != SchemaVersion
	fullRebuild := opts.Rebuild || existing == nil || schemaMismatch ||
		!sameRootSet(existing.Roots, absRoots)

	start := time.Now()
	stats := &PackStats{}

	walkRes, err := Walk(absRoots)
	if err != nil {
		return nil, fmt.Errorf("walk: %w", err)
	}

	dir := PackDir(name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir pack: %w", err)
	}

	idxPath := IndexPath(name)
	if fullRebuild {
		_ = os.RemoveAll(idxPath)
	}

	index, err := openOrCreateIndex(idxPath)
	if err != nil {
		return nil, fmt.Errorf("open index: %w", err)
	}
	defer index.Close()

	var previous map[string]prevEntry
	if !fullRebuild {
		previous, err = enumerateIndex(index)
		if err != nil {
			return nil, fmt.Errorf("enumerate previous state: %w", err)
		}
	}

	batch := index.NewBatch()
	batched := 0
	flush := func() error {
		if batched == 0 {
			return nil
		}
		if err := index.Batch(batch); err != nil {
			return err
		}
		batch = index.NewBatch()
		batched = 0
		return nil
	}

	currentIDs := map[string]struct{}{}
	for _, f := range walkRes.Files {
		id := f.ID()
		if !fullRebuild {
			if p, ok := previous[id]; ok && p.size == f.Size && p.mtime == f.MTime.Unix() {
				currentIDs[id] = struct{}{}
				stats.Unchanged++
				stats.Files++
				stats.Bytes += f.Size
				continue
			}
		}
		content, err := os.ReadFile(filepath.Join(f.Root, f.PathRel))
		if err != nil {
			walkRes.Skipped = append(walkRes.Skipped, Skipped{Path: f.PathRel, Reason: "read error: " + err.Error()})
			continue
		}
		currentIDs[id] = struct{}{}
		doc := buildDoc(f, content)
		if err := batch.Index(id, doc); err != nil {
			return nil, fmt.Errorf("batch index: %w", err)
		}
		if !fullRebuild {
			if _, existed := previous[id]; existed {
				stats.Modified++
			} else {
				stats.New++
			}
		} else {
			stats.New++
		}
		stats.Files++
		stats.Bytes += f.Size
		batched++
		if batched >= 200 {
			if err := flush(); err != nil {
				return nil, fmt.Errorf("flush batch: %w", err)
			}
		}
	}

	if !fullRebuild {
		for id := range previous {
			if _, keep := currentIDs[id]; keep {
				continue
			}
			batch.Delete(id)
			stats.Deleted++
			batched++
			if batched >= 200 {
				if err := flush(); err != nil {
					return nil, fmt.Errorf("flush batch: %w", err)
				}
			}
		}
	}
	if err := flush(); err != nil {
		return nil, fmt.Errorf("flush final: %w", err)
	}

	// Total tokens reflect the post-pack state: sum across every file
	// currently indexed (new, modified, and unchanged). Bytes come from the walk.
	stats.Tokens = estimateTokens(int(stats.Bytes))
	stats.Duration = time.Since(start)

	manifest := &Manifest{
		SchemaVersion: SchemaVersion,
		Version:       opts.DetritusVersion,
		Name:          name,
		Roots:         absRoots,
		PackedAt:      time.Now().UTC(),
		FileCount:     stats.Files,
		TotalBytes:    stats.Bytes,
		TotalTokens:   stats.Tokens,
		Skipped:       walkRes.Skipped,
	}
	if err := saveManifest(name, manifest); err != nil {
		return nil, fmt.Errorf("save manifest: %w", err)
	}

	return stats, nil
}

// Unpack deletes a pack's directory.
func Unpack(name string) error {
	if err := ValidatePackName(name); err != nil {
		return err
	}
	dir := PackDir(name)
	if _, err := os.Stat(dir); err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

// LoadManifest reads a pack's manifest from disk.
func LoadManifest(name string) (*Manifest, error) {
	if err := ValidatePackName(name); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(ManifestPath(name))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return &m, nil
}

// ListManifests returns every pack's manifest, sorted by name.
func ListManifests() ([]*Manifest, error) {
	entries, err := os.ReadDir(PacksDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*Manifest
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m, err := LoadManifest(e.Name())
		if err != nil {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func saveManifest(name string, m *Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(PackDir(name), 0o755); err != nil {
		return err
	}
	return os.WriteFile(ManifestPath(name), append(data, '\n'), 0o644)
}

func absolutizeAll(roots []string) ([]string, error) {
	out := make([]string, 0, len(roots))
	for _, r := range roots {
		abs, err := filepath.Abs(r)
		if err != nil {
			return nil, err
		}
		out = append(out, filepath.Clean(abs))
	}
	return out, nil
}

func sameRootSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := map[string]bool{}
	for _, x := range a {
		set[x] = true
	}
	for _, x := range b {
		if !set[x] {
			return false
		}
	}
	return true
}

// estimateTokens approximates the LLM token count from a byte length.
// Rule of thumb: ~1 token per 4 bytes of source.
func estimateTokens(size int) int {
	return size / 4
}

// indexMapping builds the Bleve index mapping for a pack.
func indexMapping() mapping.IndexMapping {
	m := bleve.NewIndexMapping()
	doc := bleve.NewDocumentMapping()

	text := bleve.NewTextFieldMapping()
	doc.AddFieldMappingsAt("path_rel", text)
	doc.AddFieldMappingsAt("root", text)
	doc.AddFieldMappingsAt("language", bleve.NewKeywordFieldMapping())
	doc.AddFieldMappingsAt("content", text)

	num := bleve.NewNumericFieldMapping()
	doc.AddFieldMappingsAt("size", num)
	doc.AddFieldMappingsAt("mtime_unix", num)
	doc.AddFieldMappingsAt("lines", num)

	m.AddDocumentMapping("file", doc)
	m.DefaultMapping = doc
	return m
}

func openOrCreateIndex(path string) (bleve.Index, error) {
	if _, err := os.Stat(path); err == nil {
		return bleve.Open(path)
	}
	return bleve.New(path, indexMapping())
}

type prevEntry struct {
	size  int64
	mtime int64
}

func enumerateIndex(idx bleve.Index) (map[string]prevEntry, error) {
	out := map[string]prevEntry{}
	const pageSize = 1000
	from := 0
	for {
		q := bleve.NewMatchAllQuery()
		req := bleve.NewSearchRequestOptions(q, pageSize, from, false)
		req.Fields = []string{"size", "mtime_unix"}
		res, err := idx.Search(req)
		if err != nil {
			return nil, err
		}
		if len(res.Hits) == 0 {
			break
		}
		for _, hit := range res.Hits {
			size, _ := hit.Fields["size"].(float64)
			mtime, _ := hit.Fields["mtime_unix"].(float64)
			out[hit.ID] = prevEntry{size: int64(size), mtime: int64(mtime)}
		}
		if len(res.Hits) < pageSize {
			break
		}
		from += pageSize
	}
	return out, nil
}

func buildDoc(f FileInfo, content []byte) map[string]any {
	return map[string]any{
		"root":       f.Root,
		"path_rel":   f.PathRel,
		"language":   f.Language,
		"content":    string(content),
		"size":       f.Size,
		"mtime_unix": f.MTime.Unix(),
		"lines":      countLines(content),
	}
}

func countLines(b []byte) int {
	if len(b) == 0 {
		return 0
	}
	n := 0
	for _, c := range b {
		if c == '\n' {
			n++
		}
	}
	if b[len(b)-1] != '\n' {
		n++
	}
	return n
}

// PackForCWD returns the name of the pack whose roots contain cwd, or "" if none.
// If multiple packs match, returns the one whose matching root is longest (most specific).
func PackForCWD(cwd string) (string, error) {
	cwdAbs, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}
	manifests, err := ListManifests()
	if err != nil {
		return "", err
	}
	best := ""
	bestLen := -1
	for _, m := range manifests {
		for _, root := range m.Roots {
			if strings.HasPrefix(cwdAbs+string(filepath.Separator), root+string(filepath.Separator)) || cwdAbs == root {
				if len(root) > bestLen {
					bestLen = len(root)
					best = m.Name
				}
			}
		}
	}
	return best, nil
}

// CleanContent applies cheap cleanup to file content:
// - strip trailing whitespace on every line
// - collapse runs of blank lines to a single blank line.
func CleanContent(content []byte) string {
	lines := strings.Split(string(content), "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimRight(ln, " \t")
	}
	var out []string
	blanks := 0
	for _, ln := range lines {
		if ln == "" {
			blanks++
			if blanks > 1 {
				continue
			}
		} else {
			blanks = 0
		}
		out = append(out, ln)
	}
	return strings.Join(out, "\n")
}

// SliceLines returns lines [start, end] (1-indexed, inclusive) of content.
// If start<=0, starts at 1. If end<=0 or >total, ends at total.
func SliceLines(content string, start, end int) string {
	lines := strings.Split(content, "\n")
	if start <= 0 {
		start = 1
	}
	if end <= 0 || end > len(lines) {
		end = len(lines)
	}
	if start > len(lines) {
		return ""
	}
	return strings.Join(lines[start-1:end], "\n")
}
