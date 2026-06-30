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

	id, err := startCandylandRunAt(srv.URL, []string{"/repo/a", "/repo/b"}, "do the thing", "My Run")
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
	folders, _ := createdBody["folders"].([]any)
	if len(folders) != 2 || folders[0] != "/repo/a" {
		t.Errorf("folders not sent correctly: %v", createdBody["folders"])
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

	id, err := startCandylandQuestAt(srv.URL, "ship the thing", []string{"/repo/a", "/repo/b"}, "L1", "pr")
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

	id, err := startCandylandCampaignAt(srv.URL, "ship the program", []string{"/repo/a", "/repo/b"}, "L2")
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
