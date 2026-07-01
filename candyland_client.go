package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
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

// questExecuteVerbs are the execute-shaped intent markers that flip a quest from
// report-only (L1) to executing (L2). Kept conservative: only verbs that clearly
// ask for a code change. Matched case-insensitively as whole words anywhere in
// the objective. Add to this list deliberately — a false positive launches runs.
var questExecuteVerbs = []string{
	"solve", "fix", "wire", "implement", "build", "add", "refactor",
	"update pr", "update the pr", "rework", "patch", "migrate", "rewrite",
}

// deriveQuestAutonomy classifies an objective's intent into an autonomy level.
// Execute-shaped objectives (asking for a code change) get the executing level
// "L2" so candyland launches runs, while deliver:"pr" keeps the PR gate as the
// safety rail. Anything else stays report-only "L1". Pure: string in → level
// out, so it is deterministic and separately testable. Detection is intentionally
// conservative — see questExecuteVerbs.
func deriveQuestAutonomy(objective string) string {
	lower := strings.ToLower(objective)
	for _, verb := range questExecuteVerbs {
		if containsWord(lower, verb) {
			return "L2"
		}
	}
	return "L1"
}

// resolveQuestAutonomy is the effective autonomy for a launch given the derived
// delivery mode AND the objective. It is pure (mode + objective in → level out) so
// it is separately testable, and it keeps autonomy anchored to the OBJECTIVE'S VERB,
// never hardcoded by delivery mode:
//
//   - feedback → L2: a feedback intent always changes code (updates a PR in place).
//   - review / pr → derived from the verb (deriveQuestAutonomy): a PURE review
//     ("review PR #N", "check PR #N against the spec") has no execute verb and stays
//     report-only (L1), but a review objective that ALSO asks to act ("fix any
//     problems on PR #N, then review the PR") carries an execute verb and runs
//     executing (L2). Forcing review→L1 unconditionally is the bug that stranded a
//     "fix problems on PR" quest at report-only just because it said "review the PR".
func resolveQuestAutonomy(deliver, objective string) string {
	if deliver == "feedback" {
		return "L2"
	}
	return deriveQuestAutonomy(objective)
}

// questFeedbackMarkers are the FEEDBACK/FIX-on-an-existing-PR intent markers.
// Their presence (alongside a parsed PR number) selects deliver:"feedback" —
// update the referenced PR in place, never open a new one. Matched
// case-insensitively as substrings (phrases, so word-boundary matching is not
// used). Kept conservative — see deriveQuestDelivery.
var questFeedbackMarkers = []string{
	"address feedback", "address the feedback", "address review",
	"fix the review", "fix review", "review comments", "review feedback",
	"update pr", "update the pr", "apply feedback", "apply the feedback",
	"resolve feedback", "resolve the comments", "feedback on pr", "feedback on #",
}

// questReviewMarkers are the REVIEW/CHECK-only intent markers. Their presence
// (alongside a parsed PR number, and absent any feedback marker) selects
// deliver:"review" — read the PR and report, opening nothing. Matched
// case-insensitively as substrings.
var questReviewMarkers = []string{
	"review pr", "review the pr", "check pr", "check the pr",
	"review #", "check #", "look at pr", "audit pr",
}

// deriveQuestDelivery classifies an objective into a delivery mode and target
// PR, mirroring /gh-feedback-work's "update the existing PR in place, never open
// a new PR" model. It is pure: string in → (mode, prNumber) out, so it is
// deterministic and separately testable. Detection is intentionally conservative
// — markers are explicit phrases (questFeedbackMarkers / questReviewMarkers) and
// a PR number must parse, or it falls back to the new-work default.
//
//   - FEEDBACK/FIX intent + a PR number → ("feedback", N): update PR #N in place.
//   - REVIEW/CHECK-only intent + a PR number → ("review", N): report on PR #N.
//   - anything else → ("pr", 0): new work, new PR (today's default).
//
// Feedback wins over review when both kinds of marker appear, since a fix intent
// is the stronger signal (it changes code). A mode that needs a PR number but
// finds none degrades to ("pr", 0) rather than guessing a target.
func deriveQuestDelivery(objective string) (deliver string, targetPR int) {
	lower := strings.ToLower(objective)
	pr := parsePRNumber(lower)
	if pr == 0 {
		return "pr", 0
	}
	for _, marker := range questFeedbackMarkers {
		if strings.Contains(lower, marker) {
			return "feedback", pr
		}
	}
	for _, marker := range questReviewMarkers {
		if strings.Contains(lower, marker) {
			return "review", pr
		}
	}
	return "pr", 0
}

