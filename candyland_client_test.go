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
	"time"
)

// resolveCandylandDelivery is the two-tier feedback classifier: an explicit PR
// marker in the plan text wins (and may select feedback OR review); absent a
// marker, an open PR on the current branch selects feedback; otherwise new work.
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
			branchPRLookup = func(string) int { return tc.branchPR }
			deliver, pr := resolveCandylandDelivery(tc.prompt, "/repo")
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

// startCandylandQuest POSTs the quest, reads back the id, then begins it —
// sending objective/folders/autonomyLevel/deliver, and hitting /api/quests then
// /api/quests/{id}/begin. Driven against a mock server, no real candyland needed.
func TestStartCandylandQuest(t *testing.T) {
	var createdBody map[string]any
	beganID := ""

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/quests":
			defer r.Body.Close()
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &createdBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"quest-456"}`))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/quests/") && strings.HasSuffix(r.URL.Path, "/begin"):
			beganID = strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/quests/"), "/begin")
			w.WriteHeader(http.StatusNoContent) // 204, matching candyland's real /begin handler
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	id, err := startCandylandQuestAt(srv.URL, "ship the thing", []string{"/repo/a", "/repo/b"}, "L1", "pr", 0)
	if err != nil {
		t.Fatalf("startCandylandQuest: %v", err)
	}
	if id != "quest-456" {
		t.Errorf("quest id = %q, want quest-456", id)
	}
	if beganID != "quest-456" {
		t.Errorf("begin was called for %q, want quest-456", beganID)
	}
	if createdBody["objective"] != "ship the thing" {
		t.Errorf("objective not sent correctly: %v", createdBody["objective"])
	}
	if createdBody["autonomyLevel"] != "L1" {
		t.Errorf("autonomyLevel not sent correctly: %v", createdBody["autonomyLevel"])
	}
	if createdBody["deliver"] != "pr" {
		t.Errorf("deliver not sent correctly: %v", createdBody["deliver"])
	}
	folders, _ := createdBody["folders"].([]any)
	if len(folders) != 2 || folders[0] != "/repo/a" {
		t.Errorf("folders not sent correctly: %v", createdBody["folders"])
	}
}

// startCandylandCampaign POSTs the campaign, reads back the id, then begins it —
// sending input/folders/autonomyLevel, and hitting /api/campaigns then
// /api/campaigns/{id}/begin. Driven against a mock server, no real candyland.
func TestStartCandylandCampaign(t *testing.T) {
	var createdBody map[string]any
	beganID := ""

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/campaigns":
			defer r.Body.Close()
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &createdBody)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"campaign-789"}`))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/campaigns/") && strings.HasSuffix(r.URL.Path, "/begin"):
			beganID = strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/campaigns/"), "/begin")
			w.WriteHeader(http.StatusNoContent) // 204, matching candyland's real /begin handler
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	id, err := startCandylandCampaignAt(srv.URL, "ship the program", []string{"/repo/a", "/repo/b"}, "L2", "pr", 0)
	if err != nil {
		t.Fatalf("startCandylandCampaign: %v", err)
	}
	if id != "campaign-789" {
		t.Errorf("campaign id = %q, want campaign-789", id)
	}
	if beganID != "campaign-789" {
		t.Errorf("begin was called for %q, want campaign-789", beganID)
	}
	if createdBody["input"] != "ship the program" {
		t.Errorf("input not sent correctly: %v", createdBody["input"])
	}
	if createdBody["autonomyLevel"] != "L2" {
		t.Errorf("autonomyLevel not sent correctly: %v", createdBody["autonomyLevel"])
	}
	folders, _ := createdBody["folders"].([]any)
	if len(folders) != 2 || folders[0] != "/repo/a" {
		t.Errorf("folders not sent correctly: %v", createdBody["folders"])
	}
}

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

// readCampaignRunArgs reads the input from the file and defaults folders to
// [cwd] when none are passed, while preserving explicit folders.
func TestReadCampaignRunArgs(t *testing.T) {
	dir := t.TempDir()
	inputFile := filepath.Join(dir, "campaign.md")
	if err := os.WriteFile(inputFile, []byte("# goal\nship the program"), 0o644); err != nil {
		t.Fatal(err)
	}

	input, folders, err := readCampaignRunArgs(inputFile, nil, "/work/repo")
	if err != nil {
		t.Fatalf("readCampaignRunArgs: %v", err)
	}
	if input != "# goal\nship the program" {
		t.Errorf("input = %q, want the file contents", input)
	}
	if len(folders) != 1 || folders[0] != "/work/repo" {
		t.Errorf("folders = %v, want [/work/repo] (cwd default)", folders)
	}

	_, folders2, err := readCampaignRunArgs(inputFile, []string{"/a", "/b"}, "/work/repo")
	if err != nil {
		t.Fatalf("readCampaignRunArgs: %v", err)
	}
	if len(folders2) != 2 || folders2[0] != "/a" || folders2[1] != "/b" {
		t.Errorf("explicit folders not preserved: %v", folders2)
	}

	if _, _, err := readCampaignRunArgs(filepath.Join(dir, "nope.md"), nil, "/work/repo"); err == nil {
		t.Error("a missing campaign input file should error")
	}
}

// ensureCandylandUp returns nil immediately when the health endpoint already
// answers 200 — no binary needed.
func TestEnsureCandylandUpAlreadyHealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	if !candylandHealthyAt(srv.URL, time.Second) {
		t.Fatal("mock health endpoint should report healthy")
	}
}

// candylandHealthyAt reports false when nothing is listening / the endpoint 500s.
func TestCandylandHealthyAtDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if candylandHealthyAt(srv.URL, time.Second) {
		t.Error("a 500 health response must not count as healthy")
	}
}

func TestTitleFriendlyPrompt(t *testing.T) {
	cases := []struct {
		name, in, wantFirst string
	}{
		{"markdown heading", "# Plan: campaign/quest agents\n\nbody here", "campaign/quest agents"},
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
