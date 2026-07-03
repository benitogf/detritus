package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
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

// titleFriendlyPrompt cleans the prompt's FIRST non-empty line so candyland's
// client-side suggestTitle (first line, /-commands stripped, first 7 words) yields
// a meaningful run title instead of a markdown artifact. Plan files usually open
// with a heading like "# Plan: <topic>", which suggestTitle would surface as
// "# Plan: …" — so strip the leading heading markers and a leading "Plan:" /
// "Plan —" label, leaving the topic itself as the title line. The body is
// untouched (the agent still receives the full plan); only the leading line is
// normalized, and only when it is a heading — a plain first line is left as-is.
func titleFriendlyPrompt(raw string) string {
	lines := strings.Split(raw, "\n")
	for i, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		cleaned := strings.TrimLeft(ln, "#")
		cleaned = strings.TrimSpace(cleaned)
		for _, prefix := range []string{"Plan:", "Plan —", "Plan -"} {
			if len(cleaned) >= len(prefix) && strings.EqualFold(cleaned[:len(prefix)], prefix) {
				cleaned = strings.TrimSpace(cleaned[len(prefix):])
				break
			}
		}
		if cleaned != "" && cleaned != ln {
			lines[i] = cleaned
		}
		break // only the first non-empty line is the title line
	}
	return strings.Join(lines, "\n")
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
	// Normalize the leading line so suggestTitle produces a meaningful title.
	prompt = titleFriendlyPrompt(prompt)
	// Recognize a PR-feedback run so it updates the existing PR in place instead
	// of opening a new one. The primary repo (folders[0]) is where a
	// branch-state fallback looks for an open PR.
	deliver, targetPR, err := resolveCandylandDelivery(prompt, folders[0])
	if err != nil {
		return err
	}
	if err := ensureCandylandUp(detritusPath); err != nil {
		return err
	}
	id, err := startCandylandRun(folders, prompt, "", deliver, targetPR)
	if err != nil {
		return err
	}
	fmt.Printf("candyland run started: %s\n", id)
	fmt.Printf("Deliver: %s (%s)\n", deliver, questDeliveryEffect(deliver, targetPR))
	fmt.Printf("API: %s\n", candylandBaseURL)
	fmt.Printf("UI / Dashboard: %s\n", candylandDashboardURL)
	return nil
}

// resolveCandylandDelivery classifies a --candyland-run into a delivery mode +
// target PR, so a feedback run updates the existing PR in place instead of
// opening a new one. Two tiers, reference first:
//   - the plan text's PR/issue reference, classified against live gh state
//     (resolveLaunchDelivery — the gh-mirror classification all launchers share);
//     an explicit reference wins and can select feedback OR review, or abort on
//     a merged/closed target.
//   - otherwise fall back to repo's current-branch open PR (branchPRLookup) — the
//     same "the branch already has a PR" signal /gh infers from — which can only
//     mean feedback (there is a PR to update).
func resolveCandylandDelivery(prompt, repo string) (deliver string, targetPR int, err error) {
	deliver, targetPR, err = resolveLaunchDelivery(prompt, repo)
	if err != nil {
		return "", 0, err
	}
	if deliver == "pr" && targetPR == 0 {
		if pr := branchPRLookup(repo); pr > 0 {
			return "feedback", pr, nil
		}
	}
	return deliver, targetPR, nil
}

// branchPRLookup resolves the open PR for a repo's current branch. It is a var so
// tests can substitute a stub for the real `gh` shell-out.
var branchPRLookup = currentBranchPR

// currentBranchPR returns the open PR number for repo's current git branch, or 0
// when there is none (no PR, detached HEAD, gh unavailable, or the default
// branch). It shells out to `gh pr view` — the same tool candyland uses — and
// stays deliberately quiet on any error: branch-state inference is a best-effort
// fallback, so a missing gh or no-PR branch must degrade to new-work delivery,
// never fail the run. A short timeout bounds a hung gh (e.g. a network/auth
// stall) so it degrades to new-work rather than blocking the run indefinitely.
func currentBranchPR(repo string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", "--json", "number,state", "--jq", "select(.state==\"OPEN\") | .number")
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	return parseGHPRNumber(string(out))
}