// parsePRNumber extracts a PR number from "#N" or "pr N" / "pr#N" markers in the
// (already-lowercased) string, returning the first one found or 0 when none
// parses. Conservative: only these two explicit forms count, so a bare number in
// prose is not mistaken for a PR reference.
func parsePRNumber(lower string) int {
	if n := numberAfter(lower, "#"); n > 0 {
		return n
	}
	if n := numberAfter(lower, "pr "); n > 0 {
		return n
	}
	if n := numberAfter(lower, "pr#"); n > 0 {
		return n
	}
	return 0
}

// numberAfter scans s for marker and returns the run of ASCII digits immediately
// following the first occurrence whose digits are non-empty, or 0 when none is
// found. It skips occurrences not followed by a digit (so "pr review" does not
// match "pr ").
func numberAfter(s, marker string) int {
	from := 0
	for {
		idx := strings.Index(s[from:], marker)
		if idx < 0 {
			return 0
		}
		start := from + idx + len(marker)
		end := start
		for end < len(s) && s[end] >= '0' && s[end] <= '9' {
			end++
		}
		if end > start {
			n := 0
			for _, c := range s[start:end] {
				n = n*10 + int(c-'0')
			}
			return n
		}
		from = from + idx + 1
	}
}

// containsWord reports whether word appears in s bounded by non-letter
// characters (or string edges), so "fix" matches "fix the bug" and "go and fix"
// but not "prefix" or "fixture". word is expected to be lowercase; s is matched
// case-insensitively by the caller lowercasing it first.
func containsWord(s, word string) bool {
	from := 0
	for {
		idx := strings.Index(s[from:], word)
		if idx < 0 {
			return false
		}
		start := from + idx
		end := start + len(word)
		if !isLetter(byteAt(s, start-1)) && !isLetter(byteAt(s, end)) {
			return true
		}
		from = start + 1
	}
}

// byteAt returns s[i] or 0 when i is out of range, so edge checks need no bounds
// guard at the call site.
func byteAt(s string, i int) byte {
	if i < 0 || i >= len(s) {
		return 0
	}
	return s[i]
}

// isLetter reports whether b is an ASCII letter — the word-boundary test.
func isLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// questAutonomyEffect returns a one-line "what this will / won't do" description
// derived from the autonomy level — so the launch output tells the user what the
// quest is actually permitted to do.
func questAutonomyEffect(autonomy string) string {
	switch autonomy {
	case "L2":
		return "launches runs and opens a PR per change for your review — never merges"
	default:
		return "surfaces findings only — no code changes, no PRs"
	}
}

// questDeliveryEffect returns a one-line honest description of what the deliver
// mode will produce, so the launch output never overstates the outcome.
// feedback updates the referenced PR in place (no new PR); review reports on it
// (may produce no PR); pr opens a new PR for review. None of the modes ever
// merge — that safety rail is stated in every line.
func questDeliveryEffect(deliver string, targetPR int) string {
	switch deliver {
	case "feedback":
		return fmt.Sprintf("updates existing PR #%d in place — no new PR, never merges", targetPR)
	case "review":
		return fmt.Sprintf("reviews PR #%d — may produce no PR, never merges", targetPR)
	default:
		return "opens a PR, never merges"
	}
}

