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
	"strings"
	"time"

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
	Title    string
	Bullets  []string
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

	if existing, err := Get(id); err == nil {
		// Itemized-delta append — never collapse or rewrite prior bullets (ACE).
		existing.Bullets = appendUnique(existing.Bullets, bullets)
		existing.Source = src
		existing.LastUsed = nowRFC3339()
		if existing.Kind == "" {
			existing.Kind = kind
		}
		return id, write(existing)
	}

	return id, write(Lesson{
		ID:       id,
		Kind:     kind,
		Status:   "active",
		Trust:    "verified",
		Source:   src,
		LastUsed: nowRFC3339(),
		Title:    id,
		Bullets:  bullets,
	})
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
		_ = os.WriteFile(gi, []byte("# derived, rebuilt from lessons/ — never committed\nindex.bleve/\n.index.lock\n.index-built\n"), 0o644)
	}
	if _, err := os.Stat(filepath.Join(MemoryDir(), ".git")); err != nil {
		cmd := exec.Command("git", "init", "-q")
		cmd.Dir = MemoryDir()
		_ = cmd.Run() // best-effort; absence of git is not fatal
	}
	return nil
}

func write(l Lesson) error {
	return os.WriteFile(lessonPath(l.ID), []byte(marshalLesson(l)), 0o644)
}

func appendUnique(existing, add []string) []string {
	seen := map[string]bool{}
	for _, b := range existing {
		seen[b] = true
	}
	for _, b := range add {
		if !seen[b] {
			existing = append(existing, b)
			seen[b] = true
		}
	}
	return existing
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