// parseGHPRNumber reads the `gh pr view … --jq` output into a PR number, or 0 for
// anything that isn't a clean integer — empty (the branch has no OPEN PR, so the
// jq select() matched nothing), "null", or stray whitespace. Kept separate from
// the shell-out so the degrade-to-zero cases are testable without a live gh.
func parseGHPRNumber(out string) int {
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0
	}
	return n
}

// launchClassification is the outcome of classifyLaunchInput — the delivery mode
// (deliver ∈ {pr,feedback,review}) and target PR derived from a launcher's input.
// Ambiguous marks an open target PR with no decisive signal (the CLI asks once via
// ambiguousDeliveryPrompt before launch); Degraded marks a `gh` failure that fell
// back to marker-only derivation, so the caller can surface the reduced confidence.
type launchClassification struct {
	Deliver   string
	TargetPR  int
	Ambiguous bool
	Degraded  bool
}

// launchReference is a parsed PR/issue reference from a launcher's input.
type launchReference struct {
	owner string
	repo  string
	num   int
	bare  bool // "#N" with no owner/repo — resolve owner/repo from the cwd repo
}

var (
	refURLRe  = regexp.MustCompile(`github\.com/([\w.\-]+)/([\w.\-]+)/(?:pull|issues)/(\d+)`)
	refNWORe  = regexp.MustCompile(`([\w.\-]+)/([\w.\-]+)#(\d+)`)
	refBareRe = regexp.MustCompile(`#(\d+)`)
)

// parseLaunchReference extracts the first PR/issue reference from input: a full
// github.com URL, an owner/repo#N shorthand, or a bare #N (resolved against the
// cwd repo). Returns found=false when the input names no reference (new work).
func parseLaunchReference(input string) (launchReference, bool) {
	if m := refURLRe.FindStringSubmatch(input); m != nil {
		return launchReference{owner: m[1], repo: m[2], num: atoiOr0(m[3])}, true
	}
	if m := refNWORe.FindStringSubmatch(input); m != nil {
		return launchReference{owner: m[1], repo: m[2], num: atoiOr0(m[3])}, true
	}
	if m := refBareRe.FindStringSubmatch(input); m != nil {
		return launchReference{num: atoiOr0(m[1]), bare: true}, true
	}
	return launchReference{}, false
}

// classifyLaunchInput mirrors the flows/github/gh Phase 0/1 classification OUTCOME
// against freshly-fetched (stubbed in tests) gh state, mapped to a delivery mode.
// It is the one classifier all sidecar launchers share. Rows (see the /gh doc):
//   - open PR + unaddressed CHANGES_REQUESTED → feedback (even with review text)
//   - open PR + review-intent text → review
//   - open PR + post-last-commit comments, no review text → feedback
//   - open PR + none of the above → Ambiguous (CLI asks once before launch)
//   - open issue → pr (the issue ref stays in the objective)
//   - merged/closed PR or closed issue → error (verbatim state), aborting launch
//   - no reference → pr (new work; gh is never touched)
//
// A `gh api` failure degrades to marker-only derivation (deriveMarkerDelivery) and
// signals Degraded, so a missing/erroring gh never fails the launch.
func classifyLaunchInput(input, cwd string) (launchClassification, error) {
	ref, found := parseLaunchReference(input)
	if !found {
		return launchClassification{Deliver: "pr"}, nil
	}
	if ref.bare {
		nwo := ghRepoNWO(cwd)
		if nwo == "" {
			return degradedClassification(input), nil
		}
		parts := strings.SplitN(strings.TrimSpace(nwo), "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return degradedClassification(input), nil
		}
		ref.owner, ref.repo = parts[0], parts[1]
	}

	issue, err := ghAPIJSON(cwd, fmt.Sprintf("repos/%s/%s/issues/%d", ref.owner, ref.repo, ref.num))
	if err != nil {
		return degradedClassification(input), nil
	}

	// .pull_request present & non-null ⇒ a PR; otherwise an issue.
	if pr, ok := issue["pull_request"]; !ok || pr == nil {
		state, _ := issue["state"].(string)
		if state != "open" {
			return launchClassification{}, fmt.Errorf("issue #%d in %s/%s is %s — not open work; aborting launch", ref.num, ref.owner, ref.repo, state)
		}
		return launchClassification{Deliver: "pr"}, nil // new work; issue ref stays in the objective
	}

	// A PR: authoritative state comes from the pulls endpoint (merged vs closed).
	pull, err := ghAPIJSON(cwd, fmt.Sprintf("repos/%s/%s/pulls/%d", ref.owner, ref.repo, ref.num))
	if err != nil {
		return degradedClassification(input), nil
	}
	state, _ := pull["state"].(string)
	if state != "open" {
		verb := "closed"
		if mergedAt, ok := pull["merged_at"]; ok && mergedAt != nil {
			verb = "merged"
		}
		return launchClassification{}, fmt.Errorf("PR #%d in %s/%s is %s — no longer open; aborting launch", ref.num, ref.owner, ref.repo, verb)
	}

	deliver := classifyOpenPR(cwd, ref, input)
	if deliver == "" {
		return launchClassification{TargetPR: ref.num, Ambiguous: true}, nil
	}
	return launchClassification{Deliver: deliver, TargetPR: ref.num}, nil
}

