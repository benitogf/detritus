package main

import (
	"os"
	"path/filepath"
	"strings"
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

	generateClaudeSkills(home, []docEntry{{name: "flows/plan/plan", alias: "plan", desc: "Plan things"}})

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

func TestUpsertTOMLTableAddsDetritusMCP(t *testing.T) {
	input := `model = "gpt-5.5"

[mcp_servers.node_repl]
command = "node-repl"
args = []
`

	got := upsertTOMLTable(input, "mcp_servers.detritus", []string{
		`command = "C:\\detritus.exe"`,
		"args = []",
	})

	if !strings.Contains(got, "[mcp_servers.node_repl]\ncommand = \"node-repl\"") {
		t.Fatalf("existing node_repl table was not preserved:\n%s", got)
	}
	if !strings.Contains(got, "[mcp_servers.detritus]\ncommand = \"C:\\\\detritus.exe\"\nargs = []") {
		t.Fatalf("detritus table was not added:\n%s", got)
	}
}

func TestUpsertTOMLTableReplacesExistingDetritusMCP(t *testing.T) {
	input := `model = "gpt-5.5"

[mcp_servers.detritus]
command = "old"
args = ["--old"]

[mcp_servers.node_repl]
command = "node-repl"
`

	got := upsertTOMLTable(input, "mcp_servers.detritus", []string{
		`command = "new"`,
		"args = []",
	})

	if strings.Contains(got, "old") || strings.Contains(got, "--old") {
		t.Fatalf("old detritus table content was not replaced:\n%s", got)
	}
	if !strings.Contains(got, "[mcp_servers.detritus]\ncommand = \"new\"\nargs = []") {
		t.Fatalf("new detritus table missing:\n%s", got)
	}
	if !strings.Contains(got, "[mcp_servers.node_repl]\ncommand = \"node-repl\"") {
		t.Fatalf("following table was not preserved:\n%s", got)
	}
}

func TestUpsertTOMLTableReplacesParentDottedDetritusMCP(t *testing.T) {
	input := `model = "gpt-5.5"

[mcp_servers]
detritus.command = "old-detritus"
detritus.args = ["--old"]
node_repl.command = "node-repl"
`

	got := upsertTOMLTable(input, "mcp_servers.detritus", []string{
		`command = "new-detritus"`,
		"args = []",
	})

	if strings.Contains(got, "old-detritus") || strings.Contains(got, `detritus.command`) {
		t.Fatalf("old parent-table dotted detritus entry was not replaced:\n%s", got)
	}
	if !strings.Contains(got, `node_repl.command = "node-repl"`) {
		t.Fatalf("sibling parent-table entry was not preserved:\n%s", got)
	}
	if !strings.Contains(got, "[mcp_servers.detritus]\ncommand = \"new-detritus\"\nargs = []") {
		t.Fatalf("canonical detritus table missing:\n%s", got)
	}
	again := upsertTOMLTable(got, "mcp_servers.detritus", []string{
		`command = "new-detritus"`,
		"args = []",
	})
	if got != again {
		t.Fatalf("upsert was not stable after parent-table migration:\nfirst:\n%s\nsecond:\n%s", got, again)
	}
}

func TestUpsertTOMLTableReplacesTopLevelDottedDetritusMCP(t *testing.T) {
	input := `model = "gpt-5.5"
mcp_servers.detritus.command = "old-detritus"
mcp_servers.detritus.args = ["--old"]
mcp_servers.node_repl.command = "node-repl"
`

	got := upsertTOMLTable(input, "mcp_servers.detritus", []string{
		`command = "new-detritus"`,
		"args = []",
	})

	if strings.Contains(got, "old-detritus") || strings.Contains(got, `mcp_servers.detritus.command`) {
		t.Fatalf("old top-level dotted detritus entry was not replaced:\n%s", got)
	}
	if !strings.Contains(got, `mcp_servers.node_repl.command = "node-repl"`) {
		t.Fatalf("sibling top-level dotted entry was not preserved:\n%s", got)
	}
	if !strings.Contains(got, "[mcp_servers.detritus]\ncommand = \"new-detritus\"\nargs = []") {
		t.Fatalf("canonical detritus table missing:\n%s", got)
	}
	again := upsertTOMLTable(got, "mcp_servers.detritus", []string{
		`command = "new-detritus"`,
		"args = []",
	})
	if got != again {
		t.Fatalf("upsert was not stable after top-level dotted migration:\nfirst:\n%s\nsecond:\n%s", got, again)
	}
}

func TestUpsertTOMLTableUpdatesInlineParentTable(t *testing.T) {
	input := `model = "gpt-5.5"
mcp_servers = { node_repl = { command = "node-repl", args = [] }, detritus = { command = "old-detritus", args = ["--old"] } }
`

	got := upsertTOMLTable(input, "mcp_servers.detritus", []string{
		`command = "new-detritus"`,
		"args = []",
	})

	if strings.Contains(got, "old-detritus") || strings.Contains(got, "[mcp_servers.detritus]") {
		t.Fatalf("inline parent table should update in place without appending a subtable:\n%s", got)
	}
	if !strings.Contains(got, `node_repl = { command = "node-repl", args = [] }`) {
		t.Fatalf("sibling inline entry was not preserved:\n%s", got)
	}
	if !strings.Contains(got, `detritus = { command = "new-detritus", args = [] }`) {
		t.Fatalf("inline detritus entry was not updated:\n%s", got)
	}
	again := upsertTOMLTable(got, "mcp_servers.detritus", []string{
		`command = "new-detritus"`,
		"args = []",
	})
	if got != again {
		t.Fatalf("inline parent-table upsert was not stable:\nfirst:\n%s\nsecond:\n%s", got, again)
	}
}

func TestUpsertCodexMCPConfigWritesEscapedWindowsPath(t *testing.T) {
	config := filepath.Join(t.TempDir(), "config.toml")

	upsertCodexMCPConfig(config, `C:\Users\Owner\AppData\Local\detritus-codex\detritus.exe`)

	raw, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if !strings.Contains(got, `[mcp_servers.detritus]`) {
		t.Fatalf("detritus table missing:\n%s", got)
	}
	if !strings.Contains(got, `command = "C:\\Users\\Owner\\AppData\\Local\\detritus-codex\\detritus.exe"`) {
		t.Fatalf("windows path was not escaped as a TOML basic string:\n%s", got)
	}
}
