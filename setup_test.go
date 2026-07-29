package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// TestWriteFileOrWarnSurfacesWriteError verifies the generators' shared write
// helper does not silently discard a failed write (issue #248): it returns true
// and writes on a good path, and returns false when the write fails.
func TestWriteFileOrWarnSurfacesWriteError(t *testing.T) {
	dir := t.TempDir()

	if !writeFileOrWarn(filepath.Join(dir, "out.md"), []byte("hi")) {
		t.Fatalf("writeFileOrWarn reported failure on a writable path")
	}
	if got, err := os.ReadFile(filepath.Join(dir, "out.md")); err != nil || string(got) != "hi" {
		t.Fatalf("content not written: got %q err %v", got, err)
	}

	// A directory path cannot be written as a file — the error must surface as a
	// false return, not be silently ignored. Silence the expected stderr warning
	// so it doesn't clutter test output.
	devnull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	orig := os.Stderr
	os.Stderr = devnull
	ok := writeFileOrWarn(dir, []byte("hi"))
	os.Stderr = orig
	devnull.Close()
	if ok {
		t.Errorf("writeFileOrWarn reported success writing to a directory path")
	}
}

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

func TestUpsertCopilotCLIMCPWritesExpectedShape(t *testing.T) {
	file := filepath.Join(t.TempDir(), "mcp-config.json")
	if err := upsertCopilotCLIMCP(file, "/usr/local/bin/detritus"); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	servers, ok := data["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("missing mcpServers in %s", file)
	}
	entry, ok := servers["detritus"].(map[string]any)
	if !ok {
		t.Fatalf("missing detritus entry in %s", file)
	}
	if entry["type"] != "local" {
		t.Fatalf("expected type=local, got %v", entry["type"])
	}
	if entry["command"] != "/usr/local/bin/detritus" {
		t.Fatalf("expected command to be preserved, got %v", entry["command"])
	}
	args, ok := entry["args"].([]any)
	if !ok || len(args) != 0 {
		t.Fatalf("expected args=[], got %T %v", entry["args"], entry["args"])
	}
	tools, ok := entry["tools"].([]any)
	if !ok || len(tools) != 1 || tools[0] != "*" {
		t.Fatalf("expected tools=[\"*\"], got %T %v", entry["tools"], entry["tools"])
	}
}

