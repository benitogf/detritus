package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestSetupDoesNotRegisterCandylandMCP: detritus drives candyland over REST (see
// candyland_client.go); it must NOT register candyland as an MCP server in a host
// config. Only detritus is upserted; candyland never appears.
func TestSetupDoesNotRegisterCandylandMCP(t *testing.T) {
	dir := t.TempDir()
	detr := filepath.Join(dir, "detritus")
	candylandName := "candyland"
	if runtime.GOOS == "windows" {
		candylandName = "candyland.exe"
	}
	cbin := filepath.Join(dir, candylandName)
	for _, p := range []string{detr, cbin} {
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cfg := filepath.Join(dir, ".claude.json")
	upsertMCP(cfg, "mcpServers", detr)

	raw, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatalf("read %s: %v", cfg, err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("parse %s: %v", cfg, err)
	}
	s, _ := data["mcpServers"].(map[string]any)
	if s["detritus"] == nil {
		t.Error("detritus must be registered")
	}
	if _, present := s["candyland"]; present {
		t.Error("candyland must NOT be registered as an MCP server")
	}
}

// TestRemoveMCPServerStripsStaleCandyland verifies that removeMCPServer deletes a
// stale candyland entry while leaving detritus and any other server untouched.
func TestRemoveMCPServerStripsStaleCandyland(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), ".claude.json")
	seed := map[string]any{
		"mcpServers": map[string]any{
			"detritus":  map[string]any{"command": "/bin/detritus", "args": []any{}},
			"candyland": map[string]any{"command": "/bin/candyland", "args": []any{"control-mcp"}},
			"other":     map[string]any{"command": "/bin/other", "args": []any{}},
		},
		"otherTopLevel": "keep-me",
	}
	raw, _ := json.MarshalIndent(seed, "", "  ")
	if err := os.WriteFile(cfg, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	removeMCPServer(cfg, "mcpServers", "candyland")

	got, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatalf("read %s: %v", cfg, err)
	}
	var data map[string]any
	if err := json.Unmarshal(got, &data); err != nil {
		t.Fatalf("parse %s: %v", cfg, err)
	}
	s, _ := data["mcpServers"].(map[string]any)
	if _, present := s["candyland"]; present {
		t.Error("stale candyland entry must be removed")
	}
	if s["detritus"] == nil {
		t.Error("detritus entry must be preserved")
	}
	if s["other"] == nil {
		t.Error("unrelated server must be preserved")
	}
	if data["otherTopLevel"] != "keep-me" {
		t.Error("unrelated top-level keys must be preserved")
	}
}

// TestRemoveMCPServerNoCandylandIsNoOp verifies that removeMCPServer leaves a
// config without a candyland entry byte-for-byte unchanged (no reformat thrash).
func TestRemoveMCPServerNoCandylandIsNoOp(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), ".claude.json")
	original := []byte("{\n  \"mcpServers\": {\n    \"detritus\": {\"command\": \"/bin/detritus\"}\n  }\n}\n")
	if err := os.WriteFile(cfg, original, 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(cfg)
	if err != nil {
		t.Fatal(err)
	}

	removeMCPServer(cfg, "mcpServers", "candyland")

	got, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatalf("read %s: %v", cfg, err)
	}
	if string(got) != string(original) {
		t.Errorf("file with no candyland entry must be untouched\nwant: %q\ngot:  %q", original, got)
	}
	after, err := os.Stat(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("no-op must not rewrite the file (modtime changed)")
	}
}

// TestRemoveMCPServerMissingFileIsNoOp verifies removeMCPServer does not create a
// file that doesn't exist.
func TestRemoveMCPServerMissingFileIsNoOp(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "absent.json")
	removeMCPServer(cfg, "mcpServers", "candyland")
	if _, err := os.Stat(cfg); !os.IsNotExist(err) {
		t.Errorf("removeMCPServer must not create a missing file, stat err: %v", err)
	}
}

// TestRemoveTOMLTableStripsStaleCandyland verifies the codex TOML remover strips
// the [mcp_servers.candyland] block while keeping [mcp_servers.detritus].
func TestRemoveTOMLTableStripsStaleCandyland(t *testing.T) {
	input := strings.Join([]string{
		"[mcp_servers.candyland]",
		`command = "/bin/candyland"`,
		`args = ["control-mcp"]`,
		"",
		"[mcp_servers.detritus]",
		`command = "/bin/detritus"`,
		"args = []",
		"",
	}, "\n")

	got := removeTOMLTable(input, "mcp_servers.candyland")

	if strings.Contains(got, "candyland") {
		t.Errorf("candyland table must be stripped, got:\n%s", got)
	}
	if !strings.Contains(got, "[mcp_servers.detritus]") {
		t.Errorf("detritus table must be preserved, got:\n%s", got)
	}
	if !strings.Contains(got, "/bin/detritus") {
		t.Errorf("detritus body must be preserved, got:\n%s", got)
	}
}

