package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resolveCandylandDelivery is the two-tier feedback classifier: an explicit PR
// reference in the plan text wins (via the shared gh-mirror classifier — here gh
// is absent so it degrades to marker derivation, which still selects feedback OR
// review); absent a reference, an open PR on the current branch selects feedback;
// otherwise new work.
func TestResolveCandylandDelivery(t *testing.T) {
	orig := branchPRLookup
	defer func() { branchPRLookup = orig }()

	cases := []struct {
		name       string
		prompt     string
		branchPR   int
		wantDelivr string
		wantPR     int
	}{
		{"text feedback marker wins over branch", "address feedback on PR #7", 99, "feedback", 7},
		{"text review marker wins over branch", "review PR #8", 99, "review", 8},
		{"no marker + open branch PR → feedback", "build a new thing", 42, "feedback", 42},
		{"no marker + no branch PR → new work", "build a new thing", 0, "pr", 0},
		{"review-style prose without a PR number falls back to branch", "please review this", 42, "feedback", 42},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			installGHStub(t, map[string]string{}) // no gh state → degrade to marker derivation
			branchPRLookup = func(string) int { return tc.branchPR }
			deliver, pr, _, err := resolveCandylandDelivery(tc.prompt, t.TempDir())
			if err != nil {
				t.Fatalf("resolveCandylandDelivery: %v", err)
			}
			if deliver != tc.wantDelivr || pr != tc.wantPR {
				t.Errorf("resolveCandylandDelivery = %q/%d, want %q/%d", deliver, pr, tc.wantDelivr, tc.wantPR)
			}
		})
	}
}

// parseGHPRNumber maps clean integers to a PR number and every degrade case
// (empty = no OPEN PR matched, "null", whitespace, junk) to 0 — so branch-state
// inference never fails the run or invents a target.
func TestParseGHPRNumber(t *testing.T) {
	cases := map[string]int{"42": 42, "  7\n": 7, "": 0, "null": 0, "   ": 0, "abc": 0, "3.5": 0}
	for in, want := range cases {
		if got := parseGHPRNumber(in); got != want {
			t.Errorf("parseGHPRNumber(%q) = %d, want %d", in, got, want)
		}
	}
}

// readCandylandRunArgs reads the prompt from the file and defaults folders to
// [cwd] when none are passed, while preserving explicit folders.
func TestReadCandylandRunArgs(t *testing.T) {
	dir := t.TempDir()
	planFile := filepath.Join(dir, "plan.md")
	if err := os.WriteFile(planFile, []byte("# the plan\nbuild it"), 0o644); err != nil {
		t.Fatal(err)
	}

	// No folders given → defaults to cwd.
	prompt, folders, err := readCandylandRunArgs(planFile, nil, "/work/repo")
	if err != nil {
		t.Fatalf("readCandylandRunArgs: %v", err)
	}
	if prompt != "# the plan\nbuild it" {
		t.Errorf("prompt = %q, want the file contents", prompt)
	}
	if len(folders) != 1 || folders[0] != "/work/repo" {
		t.Errorf("folders = %v, want [/work/repo] (cwd default)", folders)
	}

	// Explicit folders preserved.
	_, folders2, err := readCandylandRunArgs(planFile, []string{"/a", "/b"}, "/work/repo")
	if err != nil {
		t.Fatalf("readCandylandRunArgs: %v", err)
	}
	if len(folders2) != 2 || folders2[0] != "/a" || folders2[1] != "/b" {
		t.Errorf("explicit folders not preserved: %v", folders2)
	}

	// Missing prompt file → error.
	if _, _, err := readCandylandRunArgs(filepath.Join(dir, "nope.md"), nil, "/work/repo"); err == nil {
		t.Error("a missing prompt file should error")
	}
}

