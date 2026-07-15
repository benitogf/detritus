package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
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

// candylandDefaultAPIPort / candylandDefaultSPAPort are the ports candyland binds
// when no override is given. Discovery may land on different ports (a foreign app
// held the default, so detritus launched the sidecar on free ports) — the actual
// endpoint is resolved via the endpoint file (C4) and reflected in the vars below.
const (
	candylandDefaultAPIPort = 8888
	candylandDefaultSPAPort = 8080
)

// candylandAPIURLEnv / candylandDashboardURLEnv override where discovery looks
// before falling back to the default ports — an internal seam (not a user
// surface) so a non-default deployment or a second sidecar bound to other ports
// can be targeted without a rebuild.
const (
	candylandAPIURLEnv       = "CANDYLAND_API_URL"
	candylandDashboardURLEnv = "CANDYLAND_DASHBOARD_URL"
)

// candylandBaseURL / candylandDashboardURL are the RESOLVED endpoint of the
// sidecar detritus is driving this launch. They seed from the env-override seam
// (or the default ports) and ensureCandylandUp rewrites them to the
// discovered/launched endpoint, so every subsequent REST call and printed URL
// points at the real listener regardless of which ports were used. candyland
// owns the contract; detritus only consumes it.
var (
	candylandBaseURL      = envURLOr(candylandAPIURLEnv, fmt.Sprintf("http://127.0.0.1:%d", candylandDefaultAPIPort))
	candylandDashboardURL = envURLOr(candylandDashboardURLEnv, fmt.Sprintf("http://localhost:%d", candylandDefaultSPAPort))
)

// envURLOr returns the trimmed value of env key, or def when it is unset/blank.
func envURLOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// candylandLastIdentity is the verified identity of the sidecar this launch is
// driving, captured by ensureCandylandUp so the launch summary can print the real
// UI outcome (window / browser / headless) rather than an unconditional URL.
var candylandLastIdentity candylandIdentity

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
	deliver, targetPR, degraded, err := resolveCandylandDelivery(prompt, folders[0])
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
	if degraded {
		fmt.Print(degradedClassificationNotice)
	}
	fmt.Printf("API: %s\n", candylandBaseURL)
	fmt.Println(candylandUIOutcomeLine(candylandLastIdentity, candylandDashboardURL))
	return nil
}