// classifyOpenPR decides feedback vs review vs "" (ambiguous) for an OPEN PR,
// combining live review/commit/comment state with the input's intent text:
//  1. an unaddressed CHANGES_REQUESTED (latest decisive review per reviewer) →
//     feedback, even if the text asks for a review;
//  2. review-intent text → review;
//  3. feedback-intent text → feedback;
//  4. comments after the last commit (no review text) → feedback;
//  5. otherwise "" — the caller marks it Ambiguous and asks once.
func classifyOpenPR(cwd string, ref launchReference, input string) string {
	if unaddressedChangesRequested(cwd, ref) {
		return "feedback"
	}
	lower := strings.ToLower(input)
	if hasReviewIntent(lower) {
		return "review"
	}
	if hasFeedbackIntent(lower) {
		return "feedback"
	}
	if hasPostCommitComments(cwd, ref) {
		return "feedback"
	}
	return ""
}

// unaddressedChangesRequested reports whether any reviewer's latest DECISIVE review
// (APPROVED/CHANGES_REQUESTED/DISMISSED, in submission order — COMMENTED/PENDING
// dropped) is CHANGES_REQUESTED. gh failure ⇒ false (no blocker seen).
func unaddressedChangesRequested(cwd string, ref launchReference) bool {
	arr, err := ghAPIArray(cwd, fmt.Sprintf("repos/%s/%s/pulls/%d/reviews", ref.owner, ref.repo, ref.num))
	if err != nil {
		return false
	}
	latest := map[string]string{}
	for _, r := range arr {
		m, _ := r.(map[string]any)
		state, _ := m["state"].(string)
		if state != "APPROVED" && state != "CHANGES_REQUESTED" && state != "DISMISSED" {
			continue // COMMENTED / PENDING are not decisive
		}
		user, _ := m["user"].(map[string]any)
		login, _ := user["login"].(string)
		latest[login] = state
	}
	for _, state := range latest {
		if state == "CHANGES_REQUESTED" {
			return true
		}
	}
	return false
}

// hasPostCommitComments reports whether the PR has any issue comment created after
// its last commit. gh failure ⇒ false.
func hasPostCommitComments(cwd string, ref launchReference) bool {
	commits, err := ghAPIArray(cwd, fmt.Sprintf("repos/%s/%s/pulls/%d/commits", ref.owner, ref.repo, ref.num))
	if err != nil {
		return false
	}
	var last time.Time
	for _, c := range commits {
		m, _ := c.(map[string]any)
		commit, _ := m["commit"].(map[string]any)
		committer, _ := commit["committer"].(map[string]any)
		if ts := parseTime(committer["date"]); ts.After(last) {
			last = ts
		}
	}
	comments, err := ghAPIArray(cwd, fmt.Sprintf("repos/%s/%s/issues/%d/comments", ref.owner, ref.repo, ref.num))
	if err != nil {
		return false
	}
	for _, c := range comments {
		m, _ := c.(map[string]any)
		if parseTime(m["created_at"]).After(last) {
			return true
		}
	}
	return false
}

