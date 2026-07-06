package memory

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Curation keeps the learned-memory corpus small and keyword-rich so FTS stays
// effective without embeddings: age unused lessons active→stale→archived by
// last_used, and enforce a hard cap on active lessons (archive the
// least-recently-used beyond it). Archived lessons stay on disk for audit
// (supersede-not-delete) but are not indexed/retrieved.

const (
	defaultStaleAfter   = 90 * 24 * time.Hour
	defaultArchiveAfter = 180 * 24 * time.Hour
)

// defaultMaxActive is the hard cap on active lessons (a var, not a const, so
// tests can lower it to exercise the cap without writing hundreds of lessons).
var defaultMaxActive = 500

// CurateOptions tunes a curation pass. Zero fields take defaults; Now is
// injectable for tests.
type CurateOptions struct {
	StaleAfter   time.Duration
	ArchiveAfter time.Duration
	MaxActive    int
	Now          time.Time
}

// CurateStats reports what a pass changed.
type CurateStats struct {
	Staled    int
	Archived  int
	ActiveNow int
}

func (o CurateOptions) withDefaults() CurateOptions {
	if o.StaleAfter == 0 {
		o.StaleAfter = defaultStaleAfter
	}
	if o.ArchiveAfter == 0 {
		o.ArchiveAfter = defaultArchiveAfter
	}
	if o.MaxActive == 0 {
		o.MaxActive = defaultMaxActive
	}
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	return o
}

// Curate runs one curation pass over the lesson store.
func Curate(opts CurateOptions) (CurateStats, error) {
	opts = opts.withDefaults()
	lessons, err := listLessons()
	if err != nil {
		return CurateStats{}, err
	}

	var stats CurateStats
	// Age by last_used: archive the long-unused, stale the medium-unused.
	for i := range lessons {
		l := &lessons[i]
		if l.Status == "archived" {
			continue
		}
		age := opts.Now.Sub(lastUsedTime(*l))
		switch {
		case age >= opts.ArchiveAfter:
			l.Status = "archived"
			stats.Archived++
			_ = write(*l)
		case age >= opts.StaleAfter && l.Status == "active":
			l.Status = "stale"
			stats.Staled++
			_ = write(*l)
		}
	}

	// Hard cap: keep the MaxActive most-recently-used active lessons; archive
	// the rest (least-recently-used first).
	var actives []Lesson
	for _, l := range lessons {
		if l.Status == "active" {
			actives = append(actives, l)
		}
	}
	sort.Slice(actives, func(i, j int) bool {
		return lastUsedTime(actives[i]).After(lastUsedTime(actives[j]))
	})
	for i := opts.MaxActive; i < len(actives); i++ {
		actives[i].Status = "archived"
		stats.Archived++
		_ = write(actives[i])
	}
	if len(actives) < opts.MaxActive {
		stats.ActiveNow = len(actives)
	} else {
		stats.ActiveNow = opts.MaxActive
	}
	return stats, nil
}

// Supersede marks a lesson stale (contradicted/replaced) without deleting it —
// it stays on disk for audit but drops out of retrieval.
func Supersede(id string) error {
	l, err := Get(id)
	if err != nil {
		return err
	}
	l.Status = "stale"
	return write(l)
}

// Touch records that a lesson was used: it refreshes last_used (so recency
// aging keeps it) and reactivates a stale lesson that proved useful again.
func Touch(id string) error {
	l, err := Get(id)
	if err != nil {
		return err
	}
	l.LastUsed = nowRFC3339()
	if l.Status == "stale" {
		l.Status = "active"
	}
	return write(l)
}

// LessonFile is a stored lesson's id and its on-disk markdown file path.
type LessonFile struct {
	ID   string
	Path string
}

// AllLessonFiles returns every stored lesson's id and file path with NO
// filtering — all statuses, all trust levels, stale or not. The lesson gateway
// (`detritus --contribute`) ships every lesson; the maturity fields ride along
// inside each file as data for downstream curation, never as a promotion gate.
// Results are sorted by id so a contribution's file list is deterministic. A
// missing lessons dir is not an error (nothing stored yet → empty slice).
func AllLessonFiles() ([]LessonFile, error) {
	entries, err := os.ReadDir(LessonsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []LessonFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		out = append(out, LessonFile{
			ID:   strings.TrimSuffix(e.Name(), ".md"),
			Path: filepath.Join(LessonsDir(), e.Name()),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func listLessons() ([]Lesson, error) {
	entries, err := os.ReadDir(LessonsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Lesson
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(LessonsDir(), e.Name()))
		if err != nil {
			continue
		}
		out = append(out, parseLesson(string(data)))
	}
	return out, nil
}

func lastUsedTime(l Lesson) time.Time {
	if t, err := time.Parse(time.RFC3339, l.LastUsed); err == nil {
		return t
	}
	return time.Time{}
}