// TestRemoveTOMLTableNoTableIsNoOp verifies removeTOMLTable returns the content
// unchanged when the table isn't present.
func TestRemoveTOMLTableNoTableIsNoOp(t *testing.T) {
	input := "[mcp_servers.detritus]\ncommand = \"/bin/detritus\"\nargs = []\n"
	if got := removeTOMLTable(input, "mcp_servers.candyland"); got != input {
		t.Errorf("absent table must be a no-op\nwant: %q\ngot:  %q", input, got)
	}
}

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

// TestDirOnPath verifies the PATH membership check.
func TestDirOnPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", "/usr/bin"+string(os.PathListSeparator)+dir)
	if !dirOnPath(dir) {
		t.Errorf("dirOnPath(%q) = false, want true", dir)
	}
	if dirOnPath(filepath.Join(dir, "absent")) {
		t.Error("dirOnPath must be false for a directory not on PATH")
	}
}

// TestSelfPlaceCopiesAndReferencesInstalledBinary verifies that when the running
// binary is not on PATH, selfPlace copies it into installBinDir() and returns
// that stable path for the rest of setup to reference.
func TestSelfPlaceCopiesAndReferencesInstalledBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix profile placement path")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", "/usr/bin") // install dir is not on PATH

	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "detritus")
	if err := os.WriteFile(src, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	dest := selfPlace(src, false)

	want := filepath.Join(home, ".local", "bin", "detritus")
	if dest != want {
		t.Fatalf("selfPlace returned %q, want %q", dest, want)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("binary not placed at %s: %v", dest, err)
	}
	// PATH export line landed in ~/.profile.
	profile, err := os.ReadFile(filepath.Join(home, ".profile"))
	if err != nil {
		t.Fatalf("read .profile: %v", err)
	}
	if !strings.Contains(string(profile), filepath.Join(home, ".local", "bin")) {
		t.Errorf(".profile missing PATH export:\n%s", profile)
	}
}

// TestSelfPlaceNoOpWhenAlreadyOnPath verifies that a binary already reachable on
// PATH is left in place (returned path unchanged, nothing copied).
func TestSelfPlaceNoOpWhenAlreadyOnPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	srcDir := t.TempDir()
	t.Setenv("PATH", srcDir)
	src := filepath.Join(srcDir, binaryName())
	if err := os.WriteFile(src, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}

	if got := selfPlace(src, false); got != src {
		t.Errorf("selfPlace returned %q, want unchanged %q", got, src)
	}
	if _, err := os.Stat(filepath.Join(installBinDir(), binaryName())); !os.IsNotExist(err) {
		t.Errorf("nothing should be copied into installBinDir when already on PATH (stat err = %v)", err)
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

// TestGenerateClaudeCoderAgentUsesEffortKey verifies the /forge coder subagent
// definition carries its low reasoning effort via the dedicated `effort`
// frontmatter key — Claude Code's per-subagent override — rather than trying to
// smuggle it through `model` (which takes an alias/ID only) or leaning on a
// Workflow model fallback. This locks the "effort frontmatter key vs Workflow
// fallback" decision: the definition file owns it directly.
func TestGenerateClaudeCoderAgentUsesEffortKey(t *testing.T) {
	home := t.TempDir()

	generateClaudeCoderAgent(home)

	agentFile := filepath.Join(home, ".claude", "agents", "detritus-coder.md")
	raw, err := os.ReadFile(agentFile)
	if err != nil {
		t.Fatalf("coder agent definition not written: %v", err)
	}
	got := string(raw)

	fm, _, ok := strings.Cut(strings.TrimPrefix(got, "---\n"), "\n---")
	if !ok {
		t.Fatalf("agent definition missing YAML frontmatter:\n%s", got)
	}
	if !strings.Contains(fm, "\neffort: low") {
		t.Errorf("frontmatter must set the effort key to low; got:\n%s", fm)
	}
	if !strings.Contains(fm, "\nmodel: inherit") {
		t.Errorf("coder should inherit the session model, not pin one; got:\n%s", fm)
	}
	// effort must be its own key, never encoded into model (which is alias/ID only).
	if strings.Contains(fm, "model: inherit low") || strings.Contains(fm, "model: low") {
		t.Errorf("effort must not be smuggled into the model field; got:\n%s", fm)
	}
	if !strings.Contains(fm, "name: detritus-coder") {
		t.Errorf("agent must be named detritus-coder so /forge can spawn it; got:\n%s", fm)
	}
	// A `tools:` restriction strips the built-in file/shell tools and leaves the
	// coder unable to edit anything — the definition must inherit the full set.
	if strings.Contains(fm, "tools:") {
		t.Errorf("frontmatter must not restrict tools (the coder needs Read/Edit/Bash to code); got:\n%s", fm)
	}
}
