package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// todoGuardMarker is the stable substring detritus writes into the hook command
// so re-runs (and uninstalls) can find the entry regardless of where the binary
// lives. Enforces meta/todo convention #13: only a delegated sub-agent may write
// the cross-session todos.json store; a main-session Edit/Write is denied.
const todoGuardMarker = "--todo-guard"

// runTodoGuard is the PreToolUse hook handler. It reads the hook payload from
// stdin and, if the call is a MAIN-session Edit/Write/MultiEdit of a
// .claude/**/todos.json store, prints a deny decision. Sub-agent calls (which
// carry agent_id/agent_type) and all other paths are allowed. Fails OPEN on any
// parse error so it can never wedge unrelated edits.
func runTodoGuard() error {
	raw, _ := io.ReadAll(os.Stdin)
	if out := todoGuardResponse(raw); out != nil {
		os.Stdout.Write(out)
	}
	return nil
}

// todoGuardResponse returns the deny-decision JSON bytes when the payload is a
// main-session write to the todo store, or nil to allow. Pure function so the
// decision is unit-testable without stdin/stdout plumbing.
func todoGuardResponse(raw []byte) []byte {
	var payload struct {
		AgentID   string         `json:"agent_id"`
		ToolInput map[string]any `json:"tool_input"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil // fail open
	}

	fp, _ := payload.ToolInput["file_path"].(string)
	if fp == "" {
		return nil // not a path-bearing edit
	}

	n := strings.ToLower(strings.ReplaceAll(fp, "\\", "/"))
	isStore := strings.Contains(n, "/.claude/") && strings.HasSuffix(n, "/todos.json")
	if !isStore {
		return nil // not the todo store
	}

	// agent_id is Claude Code's canonical sub-agent discriminator: per the hook
	// docs it is present ONLY when the hook fires inside a delegated sub-agent
	// (Task/Agent), and the docs say to "use this to distinguish subagent hook
	// calls from main-thread calls". A --agent-launched MAIN session carries
	// neither agent_id nor agent_type, so keying on agent_id alone both allows
	// every real sub-agent write and still denies the main session. agent_type
	// (the agent *name*) is intentionally NOT trusted — it is a weaker signal
	// that could be set by other means, which would open a main-session bypass.
	if payload.AgentID != "" {
		return nil
	}

	out, _ := json.Marshal(map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":            "PreToolUse",
			"permissionDecision":       "deny",
			"permissionDecisionReason": "Blocked by detritus /todo guard: todos.json is the /todo cross-session store and may only be mutated by a delegated sub-agent (todo convention #13). The main session must not Edit/Write it directly — route the change through a /todo-* skill, which spawns a sub-agent to perform the epoch-checked write.",
		},
	})
	return out
}

// hasTodoDoc reports whether this build ships the /todo skill family. The write
// guard is installed only when /todo is present, so the hook's lifecycle tracks
// the skill tree: a future build that drops /todo will also drop the guard.
func hasTodoDoc(docs []docEntry) bool {
	for _, d := range docs {
		if d.name == "meta/todo" {
			return true
		}
	}
	return false
}

// setupClaudeTodoGuard installs (install=true) or removes (install=false) the
// PreToolUse hook that enforces convention #13 in ~/.claude/settings.json.
//
// Idempotent and self-healing: every existing detritus todo-guard entry is
// stripped first (matched by todoGuardMarker), then a single fresh entry is
// appended when install is true. This updates the command in place if the
// binary moved, never duplicates, and leaves all non-detritus hooks untouched.
// Fails safe: if settings.json is present but unparseable, it is left alone.
func setupClaudeTodoGuard(home, binaryPath string, install, dryRun bool) {
	settingsFile := filepath.Join(home, ".claude", "settings.json")
	command := `"` + binaryPath + `" ` + todoGuardMarker

	if dryRun {
		if install {
			fmt.Printf("[dry-run] Would install /todo write-guard PreToolUse hook in %s\n", settingsFile)
		} else {
			fmt.Printf("[dry-run] Would remove /todo write-guard PreToolUse hook from %s (if present)\n", settingsFile)
		}
		return
	}

	data := map[string]any{}
	if rawBytes, err := os.ReadFile(settingsFile); err == nil && len(rawBytes) > 0 {
		if err := json.Unmarshal(rawBytes, &data); err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s is not valid JSON; skipping /todo guard: %v\n", settingsFile, err)
			return
		}
	}

	hooks, _ := data["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	pre, _ := hooks["PreToolUse"].([]any)

	// Strip any existing detritus todo-guard hook from every group. Groups we
	// don't touch are appended verbatim — including malformed ones (no []any
	// "hooks") — so the installer never rewrites or reshapes a user's hooks.
	cleaned := make([]any, 0, len(pre))
	for _, g := range pre {
		group, ok := g.(map[string]any)
		if !ok {
			cleaned = append(cleaned, g)
			continue
		}
		inner, ok := group["hooks"].([]any)
		if !ok {
			cleaned = append(cleaned, group) // no hooks array we understand — leave as-is
			continue
		}
		kept := make([]any, 0, len(inner))
		for _, h := range inner {
			if hm, ok := h.(map[string]any); ok {
				if c, _ := hm["command"].(string); strings.Contains(c, todoGuardMarker) {
					continue // drop our previous entry
				}
			}
			kept = append(kept, h)
		}
		if len(kept) == len(inner) {
			cleaned = append(cleaned, group) // removed nothing — leave untouched
			continue
		}
		if len(kept) == 0 {
			continue // we emptied it — drop the now-empty group
		}
		group["hooks"] = kept // removed ours; siblings remain
		cleaned = append(cleaned, group)
	}

	if install {
		cleaned = append(cleaned, map[string]any{
			"matcher": "Edit|Write|MultiEdit",
			"hooks": []any{
				map[string]any{"type": "command", "command": command},
			},
		})
	}

	if len(cleaned) > 0 {
		hooks["PreToolUse"] = cleaned
	} else {
		delete(hooks, "PreToolUse")
	}
	if len(hooks) > 0 {
		data["hooks"] = hooks
	} else {
		delete(data, "hooks")
	}

	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: marshal %s: %v\n", settingsFile, err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(settingsFile), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "warning: claude settings dir: %v\n", err)
		return
	}
	if err := os.WriteFile(settingsFile, append(out, '\n'), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "warning: write %s: %v\n", settingsFile, err)
		return
	}
	if install {
		fmt.Printf("Claude Code /todo write-guard hook: %s\n", settingsFile)
	} else {
		fmt.Printf("Claude Code /todo write-guard hook removed: %s\n", settingsFile)
	}
}
