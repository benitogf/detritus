package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGenerateClaudeSkillsPrunesStale verifies that a detritus-generated skill
// whose backing doc was removed gets pruned on the next setup, while a
// hand-authored skill (no detritus marker) is left untouched and current docs
// install as before.
func TestGenerateClaudeSkillsPrunesStale(t *testing.T) {
	home := t.TempDir()
	skillsDir := filepath.Join(home, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Stale detritus-generated skill (its doc was deleted) — carries our marker.
	staleDir := filepath.Join(skillsDir, "audit-add")
	if err := os.MkdirAll(staleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staleDir, "SKILL.md"),
		[]byte("---\nname: audit-add\ndescription: x\n---\n\nCall kb_get with name=\"meta/audit-add\" and follow the instructions in the returned document.\n"),
		0o644); err != nil {
		t.Fatal(err)
	}

	// Hand-authored skill that even mentions kb_get in prose — but lacks the
	// detritus generator marker, so it must survive (the false-positive the
	// issue specifically warns against).
	userDir := filepath.Join(skillsDir, "my-custom")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userDir, "SKILL.md"),
		[]byte("---\nname: my-custom\ndescription: mine\n---\n\nMy own workflow. It happens to call kb_get sometimes.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A directory with no SKILL.md at all — must be left alone (guards the
	// err == nil branch from ever deleting unrelated dirs).
	emptyDir := filepath.Join(skillsDir, "not-a-skill")
	if err := os.MkdirAll(emptyDir, 0o755); err != nil {
		t.Fatal(err)
	}

	generateClaudeSkills(home, []docEntry{{name: "meta/plan", alias: "plan", desc: "Plan things"}})

	// Current doc installed.
	if _, err := os.Stat(filepath.Join(skillsDir, "plan", "SKILL.md")); err != nil {
		t.Fatalf("current skill not installed: %v", err)
	}
	// Stale detritus skill pruned.
	if _, err := os.Stat(staleDir); !os.IsNotExist(err) {
		t.Fatalf("stale detritus skill not pruned (stat err = %v)", err)
	}
	// Hand-authored skill (with a prose kb_get mention) preserved.
	if _, err := os.Stat(filepath.Join(userDir, "SKILL.md")); err != nil {
		t.Fatalf("hand-authored skill was removed: %v", err)
	}
	// SKILL.md-less directory left alone.
	if _, err := os.Stat(emptyDir); err != nil {
		t.Fatalf("directory without SKILL.md was removed: %v", err)
	}
}