func TestSetupOpenCodeWritesMCPAndCommands(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(config), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte(`{"username":"me","mcp":{"other":{"type":"local","command":["other"]}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	setupOpenCode(home, "/usr/local/bin/detritus", false)

	raw, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	if data["username"] != "me" {
		t.Fatalf("unrelated config was not preserved: %v", data["username"])
	}
	if data["$schema"] != "https://opencode.ai/config.json" {
		t.Fatalf("schema missing: %v", data["$schema"])
	}
	mcp, ok := data["mcp"].(map[string]any)
	if !ok {
		t.Fatalf("missing mcp config: %v", data)
	}
	detritus, ok := mcp["detritus"].(map[string]any)
	if !ok {
		t.Fatalf("missing detritus MCP config: %v", mcp)
	}
	if detritus["type"] != "local" || detritus["enabled"] != true {
		t.Fatalf("invalid detritus MCP config: %v", detritus)
	}
	command, ok := detritus["command"].([]any)
	if !ok || len(command) != 1 || command[0] != "/usr/local/bin/detritus" {
		t.Fatalf("invalid detritus command: %v", detritus["command"])
	}

	plan, err := os.ReadFile(filepath.Join(home, ".config", "opencode", "commands", "plan.md"))
	if err != nil {
		t.Fatalf("plan command was not written: %v", err)
	}
	if !strings.Contains(string(plan), `name="flows/plan/plan"`) || !strings.Contains(string(plan), "$ARGUMENTS") {
		t.Fatalf("plan command has unexpected content:\n%s", plan)
	}

	truthseeker, err := os.ReadFile(filepath.Join(home, ".config", "opencode", "commands", "truthseeker.md"))
	if err != nil {
		t.Fatalf("truthseeker command was not written: %v", err)
	}
	if !strings.Contains(string(truthseeker), `name="flows/principles/truthseeker"`) {
		t.Fatalf("truthseeker command has unexpected content:\n%s", truthseeker)
	}
}

func TestOpenCodeConfigFileUsesExistingJSONC(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, ".config", "opencode", "opencode.jsonc")
	if err := os.MkdirAll(filepath.Dir(config), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte(`{"$schema":"https://opencode.ai/config.json"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	setupOpenCode(home, "/usr/local/bin/detritus", false)

	if !fileExists(config) {
		t.Fatal("existing JSONC config was not retained")
	}
	if fileExists(filepath.Join(filepath.Dir(config), "opencode.json")) {
		t.Fatal("setup created opencode.json beside an existing JSONC config")
	}
	if !fileContains(config, `"detritus"`) {
		t.Fatal("detritus MCP entry was not added to JSONC config")
	}
}

func TestOpenCodeConfigPrefersJSONCAndSupportsComments(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "opencode.json"), []byte(`{"mcp":{"detritus":{"enabled":false}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(dir, "opencode.jsonc")
	if err := os.WriteFile(config, []byte("{\n  // User settings\n  \"mcp\": {\n    \"other\": {\"type\": \"local\", \"command\": [\"other\"],},\n  },\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	setupOpenCode(home, "/usr/local/bin/detritus", false)

	raw, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("parse rewritten JSONC config: %v", err)
	}
	mcp := data["mcp"].(map[string]any)
	if _, ok := mcp["other"]; !ok {
		t.Fatalf("unrelated JSONC MCP config was not preserved: %v", mcp)
	}
	detritus, ok := mcp["detritus"].(map[string]any)
	if !ok || detritus["enabled"] != true {
		t.Fatalf("detritus config was not updated in effective JSONC file: %v", mcp)
	}
}

func TestOpenCodeConfigDirUsesXDGConfigHome(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	got := openCodeConfigDir(t.TempDir())
	want := filepath.Join(configHome, "opencode")
	if got != want {
		t.Fatalf("OpenCode config dir = %q, want %q", got, want)
	}
}

func TestOpenCodeConfigDirUsesOpenCodeOverride(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("OPENCODE_CONFIG_DIR", configDir)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if got := openCodeConfigDir(t.TempDir()); got != configDir {
		t.Fatalf("OpenCode config dir = %q, want %q", got, configDir)
	}
}

func TestOpenCodeConfigFileUsesOpenCodeOverride(t *testing.T) {
	config := filepath.Join(t.TempDir(), "custom.jsonc")
	t.Setenv("OPENCODE_CONFIG", config)

	if got := openCodeConfigFile(t.TempDir()); got != config {
		t.Fatalf("OpenCode config file = %q, want %q", got, config)
	}
}

func TestSetupOpenCodePrunesOnlyGeneratedCommands(t *testing.T) {
	home := t.TempDir()
	commands := filepath.Join(home, ".config", "opencode", "commands")
	if err := os.MkdirAll(commands, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(commands, "removed.md")
	if err := os.WriteFile(stale, []byte(pluginCommandGeneratedMarker), 0o644); err != nil {
		t.Fatal(err)
	}
	custom := filepath.Join(commands, "custom.md")
	if err := os.WriteFile(custom, []byte("My own command that can call kb_get."), 0o644); err != nil {
		t.Fatal(err)
	}

	setupOpenCode(home, "/usr/local/bin/detritus", false)

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale generated command was not pruned: %v", err)
	}
	if _, err := os.Stat(custom); err != nil {
		t.Fatalf("custom command was removed: %v", err)
	}
}

func TestSetupOpenCodePreservesCustomCommandWithConflictingName(t *testing.T) {
	home := t.TempDir()
	commands := filepath.Join(home, ".config", "opencode", "commands")
	if err := os.MkdirAll(commands, 0o755); err != nil {
		t.Fatal(err)
	}
	custom := filepath.Join(commands, "plan.md")
	const content = "My custom plan command."
	if err := os.WriteFile(custom, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	setupOpenCode(home, "/usr/local/bin/detritus", false)

	got, err := os.ReadFile(custom)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Fatalf("custom command was overwritten: %q", got)
	}
}

func TestSetupOpenCodePreservesCustomCommandThatUsesKBGet(t *testing.T) {
	home := t.TempDir()
	commands := filepath.Join(home, ".config", "opencode", "commands")
	if err := os.MkdirAll(commands, 0o755); err != nil {
		t.Fatal(err)
	}
	custom := filepath.Join(commands, "truthseeker.md")
	const content = "Call the detritus MCP tool `kb_get` with `name=\"custom\"`."
	if err := os.WriteFile(custom, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	setupOpenCode(home, "/usr/local/bin/detritus", false)

	got, err := os.ReadFile(custom)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Fatalf("custom command was overwritten: %q", got)
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

// TestGenerateClaudeReviewerAgentPinsModelAndEffort verifies the review-loop
// reviewer subagent definition pins its model and effort per ROLE: it runs on
// claude-fable-5 at high effort, independent of the session model, and never
// restricts tools (it needs kb_get + Read/Bash/Grep to verify).
func TestGenerateClaudeReviewerAgentPinsModelAndEffort(t *testing.T) {
	home := t.TempDir()

	generateClaudeReviewerAgent(home)

	agentFile := filepath.Join(home, ".claude", "agents", "detritus-reviewer.md")
	raw, err := os.ReadFile(agentFile)
	if err != nil {
		t.Fatalf("reviewer agent definition not written: %v", err)
	}
	got := string(raw)

	fm, _, ok := strings.Cut(strings.TrimPrefix(got, "---\n"), "\n---")
	if !ok {
		t.Fatalf("agent definition missing YAML frontmatter:\n%s", got)
	}
	if !strings.Contains(fm, "name: detritus-reviewer") {
		t.Errorf("agent must be named detritus-reviewer; got:\n%s", fm)
	}
	if !strings.Contains(fm, "\nmodel: claude-fable-5") {
		t.Errorf("reviewer must pin model to claude-fable-5; got:\n%s", fm)
	}
	if !strings.Contains(fm, "\neffort: high") {
		t.Errorf("reviewer must set the effort key to high; got:\n%s", fm)
	}
	// A `tools:` restriction strips the built-in tools the reviewer needs to verify.
	if strings.Contains(fm, "tools:") {
		t.Errorf("frontmatter must not restrict tools; got:\n%s", fm)
	}
	if !strings.Contains(got, "roles/reviewer") {
		t.Errorf("body must direct the reviewer to load roles/reviewer; got:\n%s", got)
	}
}

// TestClaudeDryRunPreviewsEveryAgentFile verifies the --dry-run branch of
// setupClaudeCode mirrors what a real run writes: every agent definition the
// real path installs must have a matching "[dry-run] Would write ..." preview
// line. Adding a generator without its preview line makes --dry-run under-report
// what setup does, and nothing else in the suite catches that divergence.
func TestClaudeDryRunPreviewsEveryAgentFile(t *testing.T) {
	home := t.TempDir()

	// The real path's agent writers — keep in sync with setupClaudeCode's non-dry-run branch.
	generateClaudeCoderAgent(home)
	generateClaudeReviewerAgent(home)
	generateClaudeReviewerPRAgent(home)

	agentsDir := filepath.Join(home, ".claude", "agents")
	written, err := os.ReadDir(agentsDir)
	if err != nil {
		t.Fatalf("agents dir not written: %v", err)
	}
	if len(written) == 0 {
		t.Fatal("no agent definitions written; the generators under test did nothing")
	}

	src, err := os.ReadFile("setup.go")
	if err != nil {
		t.Fatalf("read setup.go: %v", err)
	}
	// Scope the search to setupClaudeCode's dry-run branch. Searching the whole
	// file would be vacuous: each agent's base name also appears in its own
	// generator, so the assertion would pass with the preview line deleted.
	const branchStart = "func setupClaudeCode("
	i := strings.Index(string(src), branchStart)
	if i < 0 {
		t.Fatalf("setupClaudeCode not found in setup.go; test needs updating")
	}
	rest := string(src)[i:]
	end := strings.Index(rest, "\n\tupsertMCP(")
	if end < 0 {
		t.Fatalf("could not delimit the dry-run branch in setupClaudeCode; test needs updating")
	}
	dryRunBranch := rest[:end]
	if !strings.Contains(dryRunBranch, "[dry-run]") {
		t.Fatalf("delimited region is not the dry-run branch:\n%s", dryRunBranch)
	}

	for _, entry := range written {
		// The preview builds the path via filepath.Join(..., "agents", "<name>.md"),
		// so the quoted base name must appear inside the dry-run branch itself.
		quoted := strconv.Quote(entry.Name())
		if !strings.Contains(dryRunBranch, quoted) {
			t.Errorf("--dry-run does not preview %s: no %s in setupClaudeCode's dry-run branch, but a real run writes it", entry.Name(), quoted)
		}
	}
}

// TestGenerateClaudeReviewerPRAgentInheritsModelKeepsEffort verifies the
// PR-review reviewer definition (/gh-pr, /gh-pr-safe) is the UNPINNED
// counterpart: it inherits the session model — never the claude-fable-5 pin,
// which is the whole point of conserving the Fable allowance — while keeping
// high effort and the same review obligations as detritus-reviewer.
func TestGenerateClaudeReviewerPRAgentInheritsModelKeepsEffort(t *testing.T) {
	home := t.TempDir()

	generateClaudeReviewerPRAgent(home)

	agentFile := filepath.Join(home, ".claude", "agents", "detritus-reviewer-pr.md")
	raw, err := os.ReadFile(agentFile)
	if err != nil {
		t.Fatalf("PR-review reviewer agent definition not written: %v", err)
	}
	got := string(raw)

	fm, _, ok := strings.Cut(strings.TrimPrefix(got, "---\n"), "\n---")
	if !ok {
		t.Fatalf("agent definition missing YAML frontmatter:\n%s", got)
	}
	if !strings.Contains(fm, "name: detritus-reviewer-pr") {
		t.Errorf("agent must be named detritus-reviewer-pr so /gh-pr can spawn it; got:\n%s", fm)
	}
	if !strings.Contains(fm, "\nmodel: inherit") {
		t.Errorf("PR-review reviewer must inherit the session model; got:\n%s", fm)
	}
	// The de-pin is the point: a fable pin here would burn the allowance this agent exists to conserve.
	if strings.Contains(fm, "claude-fable-5") {
		t.Errorf("PR-review reviewer must NOT pin claude-fable-5; got:\n%s", fm)
	}
	if !strings.Contains(fm, "\neffort: high") {
		t.Errorf("PR-review reviewer must keep effort high — only the model pin is dropped; got:\n%s", fm)
	}
	if strings.Contains(fm, "tools:") {
		t.Errorf("frontmatter must not restrict tools; got:\n%s", fm)
	}
	// Same rigor obligations as the pinned reviewer — this must not be a watered-down
	// copy. Each assertion below names the exact obligation it guards: a bare
	// `strings.Contains(got, "roles/reviewer")` would be satisfied by unrelated prose
	// elsewhere in the file, so match the kb_get directive itself.
	if !strings.Contains(got, "`kb_get name=\"roles/reviewer\"`") {
		t.Errorf("body must direct the reviewer to load its role doc via kb_get; got:\n%s", got)
	}
	for _, doc := range []string{"core/review-rigor", "flows/principles/truthseeker"} {
		if !strings.Contains(got, doc) {
			t.Errorf("body must compose %s like the pinned reviewer does; got:\n%s", doc, got)
		}
	}
	if !strings.Contains(got, "Never edit files, stage, commit, push, or post") {
		t.Errorf("body must carry the review-only prohibition; got:\n%s", got)
	}
	if !strings.Contains(got, "Review stamps") {
		t.Errorf("body must reference the review-rigor Review stamps section; got:\n%s", got)
	}
	if !strings.Contains(got, "self-report") {
		t.Errorf("body must direct the reviewer to self-report the model it runs on; got:\n%s", got)
	}
}

// TestGenerateClaudeReviewerAgentSelfReportsProvenance verifies the reviewer
// body directs the reviewer to self-report the model it is actually running on
// plus the two review-stamp lines, so a silent model degrade (an old CLI
// falling off the claude-fable-5 pin) is visible in the returned verdict.
func TestGenerateClaudeReviewerAgentSelfReportsProvenance(t *testing.T) {
	home := t.TempDir()

	generateClaudeReviewerAgent(home)

	raw, err := os.ReadFile(filepath.Join(home, ".claude", "agents", "detritus-reviewer.md"))
	if err != nil {
		t.Fatalf("reviewer agent definition not written: %v", err)
	}
	got := string(raw)

	if !strings.Contains(got, "Review stamps") {
		t.Errorf("body must reference the review-rigor Review stamps section; got:\n%s", got)
	}
	if !strings.Contains(got, "self-report") {
		t.Errorf("body must direct the reviewer to self-report the model it runs on; got:\n%s", got)
	}
}

// The Copilot agent has the same contract: its frontmatter `tools` is a strict
// allowlist (omitted = all tools), so a coding agent must not be pinned to the
// detritus MCP server alone.
func TestGenerateAgentFileInheritsTools(t *testing.T) {
	home := t.TempDir()

	generateAgentFile(home, false)

	raw, err := os.ReadFile(filepath.Join(home, ".copilot", "agents", "detritus.agent.md"))
	if err != nil {
		t.Fatalf("copilot agent definition not written: %v", err)
	}
	fm, _, ok := strings.Cut(strings.TrimPrefix(string(raw), "---\n"), "\n---")
	if !ok {
		t.Fatalf("agent definition missing YAML frontmatter:\n%s", raw)
	}
	if !strings.Contains(fm, "name: detritus") {
		t.Errorf("agent must keep its name; got:\n%s", fm)
	}
	if strings.Contains(fm, "tools:") {
		t.Errorf("frontmatter must not restrict tools (allowlist semantics strip read/search/edit); got:\n%s", fm)
	}
}
