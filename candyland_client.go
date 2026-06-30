package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// candyland owns its REST API; detritus is only the client/launcher. The build
// flows call ensureCandylandUp to bring the installed sidecar binary up (health
// check, start if down, poll until ready), then drive a run over REST. There is
// no MCP server in this path — see setup.go for why candyland is not registered.
// `detritus --candyland-run <prompt-file> [folder ...]` is the in-session trigger
// the /candyland skill invokes (runCandyland below).

// candylandBaseURL is where the sidecar's REST API listens. candyland owns this
// contract; detritus only consumes it.
const candylandBaseURL = "http://127.0.0.1:8888"

// candylandDashboardURL is where the candyland SPA serves the live dashboard.
const candylandDashboardURL = "http://localhost:8080"

// readCandylandRunArgs parses `--candyland-run` args: the prompt file path and
// optional folders. It reads the prompt from promptFile (the .plan/<slug>.md path
// — keeps a large plan off argv) and defaults folders to [cwd] when none given.
func readCandylandRunArgs(promptFile string, folders []string, cwd string) (prompt string, resolvedFolders []string, err error) {
	raw, err := os.ReadFile(promptFile)
	if err != nil {
		return "", nil, fmt.Errorf("read prompt file %s: %w", promptFile, err)
	}
	if len(folders) == 0 {
		folders = []string{cwd}
	}
	return string(raw), folders, nil
}

// runCandyland is the `detritus --candyland-run <prompt-file> [folder ...]`
// handler: read the plan, ensure the sidecar is up, start the run, and print the
// run id + dashboard URL. ensureCandylandUp failures are returned so the caller
// exits non-zero and the skill falls back to the in-process flow.
func runCandyland(detritusPath, promptFile string, folders []string, cwd string) error {
	prompt, folders, err := readCandylandRunArgs(promptFile, folders, cwd)
	if err != nil {
		return err
	}
	if err := ensureCandylandUp(detritusPath); err != nil {
		return err
	}
	id, err := startCandylandRun(folders, prompt, "")
	if err != nil {
		return err
	}
	fmt.Printf("candyland run started: %s\n", id)
	fmt.Printf("Dashboard: %s\n", candylandDashboardURL)
	return nil
}

// readQuestRunArgs parses `--quest-run` args: the objective file path and
// optional folders. It reads the objective from objectiveFile (keeps a large
// objective off argv) and defaults folders to [cwd] when none given.
func readQuestRunArgs(objectiveFile string, folders []string, cwd string) (objective string, resolvedFolders []string, err error) {
	raw, err := os.ReadFile(objectiveFile)
	if err != nil {
		return "", nil, fmt.Errorf("read objective file %s: %w", objectiveFile, err)
	}
	if len(folders) == 0 {
		folders = []string{cwd}
	}
	return string(raw), folders, nil
}

// runQuestCmd is the `detritus --quest-run <objective-file> [folder ...]`
// handler: read the objective, ensure the sidecar is up, start the quest, and
// print the quest id + dashboard URL. Mirrors runCandyland. ensureCandylandUp
// failures are returned so the caller exits non-zero.
func runQuestCmd(detritusPath, objectiveFile string, folders []string, autonomy, deliver, cwd string) error {
	objective, folders, err := readQuestRunArgs(objectiveFile, folders, cwd)
	if err != nil {
		return err
	}
	if err := ensureCandylandUp(detritusPath); err != nil {
		return err
	}
	id, err := startCandylandQuest(objective, folders, autonomy, deliver)
	if err != nil {
		return err
	}
	fmt.Printf("candyland quest started: %s\n", id)
	fmt.Printf("Dashboard: %s\n", candylandDashboardURL)
	return nil
}

// readCampaignRunArgs parses `--campaign-run` args: the campaign input file path
// and optional folders. It reads the input from inputFile (a high-level goal,
// partial brief, or detailed plan — keeps a large input off argv) and defaults
// folders to [cwd] when none given.
func readCampaignRunArgs(inputFile string, folders []string, cwd string) (input string, resolvedFolders []string, err error) {
	raw, err := os.ReadFile(inputFile)
	if err != nil {
		return "", nil, fmt.Errorf("read campaign input file %s: %w", inputFile, err)
	}
	if len(folders) == 0 {
		folders = []string{cwd}
	}
	return string(raw), folders, nil
}

