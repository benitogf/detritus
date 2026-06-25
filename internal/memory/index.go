package memory

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/benitogf/detritus/internal/core"
	"github.com/blevesearch/bleve/v2"
	"github.com/gofrs/flock"
)

func lockPath() string   { return filepath.Join(MemoryDir(), ".index.lock") }
func markerPath() string { return filepath.Join(MemoryDir(), ".index-built") }

// Search returns ranked snippets for verified lessons matching the query. The
// derived Bleve index is rebuilt from the lesson files whenever they have
// changed (mtime sentinel), and the rebuild+search is serialized across
// sessions by a flock — so concurrent rebuilds never leave a half-written index
// (LM3; the index is derived and always recoverable by re-deriving from disk).
// Lesson-file writes (Put/Touch/Curate) are not under this lock: within a
// session MCP stdio serializes them, and the cross-session case (two processes
// sharing ~/.detritus) is a documented v1 boundary — a lost append at worst,
// never index corruption, since files are the source of truth and git handles
// their merge. Results carry only keys + snippets (JIT, never the corpus — LM7).
func Search(query string, topN int) ([]core.Result, error) {
	if topN <= 0 {
		topN = 10
	}
	if err := ensureStore(); err != nil {
		return nil, err
	}
	fl := flock.New(lockPath())
	if err := fl.Lock(); err != nil {
		return nil, err
	}
	defer fl.Unlock()

	if err := rebuildIfStale(); err != nil {
		return nil, err
	}
	idx, err := bleve.Open(IndexDir())
	if err != nil {
		return nil, nil // no lessons indexed yet
	}
	defer idx.Close()

	raw, err := bleveSearch(idx, query, topN*3)
	if err != nil {
		return nil, err
	}
	return core.Rank(raw, core.DefaultMMRLambda, topN, core.DefaultMinScore), nil
}

type indexedLesson struct {
	id, content, kind, status, trust string
}

// readLessons loads the active/stale lessons from disk and the newest mtime
// across all lesson files (the rebuild sentinel). Archived lessons are kept on
// disk for audit but not indexed.
func readLessons() ([]indexedLesson, time.Time, error) {
	entries, err := os.ReadDir(LessonsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, time.Time{}, nil
		}
		return nil, time.Time{}, err
	}
	var out []indexedLesson
	var newest time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		if fi.ModTime().After(newest) {
			newest = fi.ModTime()
		}
		data, err := os.ReadFile(filepath.Join(LessonsDir(), e.Name()))
		if err != nil {
			continue
		}
		l := parseLesson(string(data))
		if l.Status == "archived" {
			continue
		}
		out = append(out, indexedLesson{
			id:      l.ID,
			content: l.Title + "\n" + strings.Join(l.Bullets, "\n"),
			kind:    l.Kind,
			status:  l.Status,
			trust:   l.Trust,
		})
	}
	return out, newest, nil
}

// rebuildIfStale rebuilds the index only when a lesson file is newer than the
// last build marker (or the index is missing). Caller must hold the flock.
func rebuildIfStale() error {
	lessons, newest, err := readLessons()
	if err != nil {
		return err
	}
	if len(lessons) == 0 {
		os.RemoveAll(IndexDir())
		os.Remove(markerPath())
		return nil
	}
	if built, err := os.ReadFile(markerPath()); err == nil {
		if ts, perr := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(built))); perr == nil && !newest.After(ts) {
			if _, serr := os.Stat(IndexDir()); serr == nil {
				return nil // index is current
			}
		}
	}
	return rebuild(lessons, newest)
}

func rebuild(lessons []indexedLesson, newest time.Time) error {
	os.RemoveAll(IndexDir())
	idx, err := bleve.New(IndexDir(), bleve.NewIndexMapping())
	if err != nil {
		return err
	}
	batch := idx.NewBatch()
	for _, l := range lessons {
		if err := batch.Index(l.id, map[string]any{
			"content": l.content,
			"kind":    l.kind,
			"status":  l.status,
			"trust":   l.trust,
		}); err != nil {
			idx.Close()
			return err
		}
	}
	if err := idx.Batch(batch); err != nil {
		idx.Close()
		return err
	}
	if err := idx.Close(); err != nil {
		return err
	}
	return os.WriteFile(markerPath(), []byte(newest.Format(time.RFC3339Nano)), 0o644)
}

func bleveSearch(idx bleve.Index, query string, n int) ([]core.Result, error) {
	q := bleve.NewMatchQuery(query)
	req := bleve.NewSearchRequestOptions(q, n, 0, false)
	req.Fields = []string{"content", "kind", "status", "trust"}
	res, err := idx.Search(req)
	if err != nil {
		return nil, err
	}
	var out []core.Result
	for _, hit := range res.Hits {
		content, _ := hit.Fields["content"].(string)
		trust, _ := hit.Fields["trust"].(string)
		out = append(out, core.Result{
			DocName: hit.ID,
			Score:   hit.Score,
			Snippet: core.Truncate(content, 200),
			Trust:   trust,
		})
	}
	return out, nil
}