// runCandylandUp is the `detritus --candyland-up` handler: bring the sidecar
// online (health check, start it detached if down, poll until ready) and print
// the API + UI ports. It starts NO run — it is the bare "get candyland online"
// entry the dashboard / observability flows use before launching any work, and
// the one /babysit uses to be sure the sidecar is reachable before it begins
// watching a PR. ensureCandylandUp failures are returned so the caller exits
// non-zero.
func runCandylandUp(detritusPath string) error {
	if err := ensureCandylandUp(detritusPath); err != nil {
		return err
	}
	fmt.Printf("candyland is up\n")
	fmt.Printf("API: %s\n", candylandBaseURL)
	fmt.Println(candylandUIOutcomeLine(candylandLastIdentity, candylandDashboardURL))
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
func resolveCandylandDelivery(prompt, repo string) (deliver string, targetPR int, degraded bool, err error) {
	deliver, targetPR, degraded, err = resolveLaunchDelivery(prompt, repo)
	if err != nil {
		return "", 0, false, err
	}
	if deliver == "pr" && targetPR == 0 {
		if pr := branchPRLookup(repo); pr > 0 {
			return "feedback", pr, degraded, nil
		}
	}
	return deliver, targetPR, degraded, nil
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
	bare  bool   // "#N" with no owner/repo — resolve owner/repo from the cwd repo
	text  string // the exact matched substring (used to detect a hand-off vs a prose citation)
}

var (
	refURLRe  = regexp.MustCompile(`(?:https?://)?github\.com/([\w.\-]+)/([\w.\-]+)/(?:pull|issues)/(\d+)`)
	refNWORe  = regexp.MustCompile(`([\w.\-]+)/([\w.\-]+)#(\d+)`)
	refBareRe = regexp.MustCompile(`#(\d+)`)
)

// parseLaunchReference extracts the first PR/issue reference from input: a full
// github.com URL, an owner/repo#N shorthand, or a bare #N (resolved against the
// cwd repo). Returns found=false when the input names no reference (new work).
func parseLaunchReference(input string) (launchReference, bool) {
	if m := refURLRe.FindStringSubmatch(input); m != nil {
		return launchReference{owner: m[1], repo: m[2], num: atoiOr0(m[3]), text: m[0]}, true
	}
	if m := refNWORe.FindStringSubmatch(input); m != nil {
		return launchReference{owner: m[1], repo: m[2], num: atoiOr0(m[3]), text: m[0]}, true
	}
	if m := refBareRe.FindStringSubmatch(input); m != nil {
		return launchReference{num: atoiOr0(m[1]), bare: true, text: m[0]}, true
	}
	return launchReference{}, false
}

// isLaunchHandoff reports whether the input IS the reference (a hand-off) rather
// than merely citing it inside prose (a plan/brief). A hand-off is a single
// trimmed line whose content is the reference alone, optionally preceded by a
// short intent verb phrase ("review ", "address feedback on ", "fix the review
// on ", "update ") and/or trailing punctuation — nothing else. Anything embedded
// in longer or multi-line prose is a citation. This is the predicate that decides
// whether gh state may drive the launch (hand-off) or whether an explicit
// feedback/review intent marker is additionally required (citation).
func isLaunchHandoff(input, refText string) bool {
	trimmed := strings.TrimSpace(input)
	if strings.ContainsAny(trimmed, "\n\r") {
		return false // multi-line prose is always a citation
	}
	idx := strings.Index(trimmed, refText)
	if idx < 0 {
		return false
	}
	after := strings.TrimRight(strings.TrimSpace(trimmed[idx+len(refText):]), " .!?;:,)")
	if after != "" {
		return false // the reference is followed by more than trailing punctuation
	}
	before := strings.TrimSpace(trimmed[:idx])
	if before == "" {
		return true // the reference is the whole objective
	}
	// A leading intent verb phrase is short and clause-punctuation-free; a prose
	// clause ("Rework the pipeline; this supersedes ") is not.
	if strings.ContainsAny(before, ".;:,") {
		return false
	}
	return len(strings.Fields(before)) <= 4
}

// classifyLaunchInput mirrors the flows/github/gh Phase 0/1 classification OUTCOME
// against freshly-fetched (stubbed in tests) gh state, mapped to a delivery mode.
// It is the one classifier all sidecar launchers share.
//
// Launcher inputs are objective/plan TEXT, which routinely cite
// issues/PRs in prose (a settled plan may mention "#97" or a full URL in passing).
// A citation must NOT hijack delivery — that is the safety the old marker-only
// path had. The distinction that matters is NOT bare-vs-explicit; it is whether
// the reference IS the input (a hand-off) or is embedded in prose (a citation),
// decided by isLaunchHandoff for ALL reference forms alike (bare #N, full URL,
// owner/repo#N):
//   - a HAND-OFF (the trimmed input is essentially just the reference, optionally
//     with a short intent verb phrase and trailing punctuation) is always driven
//     by live gh state, the same way /gh treats a URL/ref it is handed;
//   - a CITATION (the reference sits inside longer/multi-line prose) drives gh
//     state only when an explicit STRICT feedback/review phrase marker is also
//     present (hasStrictFeedbackMarker / hasStrictReviewMarker — the same gate the
//     degraded fallback uses); a bare "review"/"feedback" word in a new-work plan
//     does NOT act on the ref, so it stays new work (pr/0, gh untouched).
//
// Rows (see the /gh doc):
//   - no reference → pr (new work; gh untouched)
//   - prose citation with no feedback/review intent marker → pr (new work; gh
//     untouched — the text mentions a ref but does not ask to act on it)
//   - open PR + unaddressed CHANGES_REQUESTED → feedback (even with review text)
//   - open PR + feedback-intent text → feedback (an explicit "address … feedback"
//     phrase wins over a bare "review" word)
//   - open PR + review-intent text → review
//   - open PR + post-last-commit comments, no intent text → feedback
//   - open PR + none of the above → Ambiguous (CLI asks once before launch)
//   - open issue → pr (the issue ref stays in the objective)
//   - merged/closed PR or closed issue on a HAND-OFF → error (verbatim state),
//     aborting launch; the same state reached via a CITATION → pr/0 (a citation
//     of a merged PR must not abort a new-work launch)
//
// A `gh api` failure degrades to marker-only derivation (deriveMarkerDelivery) and
// signals Degraded, so a missing/erroring gh never fails the launch.
func classifyLaunchInput(input, cwd string) (launchClassification, error) {
	ref, found := parseLaunchReference(input)
	if !found {
		return launchClassification{Deliver: "pr"}, nil
	}
	handoff := isLaunchHandoff(input, ref.text)
	if !handoff {
		// Prose citation: gate on the SAME strict phrase markers as the degraded
		// fallback (deriveMarkerDelivery), NOT the loose word-level intent helpers.
		// A new-work plan that merely contains "design review"/"audit the schema"
		// must keep pr/0 and its objective — only an explicit PR-directed phrase
		// ("address feedback on PR #N", "review PR #N") acts on the cited ref.
		lower := strings.ToLower(input)
		if !hasStrictFeedbackMarker(lower) && !hasStrictReviewMarker(lower) {
			return launchClassification{Deliver: "pr"}, nil // prose citation, no intent to act on the ref
		}
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
			if handoff {
				return launchClassification{}, fmt.Errorf("issue #%d in %s/%s is %s — not open work; aborting launch", ref.num, ref.owner, ref.repo, state)
			}
			return launchClassification{Deliver: "pr"}, nil // citation of a non-open issue → new work
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
		if !handoff {
			return launchClassification{Deliver: "pr"}, nil // citation of a merged/closed PR → new work, not an abort
		}
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
//  2. feedback-intent text → feedback (an explicit feedback phrase like
//     "address … feedback" wins over a bare "review" word in the same input —
//     "address review feedback" is a fix-the-feedback ask, not a review ask);
//  3. review-intent text → review;
//  4. comments after the last commit (no intent text) → feedback;
//  5. otherwise "" — the caller marks it Ambiguous and asks once.
func classifyOpenPR(cwd string, ref launchReference, input string) string {
	if unaddressedChangesRequested(cwd, ref) {
		return "feedback"
	}
	lower := strings.ToLower(input)
	if hasFeedbackIntent(lower) {
		return "feedback"
	}
	if hasReviewIntent(lower) {
		return "review"
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
// LOOSE: a bare "review"/"audit" word anywhere counts. Use ONLY where the input
// IS the PR reference (the hand-off path / classifyOpenPR), never to gate a prose
// citation — there a bare word inside a new-work plan must NOT hijack the launch.
func hasReviewIntent(lower string) bool {
	return containsWord(lower, "review") || containsWord(lower, "audit")
}

// hasStrictReviewMarker reports a STRICT review/check-only phrase tied to a PR
// context (questReviewMarkers, e.g. "review pr", "review #"). It is the gate the
// prose-citation branch and the degraded fallback (deriveMarkerDelivery) share,
// so they cannot drift — a loose word like "design review" does not match.
func hasStrictReviewMarker(lower string) bool {
	for _, marker := range questReviewMarkers {
		if containsWord(lower, marker) {
			return true
		}
	}
	return false
}

// hasStrictFeedbackMarker reports a STRICT feedback/fix-on-a-PR phrase
// (questFeedbackMarkers, e.g. "address feedback", "review feedback", "feedback on
// pr"). Like hasStrictReviewMarker it is shared by the prose-citation gate and the
// degraded fallback so both agree; a bare "feedback" word does not match.
func hasStrictFeedbackMarker(lower string) bool {
	for _, marker := range questFeedbackMarkers {
		if containsWord(lower, marker) {
			return true
		}
	}
	return false
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

// degradedClassificationNotice is the launch-summary line printed when delivery
// classification fell back to marker-only derivation because `gh` was unavailable,
// so the operator sees the reduced confidence rather than trusting a silent guess.
const degradedClassificationNotice = "classification degraded: gh unavailable, used marker fallback\n"

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
func resolveLaunchDelivery(input, cwd string) (deliver string, targetPR int, degraded bool, err error) {
	c, err := classifyLaunchInput(input, cwd)
	if err != nil {
		return "", 0, false, err
	}
	if c.Ambiguous {
		switch ambiguousDeliveryPrompt(c.TargetPR) {
		case "review":
			return "review", c.TargetPR, c.Degraded, nil
		case "feedback":
			return "feedback", c.TargetPR, c.Degraded, nil
		default:
			return "", 0, false, fmt.Errorf("launch cancelled: ambiguous delivery for PR #%d", c.TargetPR)
		}
	}
	return c.Deliver, c.TargetPR, c.Degraded, nil
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
// used). Kept conservative — see deriveMarkerDelivery.
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
	if hasStrictFeedbackMarker(lower) {
		return "feedback", pr
	}
	if hasStrictReviewMarker(lower) {
		return "review", pr
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

// extractPerFindingFlag pulls the --per-finding flag out of the trailing quest
// args, returning whether it was present and the remaining args (the folders).
// The flag may appear anywhere among the folders.
func extractPerFindingFlag(args []string) (perFinding bool, folders []string) {
	for _, arg := range args {
		if arg == "--per-finding" {
			perFinding = true
			continue
		}
		folders = append(folders, arg)
	}
	return perFinding, folders
}

// convergenceMode maps the --per-finding flag to the convergence argument
// runQuestCmd expects: perFinding when the flag is set, converge otherwise.
func convergenceMode(perFinding bool) string {
	if perFinding {
		return "perFinding"
	}
	return "converge"
}

// questLaunchSummary renders the launch output for a started quest: the id,
// deliver mode with a what-will/won't-do line (questDeliveryEffect: feedback
// updates PR #N in place, review reports on PR #N, pr opens a new PR), and the
// API + UI ports with a remote/WSL port-forwarding hint. Both delivery modes
// (converge and per-finding) are quests, so the summary always reads "quest".
func questLaunchSummary(id, deliver string, targetPR int, degraded bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "candyland quest started: %s\n", id)
	fmt.Fprintf(&b, "Deliver: %s (%s)\n", deliver, questDeliveryEffect(deliver, targetPR))
	if degraded {
		b.WriteString(degradedClassificationNotice)
	}
	fmt.Fprintf(&b, "API: %s\n", candylandBaseURL)
	fmt.Fprintln(&b, candylandUIOutcomeLine(candylandLastIdentity, candylandDashboardURL))
	fmt.Fprintf(&b, "Remote/WSL: forward BOTH ports — the UI stays empty until the API is reachable (%s)\n", candylandBaseURL)
	return b.String()
}

// runQuestCmd is the `detritus --quest-run <objective-file> [--per-finding]
// [folder ...]` handler: read the objective, classify its delivery against live
// gh state, ensure the sidecar is up, start the quest, and print the launch
// summary. Mirrors runCandyland. convergence is the quest's delivery policy —
// "converge" (bounded, one PR per repo at terminal) or "perFinding"
// (--per-finding: open-ended, a PR per accepted finding).
func runQuestCmd(detritusPath, objectiveFile string, folders []string, convergence, cwd string) error {
	objective, folders, err := readQuestRunArgs(objectiveFile, folders, cwd)
	if err != nil {
		return err
	}
	deliver, targetPR, degraded, err := resolveLaunchDelivery(objective, folders[0])
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
	fmt.Print(questLaunchSummary(id, deliver, targetPR, degraded))
	return nil
}

// ensureCandylandUp returns nil once a candyland sidecar this launch can drive is
// online, with candylandBaseURL/candylandDashboardURL/candylandLastIdentity set to
// the resolved endpoint. It resolves in order (D2): an already-running sidecar
// advertised by the endpoint file, then the default ports — each VERIFIED by health
// body (candylandIdentityAt), never trusting a bare 200. A verified sidecar goes
// through the smart-takeover ladder (D3): a matching, UI-healthy one is driven;
// a version-skewed or UI-degraded one is gracefully restarted ONLY when idle
// (running work is never killed), and fails/warns when busy. Nothing verified →
// launch the installed binary, seamlessly stepping to free ports if a foreign app
// holds the default (D5). detritusPath finds the sibling candyland binary.
func ensureCandylandUp(detritusPath string) error {
	bin, hasBin := candylandBinFor(detritusPath)

	if res, found := resolveExistingCandyland(2 * time.Second); found {
		installed := ""
		if hasBin {
			installed = installedVersionLookup(bin)
		}
		decision := decideTakeover(res.id, installed, displayMarkersDetected(), res.dashURL)
		switch decision.action {
		case takeoverDrive:
			return adoptCandyland(res)
		case takeoverWarnProceed:
			log.Printf("candyland: %s", decision.reason)
			return adoptCandyland(res)
		case takeoverFail:
			return fmt.Errorf("candyland: %s", decision.reason)
		case takeoverRestart:
			// Never shut down a working sidecar we cannot replace — with no
			// installed binary the restart would strand the user with nothing.
			if !hasBin {
				return errCandylandNotInstalled
			}
			log.Printf("candyland: %s", decision.reason)
			if err := shutdownCandyland(res.baseURL, 10*time.Second); err != nil {
				return fmt.Errorf("candyland: graceful takeover of the stale sidecar failed: %w", err)
			}
			// Relaunch the installed binary on the SAME ports the old one held.
			return launchCandyland(detritusPath, bin, res.apiPort, res.spaPort)
		}
	}

	if !hasBin {
		return errCandylandNotInstalled
	}

	apiPort, spaPort := candylandDefaultAPIPort, candylandDefaultSPAPort
	if defaultAPIPortHeld() {
		p1, err1 := pickFreePort()
		p2, err2 := pickFreePort()
		if err1 != nil || err2 != nil {
			return fmt.Errorf("candyland: default port %d is in use and picking free ports failed", candylandDefaultAPIPort)
		}
		apiPort, spaPort = p1, p2
		log.Printf("candyland: default API port %d is held by another app — launching on free ports %d/%d", candylandDefaultAPIPort, apiPort, spaPort)
	}
	return launchCandyland(detritusPath, bin, apiPort, spaPort)
}

// errCandylandNotInstalled is the honest error when no sidecar binary sits beside
// detritus (no release fetched yet), so the launcher never starts a missing binary.
var errCandylandNotInstalled = fmt.Errorf("candyland binary not installed beside detritus (run `detritus --setup` to fetch it)")

// adoptCandyland points the resolved-endpoint vars at a verified sidecar so every
// subsequent REST call and printed URL targets the real listener.
func adoptCandyland(res candylandResolution) error {
	candylandBaseURL = res.baseURL
	candylandDashboardURL = res.dashURL
	candylandLastIdentity = res.id
	return nil
}

// launchCandyland starts the sidecar detached on the given ports and waits for it
// to advertise a verified endpoint, adopting it. Ports are passed explicitly so
// discovery is deterministic regardless of which ports were chosen.
func launchCandyland(detritusPath, bin string, apiPort, spaPort int) error {
	cmd := buildCandylandLaunchCmd(detritusPath, bin, apiPort, spaPort)
	if err := startDetachedProcess(cmd); err != nil {
		return fmt.Errorf("start candyland (%s): %w", bin, err)
	}
	return waitForCandylandHealthy(20 * time.Second)
}

// startDetachedProcess starts cmd in its own session/process group (via
// detachProcess) so the sidecar outlives detritus, then releases the handle. It is
// a var so tests can substitute a spawn seam that captures the built cmd.Args.
var startDetachedProcess = func(cmd *exec.Cmd) error {
	detachProcess(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// buildCandylandLaunchCmd builds the detached launch command for the sidecar on
// the given ports. It always passes --openBrowser so a visible surface materializes
// (D5/C8), and propagates gh/HOME/GH_* creds (via os.Environ()) plus DETRITUS_BIN —
// the detritus executable path — so candyland can register a passive detritus stdio
// MCP ({command: <bin>, args: []}) in each agent's --mcp-config. Each agent then
// spawns its own detritus stdio child, exactly like a VSCode Claude session; there
// is no long-lived shared detritus process. The origin session's OTHER MCP servers
// (obsidian, project tools) are enumerated from ~/.claude.json and handed over via a
// private 0600 file path in CANDYLAND_INHERITED_MCP — never inline JSON, since they
// may carry secret env values. Best-effort: a missing/unreadable config or a write
// failure just means no extra servers to inherit, never a failed launch.
func buildCandylandLaunchCmd(detritusPath, bin string, apiPort, spaPort int) *exec.Cmd {
	cmd := exec.Command(bin, candylandLaunchArgs(apiPort, spaPort)...)
	selfExe, err := os.Executable()
	if err != nil || selfExe == "" {
		selfExe = detritusPath
	}
	cmd.Env = append(os.Environ(), "DETRITUS_BIN="+selfExe)
	if path, err := writeOriginMCPConfigFile(readOriginMCPServers(originClaudeConfigPath(), originCWD())); err == nil && path != "" {
		cmd.Env = append(cmd.Env, detritusOriginMCPEnv+"="+path)
	}
	return cmd
}

// candylandLaunchArgs builds the sidecar launch flags: explicit --port/--spaPort so
// the endpoint is deterministic, and --openBrowser so a takeover relaunch (and any
// launcher-driven start) reopens a visible surface (C8: default false, launcher
// opts in).
func candylandLaunchArgs(apiPort, spaPort int) []string {
	return []string{
		"--port", strconv.Itoa(apiPort),
		"--spaPort", strconv.Itoa(spaPort),
		"--openBrowser",
	}
}

// waitForCandylandHealthy polls until a verified sidecar endpoint resolves (the
// endpoint file it wrote at bind, or the default ports), adopting it, or returns an
// honest error after the deadline.
func waitForCandylandHealthy(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if res, found := resolveExistingCandyland(2 * time.Second); found {
			return adoptCandyland(res)
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("candyland did not become healthy within %s after start", timeout)
}

// pickFreePort grabs an ephemeral TCP port from the OS and immediately releases it,
// returning the number for an explicit --port launch. The tiny TOCTOU window is
// acceptable for a loopback singleton (see Decisions).
func pickFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
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
// lifetime — so we cannot delete it after launch. A stable name means each launch
// overwrites the previous file rather than leaving a new random one behind, so the
// secret-bearing config never accumulates past a single file.
//
// It lives under the per-user cache dir (platformCacheDir), NOT the world-shared
// TempDir: the file carries secret MCP env values, and a fixed name in a shared
// /tmp both leaks a predictable secret path and collides across concurrent users
// (one user's launch would remove/overwrite the file another user's agents are
// still re-reading). A per-user 0700 dir removes both hazards while keeping the
// single-file, no-accumulation property.
func originMCPConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(platformCacheDir(home), "detritus")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "origin-mcp.json"), nil
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
	name, err := originMCPConfigPath()
	if err != nil {
		return "", err
	}
	// Remove any pre-existing file (possibly a dangling symlink or a stale file)
	// before creating ours with O_EXCL so we never write through it.
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

// sweepStaleOriginMCPConfigFiles removes secret-bearing config files earlier
// detritus versions left in the world-shared TMPDIR: the random per-launch names
// (detritus-origin-mcp-*.json) that never got cleaned up, and the later fixed
// name (detritus-origin-mcp.json) now superseded by the per-user cache path.
// Best-effort: glob/remove errors are ignored — a failed sweep must never block
// a launch.
func sweepStaleOriginMCPConfigFiles() {
	patterns := []string{
		filepath.Join(os.TempDir(), "detritus-origin-mcp-*.json"),
		filepath.Join(os.TempDir(), "detritus-origin-mcp.json"),
	}
	for _, p := range patterns {
		matches, err := filepath.Glob(p)
		if err != nil {
			continue
		}
		for _, m := range matches {
			os.Remove(m)
		}
	}
}

// candylandIdentity mirrors the sidecar's /api/health body (C1): who answered and
// what it is doing. preUpgrade marks a 200+version response that predates the
// identity fields (missing pid/activeRuns) — its idleness cannot be verified and it
// has no /api/shutdown, so the takeover ladder warns rather than guess-kills.
type candylandIdentity struct {
	OK           bool
	Version      string
	PID          int
	StartedAt    string
	ActiveRuns   int
	ActiveQuests int
	UI           string
	preUpgrade   bool
}

// errCandylandForeignApp marks a health probe that got a 200 but a body that is not
// candyland (missing ok/version) — a foreign app squatting the port, not a sidecar
// (D1: never trust a bare 200). The free-port fallback itself keys on the decisive
// bind probe (defaultAPIPortHeld), which also covers 404-ing and non-HTTP squatters.
var errCandylandForeignApp = errors.New("foreign app answered /api/health (not candyland)")

// candylandIdentityAt verifies the sidecar's IDENTITY, not just a 200 (D1). It
// requires HTTP 200 AND a body parsing to ok:true with a non-empty version;
// a 200 with any other body is errCandylandForeignApp (a foreign app), and a refused
// connection / non-200 is a plain error (down). A 200+version body missing the newer
// pid/activeRuns fields is accepted as a pre-upgrade sidecar (preUpgrade set).
func candylandIdentityAt(baseURL string, timeout time.Duration) (candylandIdentity, error) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(baseURL + "/api/health")
	if err != nil {
		return candylandIdentity{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return candylandIdentity{}, fmt.Errorf("candyland health: HTTP %d", resp.StatusCode)
	}
	var body struct {
		OK           bool   `json:"ok"`
		Version      string `json:"version"`
		PID          *int   `json:"pid"`
		StartedAt    string `json:"startedAt"`
		ActiveRuns   *int   `json:"activeRuns"`
		ActiveQuests *int   `json:"activeQuests"`
		UI           string `json:"ui"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return candylandIdentity{}, fmt.Errorf("candyland health: %w", errCandylandForeignApp)
	}
	if !body.OK || body.Version == "" {
		return candylandIdentity{}, errCandylandForeignApp
	}
	id := candylandIdentity{OK: true, Version: body.Version, StartedAt: body.StartedAt, UI: body.UI}
	id.preUpgrade = body.PID == nil || body.ActiveRuns == nil
	if body.PID != nil {
		id.PID = *body.PID
	}
	if body.ActiveRuns != nil {
		id.ActiveRuns = *body.ActiveRuns
	}
	if body.ActiveQuests != nil {
		id.ActiveQuests = *body.ActiveQuests
	}
	return id, nil
}

// defaultAPIPortHeld reports whether SOMETHING holds the default API port, so the
// launcher steps to free ports (D5). It is called only after resolveExistingCandyland
// failed to verify a sidecar there, so any listener — an HTTP app that 404s
// /api/health, a non-HTTP squatter — is by elimination foreign. A successful bind
// (immediately released) is the decisive free-port check; the tiny TOCTOU window
// before the launch rebinds is the same accepted race as pickFreePort.
func defaultAPIPortHeld() bool {
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", candylandDefaultAPIPort))
	if err != nil {
		return true
	}
	l.Close()
	return false
}

// candylandURLs builds the REST base and dashboard URLs for a port pair.
func candylandURLs(apiPort, spaPort int) (baseURL, dashURL string) {
	return fmt.Sprintf("http://127.0.0.1:%d", apiPort), fmt.Sprintf("http://localhost:%d", spaPort)
}

// candylandEndpoint mirrors ~/.candyland/endpoint.json (C4): the sidecar's advertised
// ports and identity, written at bind and removed on clean exit. Stale files are
// harmless — every consumer verifies via health before trusting it (D2).
type candylandEndpoint struct {
	APIPort   int    `json:"apiPort"`
	SPAPort   int    `json:"spaPort"`
	PID       int    `json:"pid"`
	Version   string `json:"version"`
	StartedAt string `json:"startedAt"`
}

// candylandEndpointPath is the fixed per-user endpoint-advertisement file, independent
// of --dataPath (discovery must not depend on knowing the data path). "" when the
// home dir can't be resolved.
func candylandEndpointPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".candyland", "endpoint.json")
}

// readCandylandEndpoint reads and parses the endpoint file. ok is false on any
// failure (absent, unreadable, malformed, or no API port) — the caller falls through
// to the default ports.
func readCandylandEndpoint(path string) (candylandEndpoint, bool) {
	if path == "" {
		return candylandEndpoint{}, false
	}
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) == 0 {
		return candylandEndpoint{}, false
	}
	var ep candylandEndpoint
	if err := json.Unmarshal(raw, &ep); err != nil {
		return candylandEndpoint{}, false
	}
	if ep.APIPort == 0 {
		return candylandEndpoint{}, false
	}
	return ep, true
}

// candylandResolution is a VERIFIED existing sidecar: its identity plus the URLs and
// ports to reach it. Ports are retained so a takeover relaunch reuses the same ports.
type candylandResolution struct {
	id      candylandIdentity
	baseURL string
	dashURL string
	apiPort int
	spaPort int
}

// resolveExistingCandyland finds an already-running, VERIFIED sidecar (D2): the
// endpoint file's advertised port first (verified via health), then the
// env-override/default URLs (the internal seam above). A stale endpoint file
// (dead port) fails verification and falls through. found is false when nothing
// verifies.
func resolveExistingCandyland(timeout time.Duration) (candylandResolution, bool) {
	if ep, ok := readCandylandEndpoint(candylandEndpointPath()); ok {
		base, dash := candylandURLs(ep.APIPort, ep.SPAPort)
		if id, err := candylandIdentityAt(base, timeout); err == nil {
			return candylandResolution{id: id, baseURL: base, dashURL: dash, apiPort: ep.APIPort, spaPort: ep.SPAPort}, true
		}
	}
	defBase, defDash := candylandURLs(candylandDefaultAPIPort, candylandDefaultSPAPort)
	base := envURLOr(candylandAPIURLEnv, defBase)
	dash := envURLOr(candylandDashboardURLEnv, defDash)
	if id, err := candylandIdentityAt(base, timeout); err == nil {
		return candylandResolution{id: id, baseURL: base, dashURL: dash,
			apiPort: portOfURL(base, candylandDefaultAPIPort), spaPort: portOfURL(dash, candylandDefaultSPAPort)}, true
	}
	return candylandResolution{}, false
}

// portOfURL extracts the port of rawURL, or def when none is present or the URL
// does not parse — an env-override URL may omit the port or be malformed, and a
// takeover relaunch still needs concrete ports to reuse.
func portOfURL(rawURL string, def int) int {
	u, err := url.Parse(rawURL)
	if err != nil {
		return def
	}
	p, err := strconv.Atoi(u.Port())
	if err != nil {
		return def
	}
	return p
}

// takeoverAction is the smart-takeover ladder's verdict for a verified sidecar (D3).
type takeoverAction int

const (
	takeoverDrive       takeoverAction = iota // match + UI healthy → use as-is
	takeoverWarnProceed                       // degraded but running work must not be blocked → warn, use as-is
	takeoverRestart                           // skew/UI-degraded AND idle → graceful shutdown + relaunch
	takeoverFail                              // skew AND busy → fail, never kill running work
)

// takeoverDecision pairs the action with an actionable reason for the log/error.
type takeoverDecision struct {
	action takeoverAction
	reason string
}

// displayMarkersDetected reports whether detritus sees a usable display (D3): DISPLAY
// or WAYLAND_DISPLAY set, or a WSLg mount present. A var so tests drive it.
var displayMarkersDetected = func() bool {
	if os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" {
		return true
	}
	if _, err := os.Stat("/mnt/wslg"); err == nil {
		return true
	}
	return false
}

// installedVersionLookup returns the version the installed candyland binary reports
// via `--version` (D4), or "" (unknown) on any failure. A var so tests stub it.
var installedVersionLookup = installedCandylandVersion

// installedCandylandVersion execs `<bin> --version` with a 5s timeout and trims the
// output. Non-zero exit / unparseable (an old binary without the flag) → "" (unknown).
func installedCandylandVersion(bin string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// versionEnforcement compares the sidecar's version against the installed binary's.
// enforce is false when either side is unknown ("") or "dev" — transition tolerance:
// dev builds never fight releases and a missing version is never treated as skew
// (D4). skewed is meaningful only when enforce is true.
func versionEnforcement(sidecar, installed string) (skewed, enforce bool) {
	if sidecar == "" || installed == "" || sidecar == "dev" || installed == "dev" {
		return false, false
	}
	return sidecar != installed, true
}

// decideTakeover is the pure smart-takeover ladder (D3). Running work is never
// killed: a busy skew fails, a busy UI-degradation warns and proceeds; only an idle
// sidecar is gracefully restarted. A pre-upgrade sidecar (missing identity fields)
// can't have its idleness verified and has no shutdown endpoint, so it warns and
// proceeds. Version enforcement is skipped (warn only) when either side is dev/unknown.
// dashURL is the RESOLVED dashboard URL of the sidecar under decision (the busy
// warn names it), passed in so the ladder stays pure of the endpoint globals.
func decideTakeover(id candylandIdentity, installedVersion string, displayMarkers bool, dashURL string) takeoverDecision {
	if id.preUpgrade {
		return takeoverDecision{takeoverWarnProceed, fmt.Sprintf("a stale sidecar (version %s) is running and predates identity reporting — cannot verify idleness or shut it down safely; run `detritus --setup` then restart it to refresh", id.Version)}
	}

	idle := id.ActiveRuns == 0 && id.ActiveQuests == 0
	skewed, enforce := versionEnforcement(id.Version, installedVersion)

	if enforce && skewed {
		if idle {
			return takeoverDecision{takeoverRestart, fmt.Sprintf("sidecar version %s differs from installed %s and it is idle — gracefully restarting on the installed version", id.Version, installedVersion)}
		}
		return takeoverDecision{takeoverFail, fmt.Sprintf("sidecar version %s differs from installed %s but it is busy (%d running run(s), %d running quest(s)) — refusing to restart; wait for the work to finish or stop it from the dashboard", id.Version, installedVersion, id.ActiveRuns, id.ActiveQuests)}
	}

	if id.UI == "headless" && displayMarkers {
		if idle {
			return takeoverDecision{takeoverRestart, "sidecar is running headless but a display is available and it is idle — gracefully restarting so a window/browser surface opens"}
		}
		return takeoverDecision{takeoverWarnProceed, fmt.Sprintf("sidecar is running headless but a display is available; it is busy (%d running run(s), %d running quest(s)) so it will not be restarted — driving it as-is, dashboard: %s", id.ActiveRuns, id.ActiveQuests, dashURL)}
	}

	// Version enforcement skipped (a dev/unknown build on one side) with the UI
	// healthy: drive it. Transition tolerance — dev builds never fight releases.
	return takeoverDecision{takeoverDrive, ""}
}

// shutdownCandyland asks the sidecar to shut down gracefully (D3): POST /api/shutdown,
// then poll until it stops answering health within deadline. A 409 means the sidecar
// reports active work — surfaced as an error so the caller never kills running work.
func shutdownCandyland(baseURL string, deadline time.Duration) error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(baseURL+"/api/shutdown", "application/json", nil)
	if err != nil {
		return fmt.Errorf("POST /api/shutdown: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return fmt.Errorf("sidecar reports active work (HTTP 409) — refusing to take it over")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("shutdown returned HTTP %d", resp.StatusCode)
	}
	stop := time.Now().Add(deadline)
	for time.Now().Before(stop) {
		if _, err := candylandIdentityAt(baseURL, 1*time.Second); err != nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("sidecar still answering health %s after shutdown request", deadline)
}

// candylandUIOutcomeLine prints the ACTUAL UI outcome (D6) from the driven sidecar's
// reported mode: a window opened, a browser tab opened, or headless (no display) with
// the dashboard URL to reach it manually. An empty/unknown mode (a pre-upgrade
// sidecar reports none) prints only the URL — its display state is not known, so
// no claim is made about it.
func candylandUIOutcomeLine(id candylandIdentity, dashURL string) string {
	switch id.UI {
	case "window":
		return "candyland window opened"
	case "browser":
		return "dashboard opened in your browser"
	case "headless":
		return fmt.Sprintf("no display available — dashboard: %s", dashURL)
	default:
		return fmt.Sprintf("dashboard: %s", dashURL)
	}
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
// full QuestSpec; detritus only sets the fields the /quest command settles.
// Title is the derived short display label (the full objective still rides in
// Objective); Convergence is the delivery policy — "converge" (bounded quest,
// one PR per repo at terminal) or "perFinding" (--per-finding: open-ended, a PR
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