// runCampaignCmd is the `detritus --campaign-run <input-file> [folder ...]`
// handler: read the campaign input, ensure the sidecar is up, start the
// campaign, and print the campaign id + dashboard URL. Mirrors runQuestCmd.
// ensureCandylandUp failures are returned so the caller exits non-zero.
func runCampaignCmd(detritusPath, inputFile string, folders []string, autonomy, cwd string) error {
	input, folders, err := readCampaignRunArgs(inputFile, folders, cwd)
	if err != nil {
		return err
	}
	if err := ensureCandylandUp(detritusPath); err != nil {
		return err
	}
	id, err := startCandylandCampaign(input, folders, autonomy)
	if err != nil {
		return err
	}
	fmt.Printf("candyland campaign started: %s\n", id)
	fmt.Printf("Dashboard: %s\n", candylandDashboardURL)
	return nil
}

// ensureCandylandUp returns nil once the candyland sidecar answers its health
// endpoint. If it is already up the call is cheap. Otherwise it locates the
// installed binary beside detritus and starts it detached, inheriting the
// current environment so gh/HOME/GH_* credentials propagate to the sidecar's
// spawned agents, then polls /api/health until ready (~20s) or returns an honest
// error. detritusPath is the detritus executable path (os.Executable()), used to
// find the sibling candyland binary.
func ensureCandylandUp(detritusPath string) error {
	if candylandHealthy(2 * time.Second) {
		return nil
	}

	bin, ok := candylandBinFor(detritusPath)
	if !ok {
		return fmt.Errorf("candyland binary not installed beside detritus (run `detritus --setup` to fetch it)")
	}

	cmd := exec.Command(bin)
	// Propagate gh/HOME/GH_* creds (via os.Environ()) plus DETRITUS_BIN — the
	// detritus executable path — so candyland can register a passive detritus
	// stdio MCP ({command: <bin>, args: []}) in each agent's --mcp-config. Each
	// agent then spawns its own detritus stdio child, exactly like a VSCode
	// Claude session. There is no long-lived shared detritus process.
	selfExe, err := os.Executable()
	if err != nil || selfExe == "" {
		selfExe = detritusPath
	}
	cmd.Env = append(os.Environ(), "DETRITUS_BIN="+selfExe)
	detachProcess(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start candyland (%s): %w", bin, err)
	}
	// Detach: don't wait on the long-lived sidecar, just release our handle.
	go func() { _ = cmd.Wait() }()

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if candylandHealthy(2 * time.Second) {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("candyland did not become healthy within 20s after start")
}

// candylandHealthy reports whether the sidecar answers GET /api/health with 200.
func candylandHealthy(timeout time.Duration) bool {
	return candylandHealthyAt(candylandBaseURL, timeout)
}

// candylandHealthyAt is candylandHealthy against an explicit base URL (test seam).
func candylandHealthyAt(baseURL string, timeout time.Duration) bool {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(baseURL + "/api/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// candylandRunRequest is the body POSTed to /api/runs. No mode field — Mode is
// being removed from candyland's API.
type candylandRunRequest struct {
	Folders []string `json:"folders"`
	Prompt  string   `json:"prompt"`
	Title   string   `json:"title"`
}

// startCandylandRun creates a run (POST /api/runs) and begins it (POST
// /api/runs/{id}/begin), returning the run id. The sidecar must already be up;
// callers run ensureCandylandUp first.
func startCandylandRun(folders []string, prompt, title string) (string, error) {
	return startCandylandRunAt(candylandBaseURL, folders, prompt, title)
}

// startCandylandRunAt is startCandylandRun against an explicit base URL (test seam).
func startCandylandRunAt(baseURL string, folders []string, prompt, title string) (string, error) {
	body, err := json.Marshal(candylandRunRequest{Folders: folders, Prompt: prompt, Title: title})
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(baseURL+"/api/runs", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create run: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("create run: HTTP %d", resp.StatusCode)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return "", fmt.Errorf("decode run id: %w", err)
	}
	if created.ID == "" {
		return "", fmt.Errorf("create run: empty id in response")
	}

	beginResp, err := client.Post(baseURL+"/api/runs/"+created.ID+"/begin", "application/json", nil)
	if err != nil {
		return "", fmt.Errorf("begin run %s: %w", created.ID, err)
	}
	defer beginResp.Body.Close()
	if beginResp.StatusCode < 200 || beginResp.StatusCode >= 300 {
		return "", fmt.Errorf("begin run %s: HTTP %d", created.ID, beginResp.StatusCode)
	}
	return created.ID, nil
}

// candylandQuestRequest is the body POSTed to /api/quests. candyland owns the
// full QuestSpec; detritus only sets the fields the /quest command settles.
type candylandQuestRequest struct {
	Objective     string   `json:"objective"`
	Folders       []string `json:"folders"`
	AutonomyLevel string   `json:"autonomyLevel"`
	Deliver       string   `json:"deliver"`
}

// startCandylandQuest creates a quest (POST /api/quests) and begins it (POST
// /api/quests/{id}/begin), returning the quest id. The sidecar must already be
// up; callers run ensureCandylandUp first. Mirrors startCandylandRun.
func startCandylandQuest(objective string, folders []string, autonomy, deliver string) (string, error) {
	return startCandylandQuestAt(candylandBaseURL, objective, folders, autonomy, deliver)
}

// startCandylandQuestAt is startCandylandQuest against an explicit base URL (test seam).
func startCandylandQuestAt(baseURL, objective string, folders []string, autonomy, deliver string) (string, error) {
	body, err := json.Marshal(candylandQuestRequest{
		Objective:     objective,
		Folders:       folders,
		AutonomyLevel: autonomy,
		Deliver:       deliver,
	})
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(baseURL+"/api/quests", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create quest: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("create quest: HTTP %d", resp.StatusCode)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return "", fmt.Errorf("decode quest id: %w", err)
	}
	if created.ID == "" {
		return "", fmt.Errorf("create quest: empty id in response")
	}

	beginResp, err := client.Post(baseURL+"/api/quests/"+created.ID+"/begin", "application/json", nil)
	if err != nil {
		return "", fmt.Errorf("begin quest %s: %w", created.ID, err)
	}
	defer beginResp.Body.Close()
	if beginResp.StatusCode < 200 || beginResp.StatusCode >= 300 {
		return "", fmt.Errorf("begin quest %s: HTTP %d", created.ID, beginResp.StatusCode)
	}
	return created.ID, nil
}

// candylandCampaignRequest is the body POSTed to /api/campaigns. candyland owns
// the full CampaignSpec; detritus only sets the fields the /campaign command
// settles. tokenBudget is left to candyland's default and not set here.
type candylandCampaignRequest struct {
	Input         string   `json:"input"`
	Folders       []string `json:"folders"`
	AutonomyLevel string   `json:"autonomyLevel"`
}

// startCandylandCampaign creates a campaign (POST /api/campaigns) and begins it
// (POST /api/campaigns/{id}/begin), returning the campaign id. The sidecar must
// already be up; callers run ensureCandylandUp first. Mirrors startCandylandQuest.
func startCandylandCampaign(input string, folders []string, autonomy string) (string, error) {
	return startCandylandCampaignAt(candylandBaseURL, input, folders, autonomy)
}

// startCandylandCampaignAt is startCandylandCampaign against an explicit base URL (test seam).
func startCandylandCampaignAt(baseURL, input string, folders []string, autonomy string) (string, error) {
	body, err := json.Marshal(candylandCampaignRequest{
		Input:         input,
		Folders:       folders,
		AutonomyLevel: autonomy,
	})
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(baseURL+"/api/campaigns", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create campaign: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("create campaign: HTTP %d", resp.StatusCode)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return "", fmt.Errorf("decode campaign id: %w", err)
	}
	if created.ID == "" {
		return "", fmt.Errorf("create campaign: empty id in response")
	}

	beginResp, err := client.Post(baseURL+"/api/campaigns/"+created.ID+"/begin", "application/json", nil)
	if err != nil {
		return "", fmt.Errorf("begin campaign %s: %w", created.ID, err)
	}
	defer beginResp.Body.Close()
	if beginResp.StatusCode < 200 || beginResp.StatusCode >= 300 {
		return "", fmt.Errorf("begin campaign %s: HTTP %d", created.ID, beginResp.StatusCode)
	}
	return created.ID, nil
}