// hasReviewIntent reports review/check-only intent in the (lowercased) input.
func hasReviewIntent(lower string) bool {
	return containsWord(lower, "review") || containsWord(lower, "audit")
}

// hasFeedbackIntent reports feedback/fix-on-a-PR intent in the (lowercased) input.
func hasFeedbackIntent(lower string) bool {
	if strings.Contains(lower, "feedback") {
		return true
	}
	for _, marker := range questFeedbackMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// degradedClassification is the `gh`-unavailable fallback: marker-only derivation
// (deriveMarkerDelivery) with Degraded set so the caller can flag reduced confidence.
func degradedClassification(input string) launchClassification {
	deliver, pr := deriveMarkerDelivery(input)
	return launchClassification{Deliver: deliver, TargetPR: pr, Degraded: true}
}

// ambiguousDeliveryPrompt asks the operator to resolve an ambiguous open PR
// (review / feedback / cancel). It is a var so tests substitute the interaction.
var ambiguousDeliveryPrompt = func(pr int) string {
	fmt.Printf("PR #%d is open with no decisive review/comment signal.\n", pr)
	fmt.Print("Deliver as [review/feedback/cancel]? ")
	var choice string
	_, _ = fmt.Scanln(&choice)
	return strings.ToLower(strings.TrimSpace(choice))
}

// resolveLaunchDelivery is the gh-mirror classification all sidecar launchers
// share: it classifies input against live gh state and resolves an ambiguous open
// PR by asking once (ambiguousDeliveryPrompt). A merged/closed target errors; a
// cancelled ambiguous prompt errors (aborting the launch).
func resolveLaunchDelivery(input, cwd string) (deliver string, targetPR int, err error) {
	c, err := classifyLaunchInput(input, cwd)
	if err != nil {
		return "", 0, err
	}
	if c.Ambiguous {
		switch ambiguousDeliveryPrompt(c.TargetPR) {
		case "review":
			return "review", c.TargetPR, nil
		case "feedback":
			return "feedback", c.TargetPR, nil
		default:
			return "", 0, fmt.Errorf("launch cancelled: ambiguous delivery for PR #%d", c.TargetPR)
		}
	}
	return c.Deliver, c.TargetPR, nil
}

// deriveShortTitle yields a compact display label from an objective/plan/input:
// leading markdown heading markers and an Objective:/Goal:/Plan: prefix stripped,
// first non-empty line only, capped at seven words (…-elided beyond that).
func deriveShortTitle(objective string) string {
	line := ""
	for _, ln := range strings.Split(objective, "\n") {
		if strings.TrimSpace(ln) != "" {
			line = strings.TrimSpace(ln)
			break
		}
	}
	if line == "" {
		return ""
	}
	line = strings.TrimSpace(strings.TrimLeft(line, "#"))
	for _, prefix := range []string{"Objective:", "Goal:", "Plan:"} {
		if len(line) >= len(prefix) && strings.EqualFold(line[:len(prefix)], prefix) {
			line = strings.TrimSpace(line[len(prefix):])
			break
		}
	}
	words := strings.Fields(line)
	if len(words) > 7 {
		return strings.Join(words[:7], " ") + "…"
	}
	return line
}

// ghAPIJSON runs `gh api <path>` in cwd and decodes a JSON object.
func ghAPIJSON(cwd, path string) (map[string]any, error) {
	out, err := ghAPIRaw(cwd, path)
	if err != nil {
		return nil, err
	}
	m := map[string]any{}
	if err := json.Unmarshal(out, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// ghAPIArray runs `gh api <path>` in cwd and decodes a JSON array.
func ghAPIArray(cwd, path string) ([]any, error) {
	out, err := ghAPIRaw(cwd, path)
	if err != nil {
		return nil, err
	}
	var arr []any
	if err := json.Unmarshal(out, &arr); err != nil {
		return nil, err
	}
	return arr, nil
}

// ghAPIRaw shells out to `gh api <path>` in cwd with a bounded timeout — the same
// gh shell-out + PATH-stub pattern currentBranchPR uses (no live network in tests).
func ghAPIRaw(cwd, path string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "api", path)
	cmd.Dir = cwd
	return cmd.Output()
}

// ghRepoNWO resolves the cwd repo's owner/name via `gh repo view`, or "" on any
// failure (so a bare #N with no resolvable repo degrades to marker derivation).
func ghRepoNWO(cwd string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "repo", "view", "--json", "nameWithOwner", "--jq", ".nameWithOwner")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// parseTime parses an RFC3339 timestamp from a JSON string value, or zero.
func parseTime(v any) time.Time {
	s, _ := v.(string)
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// atoiOr0 parses n as a base-10 int, returning 0 on failure.
func atoiOr0(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
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

// deriveMarkerDelivery is the marker-only (text-heuristic) delivery classifier,
// kept ONLY as the degraded fallback for when `gh api` is unavailable — the
// primary classifier is classifyLaunchInput, which mirrors flows/github/gh
// Phase 0/1 against live gh state. It is pure: string in → (mode, prNumber) out.
// Detection is intentionally conservative — markers are explicit phrases
// (questFeedbackMarkers / questReviewMarkers) and a PR number must parse, or it
// falls back to the new-work default.
//
//   - FEEDBACK/FIX intent + a PR number → ("feedback", N): update PR #N in place.
//   - REVIEW/CHECK-only intent + a PR number → ("review", N): report on PR #N.
//   - anything else → ("pr", 0): new work, new PR (the default).
//
// Feedback wins over review when both kinds of marker appear, since a fix intent
// is the stronger signal (it changes code). A mode that needs a PR number but
// finds none degrades to ("pr", 0) rather than guessing a target.
func deriveMarkerDelivery(objective string) (deliver string, targetPR int) {
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

// questLaunchSummary renders the launch output for a started quest or
// adventure: the id, deliver mode with a what-will/won't-do line
// (questDeliveryEffect: feedback updates PR #N in place, review reports on
// PR #N, pr opens a new PR), and the API + UI ports with a remote/WSL
// port-forwarding hint. kind names the flow ("quest" or "adventure").
func questLaunchSummary(kind, id, deliver string, targetPR int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "candyland %s started: %s\n", kind, id)
	fmt.Fprintf(&b, "Deliver: %s (%s)\n", deliver, questDeliveryEffect(deliver, targetPR))
	fmt.Fprintf(&b, "API: %s\n", candylandBaseURL)
	fmt.Fprintf(&b, "UI / Dashboard: %s\n", candylandDashboardURL)
	fmt.Fprintf(&b, "Remote/WSL: forward BOTH ports — the UI on :8080 stays empty until the API on :8888 is reachable\n")
	return b.String()
}

// runQuestCmd is the `detritus --quest-run` / `--adventure-run
// <objective-file> [folder ...]` handler: read the objective, classify its
// delivery against live gh state, ensure the sidecar is up, start the quest,
// and print the launch summary. Mirrors runCandyland. convergence is the
// quest's delivery policy — "converge" (--quest-run: bounded, one PR per repo
// at terminal) or "perFinding" (--adventure-run: open-ended, a PR per accepted
// finding); kind names the flow for the summary line.
func runQuestCmd(detritusPath, objectiveFile string, folders []string, kind, convergence, cwd string) error {
	objective, folders, err := readQuestRunArgs(objectiveFile, folders, cwd)
	if err != nil {
		return err
	}
	deliver, targetPR, err := resolveLaunchDelivery(objective, folders[0])
	if err != nil {
		return err
	}
	if err := ensureCandylandUp(detritusPath); err != nil {
		return err
	}
	id, err := startCandylandQuest(objective, deriveShortTitle(objective), folders, deliver, targetPR, convergence)
	if err != nil {
		return err
	}
	fmt.Print(questLaunchSummary(kind, id, deliver, targetPR))
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
// handler: read the campaign input, classify its delivery against live gh state
// (resolveLaunchDelivery — the gh-mirror classification all launchers share),
// ensure the sidecar is up, start the campaign, and print the launch summary.
// Mirrors runQuestCmd. ensureCandylandUp failures are returned so the caller
// exits non-zero.
func runCampaignCmd(detritusPath, inputFile string, folders []string, cwd string) error {
	input, folders, err := readCampaignRunArgs(inputFile, folders, cwd)
	if err != nil {
		return err
	}
	deliver, targetPR, err := resolveLaunchDelivery(input, folders[0])
	if err != nil {
		return err
	}
	if err := ensureCandylandUp(detritusPath); err != nil {
		return err
	}
	id, err := startCandylandCampaign(input, deriveShortTitle(input), folders, deliver, targetPR)
	if err != nil {
		return err
	}
	fmt.Print(campaignLaunchSummary(id, deliver, targetPR))
	return nil
}

// campaignLaunchSummary renders the launch output for a started campaign: the
// campaign id, the derived delivery mode with an honest what-it-produces line
// (questDeliveryEffect, reused), and the API + UI ports. Mirrors
// questLaunchSummary's mode line so feedback/review/pr read the same across
// both flows.
func campaignLaunchSummary(id, deliver string, targetPR int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "candyland campaign started: %s\n", id)
	fmt.Fprintf(&b, "Deliver: %s (%s)\n", deliver, questDeliveryEffect(deliver, targetPR))
	fmt.Fprintf(&b, "API: %s\n", candylandBaseURL)
	fmt.Fprintf(&b, "UI / Dashboard: %s\n", candylandDashboardURL)
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
	// Enumerate the origin Claude session's OTHER MCP servers (obsidian, project
	// tools, etc.) from ~/.claude.json and hand them to candyland so it can graft
	// the SAME tool surface into each spawned agent's --mcp-config. We write the
	// --mcp-config-shaped JSON to a private temp file and pass only its PATH via
	// CANDYLAND_INHERITED_MCP — never the inline JSON. The servers may carry secret
	// env values (API keys); keeping them in a 0600 file rather than an env value
	// avoids leaking them through process/env inspection. Best-effort: a
	// missing/unreadable config or a write failure just means no extra servers to
	// inherit, never a failed launch, and no secret is ever printed. detritus
	// rides via DETRITUS_BIN above and candyland is never a child server, so both
	// are dropped from the inherited set.
	if path, err := writeOriginMCPConfigFile(readOriginMCPServers(originClaudeConfigPath(), originCWD())); err == nil && path != "" {
		cmd.Env = append(cmd.Env, detritusOriginMCPEnv+"="+path)
	}
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

// detritusOriginMCPEnv is the env var carrying the origin session's inherited
// MCP servers to candyland. Its VALUE is the path to a temp JSON file holding an
// --mcp-config-shaped object ({"mcpServers": {...}}), NOT the JSON itself — the
// servers can contain secret env values, so we never place them in an env value.
// Empty/absent means nothing extra to inherit.
//
// The name MUST match what candyland's conductor reads (inheritedMCPServers in
// internal/conductor/coordinator.go). candyland reads CANDYLAND_INHERITED_MCP as
// the path to an --mcp-config file and merges those servers into every spawned
// agent's config, so we export under that exact name — otherwise the handshake is
// silently dropped and no inherited servers reach the agents.
const detritusOriginMCPEnv = "CANDYLAND_INHERITED_MCP"

// originMCPExcluded are server names never propagated as inherited servers.
// detritus rides via DETRITUS_BIN (candyland registers it passively) and
// candyland is deliberately never registered as a child MCP (see setup.go), so
// re-inheriting either would double-register or recurse.
var originMCPExcluded = map[string]bool{"detritus": true, "candyland": true}

// originClaudeConfigPath is the origin Claude session's config file
// (~/.claude.json), where its MCP servers are registered. Returns "" when the
// home directory can't be resolved, which readOriginMCPServers degrades to "no
// servers to inherit".
func originClaudeConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".claude.json")
}

// originCWD is the current working directory used to resolve the project-scoped
// MCP servers, or "" when it can't be determined (only the global servers apply).
func originCWD() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return cwd
}

// readOriginMCPServers reads claudeConfigPath and returns the origin session's
// MCP servers for cwd (mcpServersFromConfig). It is best-effort: a
// missing/empty/unparseable config or empty path yields an empty map (nothing to
// inherit), never an error — inheriting the tool surface must never fail a launch.
func readOriginMCPServers(claudeConfigPath, cwd string) map[string]any {
	if claudeConfigPath == "" {
		return map[string]any{}
	}
	raw, err := os.ReadFile(claudeConfigPath)
	if err != nil || len(raw) == 0 {
		return map[string]any{}
	}
	data := map[string]any{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return map[string]any{}
	}
	return mcpServersFromConfig(data, cwd)
}

// mcpServersFromConfig extracts the inheritable MCP servers from a parsed
// ~/.claude.json: the global "mcpServers" map merged with the project-scoped map
// for cwd ("projects".<cwd>."mcpServers"), the project scope winning on key
// collisions (it is the more specific one the origin session actually loaded).
// Servers in originMCPExcluded are dropped. Pure (parsed config + cwd in → map
// out) so the merge/exclusion is separately testable.
func mcpServersFromConfig(data map[string]any, cwd string) map[string]any {
	out := map[string]any{}
	merge := func(m map[string]any) {
		for name, spec := range m {
			if originMCPExcluded[name] {
				continue
			}
			out[name] = spec
		}
	}
	if global, ok := data["mcpServers"].(map[string]any); ok {
		merge(global)
	}
	if cwd != "" {
		if projects, ok := data["projects"].(map[string]any); ok {
			if project, ok := projects[cwd].(map[string]any); ok {
				if scoped, ok := project["mcpServers"].(map[string]any); ok {
					merge(scoped)
				}
			}
		}
	}
	return out
}

// originMCPConfigPath is the STABLE path of the inherited-MCP config file handed
// to candyland. It is deliberately fixed (not a random temp name): the file must
// outlive our process — candyland re-reads it on every agent spawn for its whole
// lifetime — so we cannot delete it after launch. A stable path means each launch
// overwrites the previous file rather than leaving a new random one behind, so the
// secret-bearing config can never accumulate past a single file in TMPDIR.
func originMCPConfigPath() string {
	return filepath.Join(os.TempDir(), "detritus-origin-mcp.json")
}

// writeOriginMCPConfigFile serializes servers as an --mcp-config-shaped JSON
// object ({"mcpServers": {...}}) and writes it to a private (0600) file at the
// stable originMCPConfigPath, returning that path for propagation via
// CANDYLAND_INHERITED_MCP. It returns ("", nil) when there are no servers to
// inherit (so the caller omits the env var). The file — not an env value —
// carries the servers because they may hold secret env values; 0600 keeps them
// readable only by the current user. Writing to a fixed path (not os.CreateTemp)
// bounds the on-disk secret to a single file that each launch overwrites: candyland
// re-reads it per agent spawn, so it cannot be removed after launch, and random
// per-launch names would otherwise accumulate indefinitely. Any stale
// randomly-named files written by earlier detritus versions are swept first.
func writeOriginMCPConfigFile(servers map[string]any) (string, error) {
	sweepStaleOriginMCPConfigFiles()
	if len(servers) == 0 {
		return "", nil
	}
	out, err := json.Marshal(map[string]any{"mcpServers": servers})
	if err != nil {
		return "", err
	}
	name := originMCPConfigPath()
	// Remove any pre-existing file (possibly a dangling symlink or another user's
	// file) before creating ours with O_EXCL so we never write through it.
	os.Remove(name)
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	if _, err := f.Write(out); err != nil {
		f.Close()
		os.Remove(name)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(name)
		return "", err
	}
	return name, nil
}

// sweepStaleOriginMCPConfigFiles removes secret-bearing config files left in
// TMPDIR by earlier detritus versions, which used random per-launch temp names
// (detritus-origin-mcp-*.json) and never cleaned them up. Best-effort: glob/remove
// errors are ignored — a failed sweep must never block a launch.
func sweepStaleOriginMCPConfigFiles() {
	matches, err := filepath.Glob(filepath.Join(os.TempDir(), "detritus-origin-mcp-*.json"))
	if err != nil {
		return
	}
	for _, m := range matches {
		os.Remove(m)
	}
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

// candylandRunRequest is the body POSTed to /api/runs. Deliver/TargetPR let a
// standalone run address an existing PR in place (feedback/review) instead of
// always opening a new one — omitted (zero) for the default new-PR delivery.
type candylandRunRequest struct {
	Folders  []string `json:"folders"`
	Prompt   string   `json:"prompt"`
	Title    string   `json:"title"`
	Deliver  string   `json:"deliver,omitempty"`
	TargetPR int      `json:"targetPr,omitempty"`
}

// startCandylandRun creates a run (POST /api/runs) and begins it (POST
// /api/runs/{id}/begin), returning the run id. The sidecar must already be up;
// callers run ensureCandylandUp first.
func startCandylandRun(folders []string, prompt, title, deliver string, targetPR int) (string, error) {
	return startCandylandRunAt(candylandBaseURL, folders, prompt, title, deliver, targetPR)
}

// startCandylandRunAt is startCandylandRun against an explicit base URL (test seam).
func startCandylandRunAt(baseURL string, folders []string, prompt, title, deliver string, targetPR int) (string, error) {
	body, err := json.Marshal(candylandRunRequest{Folders: folders, Prompt: prompt, Title: title, Deliver: deliver, TargetPR: targetPR})
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
// full QuestSpec; detritus only sets the fields the /quest and /adventure
// commands settle. Title is the derived short display label (the full objective
// still rides in Objective); Convergence is the delivery policy — "converge"
// (bounded quest, one PR per repo at terminal) or "perFinding" (adventure, a PR
// per accepted finding).
type candylandQuestRequest struct {
	Objective   string   `json:"objective"`
	Title       string   `json:"title"`
	Folders     []string `json:"folders"`
	Deliver     string   `json:"deliver"`
	TargetPR    int      `json:"targetPr"`
	Convergence string   `json:"convergence"`
}

// startCandylandQuest creates a quest (POST /api/quests) and begins it (POST
// /api/quests/{id}/begin), returning the quest id. The sidecar must already be
// up; callers run ensureCandylandUp first. Mirrors startCandylandRun.
// deliver/targetPR are the shared wire contract with the candyland receiver:
// "feedback"|"review" carry targetPR, "pr" carries 0.
func startCandylandQuest(objective, title string, folders []string, deliver string, targetPR int, convergence string) (string, error) {
	return startCandylandQuestAt(candylandBaseURL, objective, title, folders, deliver, targetPR, convergence)
}

// startCandylandQuestAt is startCandylandQuest against an explicit base URL (test seam).
func startCandylandQuestAt(baseURL, objective, title string, folders []string, deliver string, targetPR int, convergence string) (string, error) {
	body, err := json.Marshal(candylandQuestRequest{
		Objective:   objective,
		Title:       title,
		Folders:     folders,
		Deliver:     deliver,
		TargetPR:    targetPR,
		Convergence: convergence,
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
// settles. tokenBudget is left to candyland's default and not set here. Title is
// the derived short display label (the full input still rides in Input).
// deliver/targetPr share the exact wire contract with candylandQuestRequest: the
// campaign launcher classifies them from the input (resolveLaunchDelivery) and
// candyland propagates them to the affected child quests.
type candylandCampaignRequest struct {
	Input    string   `json:"input"`
	Title    string   `json:"title"`
	Folders  []string `json:"folders"`
	Deliver  string   `json:"deliver"`
	TargetPR int      `json:"targetPr"`
}

// startCandylandCampaign creates a campaign (POST /api/campaigns) and begins it
// (POST /api/campaigns/{id}/begin), returning the campaign id. The sidecar must
// already be up; callers run ensureCandylandUp first. Mirrors startCandylandQuest.
// deliver/targetPR carry the input's delivery intent; candyland propagates them
// to the child quests the campaign decomposes into.
func startCandylandCampaign(input, title string, folders []string, deliver string, targetPR int) (string, error) {
	return startCandylandCampaignAt(candylandBaseURL, input, title, folders, deliver, targetPR)
}

// startCandylandCampaignAt is startCandylandCampaign against an explicit base URL (test seam).
func startCandylandCampaignAt(baseURL, input, title string, folders []string, deliver string, targetPR int) (string, error) {
	body, err := json.Marshal(candylandCampaignRequest{
		Input:    input,
		Title:    title,
		Folders:  folders,
		Deliver:  deliver,
		TargetPR: targetPR,
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