// questLaunchSummary renders the enriched launch output for a started quest: the
// quest id, autonomy level, deliver mode, a what-will/won't-do line, and the API
// + UI ports with a remote/WSL port-forwarding hint. When the effective autonomy
// is L1 (report-only) for an execute-shaped objective it leads with a LOUD
// warning, since the user asked for a change but the quest will only report.
//
// autonomy is supplied by the caller independently of objective; this guard fires
// whenever the two contradict — an L1 (report-only) autonomy against an
// execute-shaped objective (an execute verb present). The --quest-run path derives
// autonomy from this same objective (resolveQuestAutonomy), so an aligned launch
// cannot trip it; the guard is defense-in-depth for any caller that sets autonomy by
// other means (e.g. an explicit operator override), honoring the doctrinal "say so
// loudly before quest started" rule. It is NOT suppressed for review delivery: a
// review objective that asks to fix (an execute verb) forced to L1 is exactly the
// mismatch worth warning about.
//
// deliver/targetPR state the delivery mode honestly (questDeliveryEffect):
// feedback updates PR #N in place, review reports on PR #N, pr opens a new PR.
func questLaunchSummary(id, autonomy, deliver string, targetPR int, objective string) string {
	var b strings.Builder
	if autonomy == "L1" && deriveQuestAutonomy(objective) == "L2" {
		fmt.Fprintf(&b, "WARNING: objective looks execute-shaped but autonomy is L1 (report-only) — no code changes or PRs will be made\n")
	}
	fmt.Fprintf(&b, "candyland quest started: %s\n", id)
	fmt.Fprintf(&b, "Autonomy: %s — %s\n", autonomy, questAutonomyEffect(autonomy))
	fmt.Fprintf(&b, "Deliver: %s (%s)\n", deliver, questDeliveryEffect(deliver, targetPR))
	fmt.Fprintf(&b, "API: %s\n", candylandBaseURL)
	fmt.Fprintf(&b, "UI / Dashboard: %s\n", candylandDashboardURL)
	fmt.Fprintf(&b, "Remote/WSL: forward BOTH ports — the UI on :8080 stays empty until the API on :8888 is reachable\n")
	return b.String()
}

// runQuestCmd is the `detritus --quest-run <objective-file> [folder ...]`
// handler: read the objective, ensure the sidecar is up, start the quest, and
// print the enriched launch summary. Mirrors runCandyland. ensureCandylandUp
// failures are returned so the caller exits non-zero. An empty autonomy means
// "derive from the objective" (the --quest-run default); a non-empty autonomy is
// an explicit override the launch summary still validates against the objective.
func runQuestCmd(detritusPath, objectiveFile string, folders []string, autonomy, deliver, cwd string) error {
	objective, folders, err := readQuestRunArgs(objectiveFile, folders, cwd)
	if err != nil {
		return err
	}
	// Derive BOTH delivery mode and autonomy from the objective. Autonomy stays
	// anchored to the objective's VERB (resolveQuestAutonomy): feedback always runs
	// executing (L2), while review/pr honor the verb — a pure review stays report-only
	// (L1) but a review objective that also asks to fix carries an execute verb and
	// runs executing. Delivery mode never dictates report-only on its own.
	deliveryMode, targetPR := deriveQuestDelivery(objective)
	if deliver == "" {
		deliver = deliveryMode
	}
	if autonomy == "" {
		autonomy = resolveQuestAutonomy(deliver, objective)
	}
	if err := ensureCandylandUp(detritusPath); err != nil {
		return err
	}
	id, err := startCandylandQuest(objective, folders, autonomy, deliver, targetPR)
	if err != nil {
		return err
	}
	fmt.Print(questLaunchSummary(id, autonomy, deliver, targetPR, objective))
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
// campaign, and print the launch summary. Mirrors runQuestCmd.
// ensureCandylandUp failures are returned so the caller exits non-zero.
//
// Delivery mode is DERIVED from the input (deriveQuestDelivery, reused from the
// quest path): a feedback/review-on-PR input carries deliver:"feedback"/"review"
// with the target PR, otherwise deliver:"pr" (new work — the default). Autonomy
// is NOT derived for campaigns: it stays the caller-supplied "L2" (campaign rule
// — campaigns are never L1, since a report-only campaign would strand with no PR).
func runCampaignCmd(detritusPath, inputFile string, folders []string, autonomy, cwd string) error {
	input, folders, err := readCampaignRunArgs(inputFile, folders, cwd)
	if err != nil {
		return err
	}
	deliver, targetPR := deriveQuestDelivery(input)
	if err := ensureCandylandUp(detritusPath); err != nil {
		return err
	}
	id, err := startCandylandCampaign(input, folders, autonomy, deliver, targetPR)
	if err != nil {
		return err
	}
	fmt.Print(campaignLaunchSummary(id, autonomy, deliver, targetPR))
	return nil
}

// campaignLaunchSummary renders the launch output for a started campaign: the
// campaign id, autonomy level (fixed L2 for campaigns — not derived), the derived
// delivery mode with an honest what-it-produces line (questDeliveryEffect, reused),
// and the dashboard URL. Mirrors questLaunchSummary's mode line so feedback/review/pr
// read the same across both flows.
func campaignLaunchSummary(id, autonomy, deliver string, targetPR int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "candyland campaign started: %s\n", id)
	fmt.Fprintf(&b, "Autonomy: %s — %s\n", autonomy, questAutonomyEffect(autonomy))
	fmt.Fprintf(&b, "Deliver: %s (%s)\n", deliver, questDeliveryEffect(deliver, targetPR))
	fmt.Fprintf(&b, "Dashboard: %s\n", candylandDashboardURL)
	return b.String()
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
	TargetPR      int      `json:"targetPr"`
}

