// Package memory is the learned-memory store: verified, distilled lessons kept
// as markdown files under ~/.detritus/memory/lessons (the durable truth, its own
// git repo) and retrieved over the shared internal/core engine. Entries are
// agent-authored and untrusted; the verification gate (only outcome=green
// distils) is the write firewall, and a per-lesson trust/source field carries
// provenance.
package memory

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/benitogf/detritus/internal/core"
)

// Source is the provenance of a distilled lesson: which verified run/task
// produced it, the outcome (only "green" distils), and when.
type Source struct {
	Run     string
	Task    string
	Outcome string
	TS      string
}

// Lesson is one verified, distilled lesson — a reusable procedure or a durable
// fact. Bullets are appended over time (itemized delta), never rewritten.
type Lesson struct {
	ID       string
	Kind     string // "procedure" | "fact"
	Status   string // "active" | "stale" | "archived"
	Trust    string // "verified"
	Source   Source
	LastUsed string // RFC3339
	Validity string // RFC3339 expiry, or "" for none
	// Confirmed counts how many times this same lesson was re-distilled (a
	// re-Put of the same id). A higher count is corroboration — the lesson has
	// proven useful/true across independent verified runs — and lifts its
	// retrieval ranking. Absent in pre-P3-2 stored records → 0.
	Confirmed     int    // times re-confirmed via re-Put
	LastConfirmed string // RFC3339 of the last re-confirmation, or ""
	Title         string
	Bullets       []string
}

// MemoryDir is the learned-memory root (its own git repo).
func MemoryDir() string { return filepath.Join(core.DataDir(), "memory") }

// LessonsDir holds one markdown file per lesson — the durable source of truth.
func LessonsDir() string { return filepath.Join(MemoryDir(), "lessons") }

// IndexDir holds the derived Bleve index (gitignored; rebuilt from disk).
func IndexDir() string { return filepath.Join(MemoryDir(), "index.bleve") }

func lessonPath(id string) string { return filepath.Join(LessonsDir(), id+".md") }

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }

// Put is the only write path. It enforces the verification gate — an
// unverified run (outcome != "green") distils nothing — then appends an
// itemized delta: new bullets are added to an existing lesson (never a
// whole-file rewrite) or a new lesson file is created. Returns the lesson's id.
func Put(id, kind string, bullets []string, src Source) (string, error) {
	if src.Outcome != "green" {
		return "", fmt.Errorf("skill_put rejected: only verified-green work distils (outcome=%q)", src.Outcome)
	}
	if kind != "procedure" && kind != "fact" {
		return "", fmt.Errorf("kind must be \"procedure\" or \"fact\", got %q", kind)
	}
	if strings.TrimSpace(id) == "" {
		return "", fmt.Errorf("id required")
	}
	if len(bullets) == 0 {
		return "", fmt.Errorf("at least one delta bullet required")
	}
	if err := ensureStore(); err != nil {
		return "", err
	}

	var lesson Lesson
	if existing, err := Get(id); err == nil {
		// Itemized-delta append — never collapse or rewrite prior bullets (ACE).
		// A re-Put of an existing id is corroboration (P3-2): bump the
		// confirmation counter even when every bullet dedups away, so a lesson
		// re-derived by independent verified runs ranks higher.
		existing.Bullets = appendUnique(existing.Bullets, bullets)
		existing.Source = src
		existing.LastUsed = nowRFC3339()
		existing.Confirmed++
		existing.LastConfirmed = nowRFC3339()
		if existing.Kind == "" {
			existing.Kind = kind
		}
		lesson = existing
	} else {
		lesson = Lesson{
			ID:       id,
			Kind:     kind,
			Status:   "active",
			Trust:    "verified",
			Source:   src,
			LastUsed: nowRFC3339(),
			Title:    id,
			Bullets:  appendUnique(nil, bullets),
		}
	}
	// P3-4: refuse to blind-append a lesson that strongly contradicts a
	// different active lesson (same kind, near-identical topic, but materially
	// different content). The caller must resolve it explicitly by re-putting
	// with supersedes:<id> (which marks the old one stale before this write, so
	// the check then passes) or by choosing a more specific id. Only a STRONG
	// overlap fires, so genuinely distinct lessons are never blocked.
	if conflictID, ok := detectConflict(lesson); ok {
		return "", fmt.Errorf(
			"skill_put conflict: lesson %q (same kind, near-identical topic) already holds different content; re-put with supersedes:%q to replace it, or use a more specific id",
			conflictID, conflictID)
	}
	if err := write(lesson); err != nil {
		return "", err
	}
	// Single-writer curation pass: age + hard-cap on every distillation so the
	// corpus stays bounded with no separate maintenance step. Best-effort — a
	// curation error never fails a write. Cost is O(lessons) per distil (a dir
	// read + parse), bounded by the active-cap (~500) and writing only on a
	// status transition; distillation is infrequent, so this is acceptable.
	_, _ = Curate(CurateOptions{})
	return id, nil
}

