package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTodoGuardResponse(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		wantDen bool
	}{
		{
			name:    "main session edits store -> deny",
			payload: `{"tool_name":"Edit","tool_input":{"file_path":"C:\\Users\\x\\.claude\\projects\\slug\\todos.json"}}`,
			wantDen: true,
		},
		{
			name:    "main session edits store posix path -> deny",
			payload: `{"tool_name":"Write","tool_input":{"file_path":"/home/u/.claude/projects/slug/todos.json"}}`,
			wantDen: true,
		},
		{
			name:    "sub-agent edits store (agent_id) -> allow",
			payload: `{"tool_name":"Edit","agent_id":"a1","tool_input":{"file_path":"/home/u/.claude/projects/slug/todos.json"}}`,
			wantDen: false,
		},
		{
			// agent_type without agent_id is NOT a real sub-agent — this is the
			// --agent-launched main session shape; keying on agent_id alone must
			// still deny it (closes the agent_type bypass).
			name:    "agent_type only, no agent_id (--agent main) -> deny",
			payload: `{"tool_name":"Edit","agent_type":"general-purpose","tool_input":{"file_path":"/home/u/.claude/projects/slug/todos.json"}}`,
			wantDen: true,
		},
		{
			name:    "sub-agent with both agent_id + agent_type -> allow",
			payload: `{"tool_name":"Edit","agent_id":"a1","agent_type":"general-purpose","tool_input":{"file_path":"/home/u/.claude/projects/slug/todos.json"}}`,
			wantDen: false,
		},
		{
			name:    "main session edits unrelated file -> allow",
			payload: `{"tool_name":"Edit","tool_input":{"file_path":"/home/u/project/main.go"}}`,
			wantDen: false,
		},
		{
			name:    "todos.json outside .claude -> allow",
			payload: `{"tool_name":"Edit","tool_input":{"file_path":"/home/u/project/todos.json"}}`,
			wantDen: false,
		},
		{
			name:    "no file_path -> allow",
			payload: `{"tool_name":"Bash","tool_input":{"command":"ls"}}`,
			wantDen: false,
		},
		{
			name:    "garbage stdin -> allow (fail open)",
			payload: `not json`,
			wantDen: false,
		},
		{
			name:    "empty stdin -> allow (fail open)",
			payload: ``,
			wantDen: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := todoGuardResponse([]byte(tc.payload))
			gotDeny := out != nil
			if gotDeny != tc.wantDen {
				t.Fatalf("deny = %v, want %v (out=%q)", gotDeny, tc.wantDen, string(out))
			}
			if gotDeny && !strings.Contains(string(out), `"permissionDecision":"deny"`) {
				t.Fatalf("deny output missing decision: %q", string(out))
			}
		})
	}
}

// countTodoGuards returns how many PreToolUse hooks reference the guard marker.
func countTodoGuards(t *testing.T, settingsFile string) int {
	t.Helper()
	raw, err := os.ReadFile(settingsFile)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("settings is not valid JSON after write: %v\n%s", err, raw)
	}
	hooks, _ := data["hooks"].(map[string]any)
	pre, _ := hooks["PreToolUse"].([]any)
	n := 0
	for _, g := range pre {
		group, _ := g.(map[string]any)
		inner, _ := group["hooks"].([]any)
		for _, h := range inner {
			hm, _ := h.(map[string]any)
			if c, _ := hm["command"].(string); strings.Contains(c, todoGuardMarker) {
				n++
			}
		}
	}
	return n
}

func TestSetupClaudeTodoGuard(t *testing.T) {
	home := t.TempDir()
	settingsFile := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsFile), 0o755); err != nil {
		t.Fatal(err)
	}

	// Seed a curated settings.json with a deny list and an unrelated PreToolUse hook.
	seed := `{
  "effortLevel": "max",
  "permissions": { "deny": ["Bash(rm -rf /)", "Bash(shutdown*)"] },
  "hooks": {
    "PreToolUse": [
      { "matcher": "Bash", "hooks": [ { "type": "command", "command": "echo audit" } ] }
    ]
  }
}`
	if err := os.WriteFile(settingsFile, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	bin := filepath.Join("/opt", "detritus")

	// Install.
	setupClaudeTodoGuard(home, bin, true, false)
	if got := countTodoGuards(t, settingsFile); got != 1 {
		t.Fatalf("after install: %d guard entries, want 1", got)
	}

	// Idempotent: re-run does not duplicate.
	setupClaudeTodoGuard(home, bin, true, false)
	if got := countTodoGuards(t, settingsFile); got != 1 {
		t.Fatalf("after re-install: %d guard entries, want 1", got)
	}

	// Existing unrelated hook and curated keys are preserved.
	raw, _ := os.ReadFile(settingsFile)
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if data["effortLevel"] != "max" {
		t.Fatalf("effortLevel lost: %v", data["effortLevel"])
	}
	perms, _ := data["permissions"].(map[string]any)
	deny, _ := perms["deny"].([]any)
	if len(deny) != 2 {
		t.Fatalf("deny list mangled: %v", deny)
	}
	if !strings.Contains(string(raw), "echo audit") {
		t.Fatalf("unrelated Bash hook lost:\n%s", raw)
	}

	// Binary path change updates the entry in place (still one).
	setupClaudeTodoGuard(home, filepath.Join("/usr", "local", "bin", "detritus"), true, false)
	if got := countTodoGuards(t, settingsFile); got != 1 {
		t.Fatalf("after path change: %d guard entries, want 1", got)
	}
	raw, _ = os.ReadFile(settingsFile)
	if !strings.Contains(string(raw), "/usr/local/bin/detritus") {
		t.Fatalf("command not updated to new binary path:\n%s", raw)
	}

	// Uninstall removes our entry but keeps the unrelated one.
	setupClaudeTodoGuard(home, bin, false, false)
	if got := countTodoGuards(t, settingsFile); got != 0 {
		t.Fatalf("after uninstall: %d guard entries, want 0", got)
	}
	raw, _ = os.ReadFile(settingsFile)
	if !strings.Contains(string(raw), "echo audit") {
		t.Fatalf("unrelated hook removed during uninstall:\n%s", raw)
	}
}