// startCandylandRun POSTs the run, reads back the id, then begins it — sending
// folders/prompt/title and NO mode field, and hitting /api/runs then
// /api/runs/{id}/begin. Driven against a mock server, no real candyland needed.
func TestStartCandylandRun(t *testing.T) {
	var createdBody map[string]any
	beganID := ""

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/runs":
			defer r.Body.Close()
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &createdBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"run-123"}`))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/runs/") && strings.HasSuffix(r.URL.Path, "/begin"):
			beganID = strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/runs/"), "/begin")
			w.WriteHeader(http.StatusNoContent) // 204, matching candyland's real /begin handler
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	id, err := startCandylandRunAt(srv.URL, []string{"/repo/a", "/repo/b"}, "do the thing", "My Run", "", 0)
	if err != nil {
		t.Fatalf("startCandylandRun: %v", err)
	}
	if id != "run-123" {
		t.Errorf("run id = %q, want run-123", id)
	}
	if beganID != "run-123" {
		t.Errorf("begin was called for %q, want run-123", beganID)
	}
	if _, hasMode := createdBody["mode"]; hasMode {
		t.Error("request body must NOT include a mode field")
	}
	if createdBody["prompt"] != "do the thing" || createdBody["title"] != "My Run" {
		t.Errorf("prompt/title not sent correctly: %v", createdBody)
	}
	// A plain run carries no delivery override — deliver/targetPr omitted so
	// candyland uses its default new-PR-per-repo delivery.
	if _, has := createdBody["deliver"]; has {
		t.Errorf("plain run must not send a deliver field: %v", createdBody)
	}
	if _, has := createdBody["targetPr"]; has {
		t.Errorf("plain run must not send a targetPr field: %v", createdBody)
	}
	folders, _ := createdBody["folders"].([]any)
	if len(folders) != 2 || folders[0] != "/repo/a" {
		t.Errorf("folders not sent correctly: %v", createdBody["folders"])
	}
}

// A feedback run threads deliver+targetPr through to /api/runs so candyland
// updates the existing PR in place instead of opening a new one.
func TestStartCandylandRunFeedback(t *testing.T) {
	var createdBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/runs":
			defer r.Body.Close()
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &createdBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"run-9"}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/begin"):
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	if _, err := startCandylandRunAt(srv.URL, []string{"/repo"}, "fix it", "", "feedback", 42); err != nil {
		t.Fatalf("startCandylandRun: %v", err)
	}
	if createdBody["deliver"] != "feedback" {
		t.Errorf("deliver = %v, want feedback", createdBody["deliver"])
	}
	if createdBody["targetPr"] != float64(42) {
		t.Errorf("targetPr = %v, want 42", createdBody["targetPr"])
	}
}

// The start-quest wire test lives in candyland_client_classify_test.go
// (TestStartCandylandQuestSendsConvergenceAndTitle): it asserts the current
// title + convergence contract and the absence of any autonomy field.

// readQuestRunArgs reads the objective from the file and defaults folders to
// [cwd] when none are passed, while preserving explicit folders.
func TestReadQuestRunArgs(t *testing.T) {
	dir := t.TempDir()
	objFile := filepath.Join(dir, "objective.md")
	if err := os.WriteFile(objFile, []byte("# objective\nkeep it green"), 0o644); err != nil {
		t.Fatal(err)
	}

	objective, folders, err := readQuestRunArgs(objFile, nil, "/work/repo")
	if err != nil {
		t.Fatalf("readQuestRunArgs: %v", err)
	}
	if objective != "# objective\nkeep it green" {
		t.Errorf("objective = %q, want the file contents", objective)
	}
	if len(folders) != 1 || folders[0] != "/work/repo" {
		t.Errorf("folders = %v, want [/work/repo] (cwd default)", folders)
	}

	_, folders2, err := readQuestRunArgs(objFile, []string{"/a", "/b"}, "/work/repo")
	if err != nil {
		t.Fatalf("readQuestRunArgs: %v", err)
	}
	if len(folders2) != 2 || folders2[0] != "/a" || folders2[1] != "/b" {
		t.Errorf("explicit folders not preserved: %v", folders2)
	}

	if _, _, err := readQuestRunArgs(filepath.Join(dir, "nope.md"), nil, "/work/repo"); err == nil {
		t.Error("a missing objective file should error")
	}
}

// mcpServersFromConfig merges the global mcpServers with the project-scoped map
// for cwd (project winning on collisions) and drops detritus/candyland.
func TestMCPServersFromConfig(t *testing.T) {
	data := map[string]any{
		"mcpServers": map[string]any{
			"detritus":  map[string]any{"command": "detritus"},
			"candyland": map[string]any{"command": "candyland"},
			"obsidian":  map[string]any{"command": "obsidian", "args": []any{"--global"}},
			"shared":    map[string]any{"command": "global-shared"},
		},
		"projects": map[string]any{
			"/work/repo": map[string]any{
				"mcpServers": map[string]any{
					"projtool": map[string]any{"command": "projtool"},
					"shared":   map[string]any{"command": "project-shared"},
				},
			},
			"/other": map[string]any{
				"mcpServers": map[string]any{"nope": map[string]any{"command": "nope"}},
			},
		},
	}

	got := mcpServersFromConfig(data, "/work/repo")

	if _, has := got["detritus"]; has {
		t.Error("detritus must be excluded from inherited servers")
	}
	if _, has := got["candyland"]; has {
		t.Error("candyland must be excluded from inherited servers")
	}
	if _, has := got["nope"]; has {
		t.Error("a different project's servers must not leak in")
	}
	if _, has := got["obsidian"]; !has {
		t.Error("global obsidian server should be inherited")
	}
	if _, has := got["projtool"]; !has {
		t.Error("project-scoped server should be inherited")
	}
	shared, _ := got["shared"].(map[string]any)
	if shared["command"] != "project-shared" {
		t.Errorf("project scope should win on key collision: shared.command = %v, want project-shared", shared["command"])
	}
}

// readOriginMCPServers degrades to an empty map (never errors) for a missing
// path, an empty path, or unparseable JSON — inheriting must never fail a launch.
func TestReadOriginMCPServers(t *testing.T) {
	if got := readOriginMCPServers("", "/work/repo"); len(got) != 0 {
		t.Errorf("empty path should yield no servers, got %v", got)
	}

	dir := t.TempDir()
	if got := readOriginMCPServers(filepath.Join(dir, "nope.json"), "/work/repo"); len(got) != 0 {
		t.Errorf("missing file should yield no servers, got %v", got)
	}

	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readOriginMCPServers(bad, "/work/repo"); len(got) != 0 {
		t.Errorf("unparseable config should yield no servers, got %v", got)
	}

	good := filepath.Join(dir, "claude.json")
	if err := os.WriteFile(good, []byte(`{"mcpServers":{"obsidian":{"command":"obsidian"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := readOriginMCPServers(good, "/work/repo")
	if _, has := got["obsidian"]; !has {
		t.Errorf("obsidian server should be read from config, got %v", got)
	}
}

// writeOriginMCPConfigFile writes the servers to a private temp file and returns
// its PATH (not inline JSON). Its VALUE for the env var must be a readable path
// to a 0600 file whose content is an --mcp-config-shaped object; no servers
// yields ("", nil) so the caller omits the env var entirely.
func TestWriteOriginMCPConfigFile(t *testing.T) {
	if path, err := writeOriginMCPConfigFile(map[string]any{}); err != nil || path != "" {
		t.Errorf("no servers should yield (\"\", nil), got (%q, %v)", path, err)
	}
	if path, err := writeOriginMCPConfigFile(nil); err != nil || path != "" {
		t.Errorf("nil servers should yield (\"\", nil), got (%q, %v)", path, err)
	}

	path, err := writeOriginMCPConfigFile(map[string]any{"obsidian": map[string]any{"command": "obsidian", "env": map[string]any{"TOKEN": "secret"}}})
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	defer os.Remove(path)

	// The returned value is a path to a real file, not inline JSON.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("returned path is not a readable file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("temp file should be 0600, got %v", info.Mode().Perm())
	}
	if json.Valid([]byte(path)) {
		t.Errorf("env value should be a path, not inline JSON, got %q", path)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("file content is not valid JSON: %v", err)
	}
	servers, ok := parsed["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("file missing mcpServers key: %v", parsed)
	}
	if _, has := servers["obsidian"]; !has {
		t.Errorf("mcpServers should contain obsidian, got %v", servers)
	}
}

func TestTitleFriendlyPrompt(t *testing.T) {
	cases := []struct {
		name, in, wantFirst string
	}{
		{"markdown heading", "# Plan: quest orchestration\n\nbody here", "quest orchestration"},
		{"heading no label", "## Fix the emoji squares\n\nbody", "Fix the emoji squares"},
		{"plain first line untouched", "Add typed cancellation\n\nbody", "Add typed cancellation"},
		{"leading blank lines", "\n\n# Plan: do the thing\nbody", "do the thing"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := titleFriendlyPrompt(c.in)
			first := ""
			for _, ln := range strings.Split(got, "\n") {
				if strings.TrimSpace(ln) != "" {
					first = strings.TrimSpace(ln)
					break
				}
			}
			if first != c.wantFirst {
				t.Errorf("first line = %q, want %q", first, c.wantFirst)
			}
			// The body must survive untouched.
			if !strings.Contains(got, "body") {
				t.Errorf("body dropped: %q", got)
			}
		})
	}
}