// Get reads a lesson by id from disk (the source of truth).
func Get(id string) (Lesson, error) {
	data, err := os.ReadFile(lessonPath(id))
	if err != nil {
		return Lesson{}, fmt.Errorf("lesson %q not found", id)
	}
	return parseLesson(string(data)), nil
}

// ensureStore creates the lesson dir, gitignores the derived index, and inits
// the memory dir as its own git repo (best-effort — files work without git;
// git is the cross-machine sync mechanism).
func ensureStore() error {
	if err := os.MkdirAll(LessonsDir(), 0o755); err != nil {
		return err
	}
	gi := filepath.Join(MemoryDir(), ".gitignore")
	if _, err := os.Stat(gi); err != nil {
		_ = os.WriteFile(gi, []byte("# derived, rebuilt from lessons/ — never committed\nindex.bleve/\n.index.lock\n.index-built\nrecall.log\nlessons/*.tmp\n"), 0o644)
	}
	if _, err := os.Stat(filepath.Join(MemoryDir(), ".git")); err != nil {
		cmd := exec.Command("git", "init", "-q")
		cmd.Dir = MemoryDir()
		_ = cmd.Run() // best-effort; absence of git is not fatal
	}
	return nil
}

// write persists a lesson atomically (temp + rename) so a concurrent reader —
// the index rebuild, or another session — never observes a half-written lesson
// file. (Lost-update under concurrent same-id read-modify-write across sessions
// remains a documented v1 boundary; see index.go.)
func write(l Lesson) error {
	dst := lessonPath(l.ID)
	tmp, err := os.CreateTemp(LessonsDir(), "lesson-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write([]byte(marshalLesson(l))); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// appendUnique appends bullets that aren't already present, comparing on a
// normalized form (case- and whitespace-insensitive) so re-distilling the same
// insight doesn't duplicate it (dedup-at-write). Original text is preserved.
func appendUnique(existing, add []string) []string {
	seen := map[string]bool{}
	for _, b := range existing {
		seen[normalizeBullet(b)] = true
	}
	for _, b := range add {
		n := normalizeBullet(b)
		if !seen[n] {
			existing = append(existing, b)
			seen[n] = true
		}
	}
	return existing
}

func normalizeBullet(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// Conflict-detection thresholds (P3-4). A conflict fires only when a lesson has
// a near-identical TOPIC (title token Jaccard ≥ conflictTitleSim) AND largely
// different CONTENT (bullet Jaccard < conflictContentOverlap) as another active
// lesson of the same kind. Requiring both — same topic yet disjoint advice — is
// what makes it a contradiction rather than a duplicate (high content overlap)
// or an unrelated lesson (low title overlap), so ordinary distinct lessons,
// whose ids/titles diverge, never trip it.
const (
	conflictTitleSim       = 0.8
	conflictContentOverlap = 0.34
)

// detectConflict reports whether l strongly contradicts a different active
// lesson, returning that lesson's id. It reads the store best-effort; a read
// error means "no conflict" (never block a write on a transient dir error).
//
// Honest scope: Put sets Title = id, so the "topic similarity" gate below (title
// token Jaccard ≥ conflictTitleSim) is in practice ID-TOKEN similarity, not a
// semantic topic match. The check therefore fires only when two lessons have
// near-identical IDs yet materially disjoint bullets — a deliberately
// conservative, false-negative-biased write guard (it will miss two same-topic
// lessons whose ids diverge), which is the right bias for an advisory guard that
// must never block a genuinely distinct distillation.
func detectConflict(l Lesson) (string, bool) {
	lessons, err := listLessons()
	if err != nil {
		return "", false
	}
	newTitle := tokenSet(l.Title)
	newBullets := bulletSet(l.Bullets)
	for _, other := range lessons {
		if other.ID == l.ID || other.Status != "active" || other.Kind != l.Kind {
			continue
		}
		if jaccard(newTitle, tokenSet(other.Title)) < conflictTitleSim {
			continue // different topic — not a contradiction
		}
		if jaccard(newBullets, bulletSet(other.Bullets)) >= conflictContentOverlap {
			continue // same topic, same content — a duplicate, not a conflict
		}
		return other.ID, true
	}
	return "", false
}

// tokenSet splits a title into a set of lowercased alphanumeric tokens, so
// "retry-with-backoff" and "Retry With Backoff" compare equal.
func tokenSet(s string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, tok := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		out[tok] = struct{}{}
	}
	return out
}

// bulletSet is the set of normalized whole-bullet strings — content compared at
// bullet granularity so differing advice on the same topic reads as disjoint.
func bulletSet(bullets []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, b := range bullets {
		out[normalizeBullet(b)] = struct{}{}
	}
	return out
}

// jaccard is |A∩B| / |A∪B| for two sets; 0 when both are empty.
func jaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	inter := 0
	for k := range a {
		if _, ok := b[k]; ok {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// marshalLesson renders a lesson as flat-frontmatter markdown. source.* is
// flattened to source_<field> so the frontmatter stays line-parseable (no YAML
// dep), mirroring the KB's hand-rolled frontmatter.
func marshalLesson(l Lesson) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "id: %s\n", l.ID)
	fmt.Fprintf(&b, "kind: %s\n", l.Kind)
	fmt.Fprintf(&b, "status: %s\n", l.Status)
	fmt.Fprintf(&b, "trust: %s\n", l.Trust)
	fmt.Fprintf(&b, "source_run: %s\n", l.Source.Run)
	fmt.Fprintf(&b, "source_task: %s\n", l.Source.Task)
	fmt.Fprintf(&b, "source_outcome: %s\n", l.Source.Outcome)
	fmt.Fprintf(&b, "source_ts: %s\n", l.Source.TS)
	fmt.Fprintf(&b, "last_used: %s\n", l.LastUsed)
	fmt.Fprintf(&b, "validity: %s\n", l.Validity)
	fmt.Fprintf(&b, "confirmed: %d\n", l.Confirmed)
	fmt.Fprintf(&b, "last_confirmed: %s\n", l.LastConfirmed)
	b.WriteString("---\n")
	title := l.Title
	if title == "" {
		title = l.ID
	}
	fmt.Fprintf(&b, "# %s\n", title)
	for _, bullet := range l.Bullets {
		fmt.Fprintf(&b, "- %s\n", bullet)
	}
	return b.String()
}

func parseLesson(content string) Lesson {
	var l Lesson
	body := content
	if strings.HasPrefix(content, "---\n") {
		if end := strings.Index(content[4:], "\n---"); end >= 0 {
			fm := content[4 : end+4]
			body = strings.TrimPrefix(content[end+4:], "\n---")
			for _, line := range strings.Split(fm, "\n") {
				key, val, ok := strings.Cut(line, ": ")
				if !ok {
					key, val, ok = strings.Cut(line, ":")
					val = strings.TrimSpace(val)
				}
				if !ok {
					continue
				}
				switch strings.TrimSpace(key) {
				case "id":
					l.ID = val
				case "kind":
					l.Kind = val
				case "status":
					l.Status = val
				case "trust":
					l.Trust = val
				case "source_run":
					l.Source.Run = val
				case "source_task":
					l.Source.Task = val
				case "source_outcome":
					l.Source.Outcome = val
				case "last_used":
					l.LastUsed = val
				case "validity":
					l.Validity = val
				case "confirmed":
					// Absent in pre-P3-2 records → left at 0 (back-compat).
					if n, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
						l.Confirmed = n
					}
				case "last_confirmed":
					l.LastConfirmed = val
				case "source_ts":
					l.Source.TS = val
				}
			}
		}
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "# ") {
			l.Title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
			continue
		}
		if strings.HasPrefix(line, "- ") {
			l.Bullets = append(l.Bullets, strings.TrimSpace(strings.TrimPrefix(line, "- ")))
		}
	}
	return l
}