// guardMatcher returns the matcher of the PreToolUse group whose hook command
// carries the guard marker, or "" if no such group exists.
func guardMatcher(t *testing.T, settingsFile string) string {
	t.Helper()
	raw, err := os.ReadFile(settingsFile)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	hooks, _ := data["hooks"].(map[string]any)
	pre, _ := hooks["PreToolUse"].([]any)
	for _, g := range pre {
		group, _ := g.(map[string]any)
		inner, _ := group["hooks"].([]any)
		for _, h := range inner {
			hm, _ := h.(map[string]any)
			if c, _ := hm["command"].(string); strings.Contains(c, todoGuardMarker) {
				m, _ := group["matcher"].(string)
				return m
			}
		}
	}
	return ""
}

func TestSetupClaudeTodoGuardMatcher(t *testing.T) {
	home := t.TempDir()
	settingsFile := filepath.Join(home, ".claude", "settings.json")

	setupClaudeTodoGuard(home, "/opt/detritus", true, false)

	// The matcher is the single value that decides whether the guard fires on
	// Edit/Write/MultiEdit at all — assert it explicitly, not just the count.
	if got := guardMatcher(t, settingsFile); got != "Edit|Write|MultiEdit" {
		t.Fatalf("guard matcher = %q, want %q", got, "Edit|Write|MultiEdit")
	}
}

func TestSetupClaudeTodoGuardStripsFromSharedGroup(t *testing.T) {
	home := t.TempDir()
	settingsFile := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsFile), 0o755); err != nil {
		t.Fatal(err)
	}

	// A group containing BOTH a stale detritus guard hook AND an unrelated
	// sibling hook — the strip-but-keep-sibling branch (kept>0 && kept<inner).
	seed := `{
  "hooks": {
    "PreToolUse": [
      { "matcher": "Edit|Write|MultiEdit", "hooks": [
        { "type": "command", "command": "\"/old/detritus\" --todo-guard" },
        { "type": "command", "command": "echo sibling" }
      ] }
    ]
  }
}`
	if err := os.WriteFile(settingsFile, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	setupClaudeTodoGuard(home, "/new/detritus", true, false)

	// Exactly one guard entry survives, and the sibling is preserved.
	if got := countTodoGuards(t, settingsFile); got != 1 {
		t.Fatalf("guard entries = %d, want 1", got)
	}
	raw, _ := os.ReadFile(settingsFile)
	if !strings.Contains(string(raw), "echo sibling") {
		t.Fatalf("sibling hook lost when stripping shared group:\n%s", raw)
	}
	if strings.Contains(string(raw), "/old/detritus") {
		t.Fatalf("stale guard command not stripped:\n%s", raw)
	}
	if !strings.Contains(string(raw), "/new/detritus") {
		t.Fatalf("fresh guard command not installed:\n%s", raw)
	}
}

func TestSetupClaudeTodoGuardLeavesInvalidJSONUntouched(t *testing.T) {
	home := t.TempDir()
	settingsFile := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsFile), 0o755); err != nil {
		t.Fatal(err)
	}

	// A present-but-unparseable settings.json must be left byte-for-byte intact
	// (fail safe — never clobber a file we can't understand).
	bad := "{ this is not valid json"
	if err := os.WriteFile(settingsFile, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}

	setupClaudeTodoGuard(home, "/opt/detritus", true, false)

	got, err := os.ReadFile(settingsFile)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != bad {
		t.Fatalf("invalid settings.json was modified:\nwant %q\ngot  %q", bad, string(got))
	}
}

func TestSetupClaudeTodoGuardCreatesFile(t *testing.T) {
	home := t.TempDir()
	settingsFile := filepath.Join(home, ".claude", "settings.json")

	// No settings.json yet — installer should create a valid one.
	setupClaudeTodoGuard(home, "/opt/detritus", true, false)
	if got := countTodoGuards(t, settingsFile); got != 1 {
		t.Fatalf("after install on missing file: %d guard entries, want 1", got)
	}
}

func TestHasTodoDoc(t *testing.T) {
	if !hasTodoDoc([]docEntry{{name: "meta/todo"}, {name: "plan/index"}}) {
		t.Fatal("expected hasTodoDoc true when meta/todo present")
	}
	if hasTodoDoc([]docEntry{{name: "plan/index"}}) {
		t.Fatal("expected hasTodoDoc false when meta/todo absent")
	}
}