// startCandylandQuest creates a quest (POST /api/quests) and begins it (POST
// /api/quests/{id}/begin), returning the quest id. The sidecar must already be
// up; callers run ensureCandylandUp first. Mirrors startCandylandRun.
// deliver/targetPR are the shared wire contract with the candyland receiver:
// "feedback"|"review" carry targetPR, "pr" carries 0.
func startCandylandQuest(objective string, folders []string, autonomy, deliver string, targetPR int) (string, error) {
	return startCandylandQuestAt(candylandBaseURL, objective, folders, autonomy, deliver, targetPR)
}

// startCandylandQuestAt is startCandylandQuest against an explicit base URL (test seam).
func startCandylandQuestAt(baseURL, objective string, folders []string, autonomy, deliver string, targetPR int) (string, error) {
	body, err := json.Marshal(candylandQuestRequest{
		Objective:     objective,
		Folders:       folders,
		AutonomyLevel: autonomy,
		Deliver:       deliver,
		TargetPR:      targetPR,
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
// deliver/targetPr share the exact wire contract with candylandQuestRequest: the
// campaign launcher derives them from the input (deriveQuestDelivery) and
// candyland propagates them to the affected child quests/runs.
type candylandCampaignRequest struct {
	Input         string   `json:"input"`
	Folders       []string `json:"folders"`
	AutonomyLevel string   `json:"autonomyLevel"`
	Deliver       string   `json:"deliver"`
	TargetPR      int      `json:"targetPr"`
}

// startCandylandCampaign creates a campaign (POST /api/campaigns) and begins it
// (POST /api/campaigns/{id}/begin), returning the campaign id. The sidecar must
// already be up; callers run ensureCandylandUp first. Mirrors startCandylandQuest.
// deliver/targetPR carry the input's delivery intent; candyland propagates them
// to the child quests/runs the campaign decomposes into.
func startCandylandCampaign(input string, folders []string, autonomy, deliver string, targetPR int) (string, error) {
	return startCandylandCampaignAt(candylandBaseURL, input, folders, autonomy, deliver, targetPR)
}

// startCandylandCampaignAt is startCandylandCampaign against an explicit base URL (test seam).
func startCandylandCampaignAt(baseURL, input string, folders []string, autonomy, deliver string, targetPR int) (string, error) {
	body, err := json.Marshal(candylandCampaignRequest{
		Input:         input,
		Folders:       folders,
		AutonomyLevel: autonomy,
		Deliver:       deliver,
		TargetPR:      targetPR,
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
