---
description: Hard-review a GitHub PR under truthseeker rigor — verify the PR's own claims, hunt for fragility, demand evidence before flagging OR approving, and post an APPROVE or REQUEST_CHANGES review via `gh api`. Auto-posts without a confirmation gate.
category: meta
triggers:
  - gh-pr
  - review pr
  - review this pr
  - review the pr
  - code review
  - review pull request
  - hard review
when: User wants a rigorous review posted to a GitHub PR. Accepts a full PR URL, `<owner>/<repo>#<n>`, or bare `#<n>` when cwd is a clone of the target repo.
related:
  - meta/gh
  - meta/gh-self-review
  - meta/review-rigor
  - meta/truthseeker
---

# /gh-pr — Hard PR review under truthseeker rigor

Posts an APPROVE or REQUEST_CHANGES review on a posted GitHub PR. The analysis itself — principles, claim verification, second-pass checklist, correctness / fragility / performance / tests / security / scope / conventions / Godot subsections — lives in `meta/review-rigor` and is **shared verbatim** with `/gh-self-review`. Same checklist, applied to whichever diff is in scope.

Auto-posts the review — no confirmation gate. The review decision (APPROVE vs. REQUEST_CHANGES vs. COMMENT) is yours to own.

## Inputs

First match wins:

- Full GitHub PR URL: `https://github.com/<owner>/<repo>/pull/<n>`.
- Fully qualified: `<owner>/<repo>#<n>`.
- Bare `#<n>` — only when cwd is inside a clone of the target repo. Derive `<owner>/<repo>` from `git remote get-url origin`.

Anything else → ask the user which PR via `AskUserQuestion`. Do not guess.

## Conventions

- **Use `gh api` for reads and writes.** Do NOT use `gh pr view` / `gh pr diff` / `gh pr review` — the Projects-classic GraphQL deprecation can make those fail on some repos even when the underlying REST call works.
- **Attribution footer on the review body** — append exactly these three lines, separated from the rest of the body by a single blank line:

  `---`
  `🤖 Generated with [Claude Code](https://claude.com/claude-code)`

  The footer goes **inside** the heredoc (see the post template in Phase 6). Do not append it after the heredoc closes — that introduces stray newlines and can land outside the body.
- **Code refs are fine in review bodies** (unlike issue/PR bodies). File paths, line numbers, function names, specific symbols — include them so the author can act. This is the one carve-out from the otherwise product-focused-bodies rule the rest of the `gh-*` family follows.

## Phase 0: Track progress

Initialize a `TodoWrite` list mirroring phases 1–7. Update in real time.

## Phase 1: Resolve target

Parse input to `<owner>/<repo>/<n>`. For bare `#<n>`:

```
git remote get-url origin | sed -E 's|.*github.com[:/]([^/]+)/([^/.]+)(\.git)?|\1/\2|'
```

If cwd isn't a git repo and the input is bare, ask.

## Phase 2: Fetch PR metadata + short-circuit checks

```
gh api repos/<o>/<r>/pulls/<n> --jq '{number, state, title, user: .user.login, base: .base.ref, head: .head.ref, head_sha: .head.sha, draft, mergeable, mergeable_state, merged_at, closed_at, additions, deletions, changed_files, body}'
gh api user --jq '.login'
```

Capture `head_sha` — Phase 6 pins the review to it via `commit_id` so a mid-review push doesn't silently reattach the review to a tree you didn't read.

Short-circuit — report and stop, do NOT post:

| Condition | Action |
|---|---|
| `state == "closed"` or `merged_at != null` | "Already closed/merged — nothing to review." |
| `draft == true` | "Draft PR — not reviewing. Offer to re-run when marked ready." |
| `user == <current gh user>` | "You're the author — GitHub blocks self-approval. Use `/gh-feedback-work` or post inline via `gh api`." |

`mergeable_state` is informational, not a short-circuit — proceed with the review but call it out in the lead:

| `mergeable_state` | What to surface |
|---|---|
| `"dirty"` | Conflicts against base — author needs to rebase/merge before merge regardless of review verdict. |
| `"behind"` | Branch is behind base — required-status-checks repos will block merge until updated. |
| `"blocked"` | Branch protection is blocking merge (required reviews, required checks, etc.) — note which. |
| `"unknown"` | GitHub hasn't computed it yet; retry once after a few seconds, then drop the line if still unknown. |
| `"clean"` / `"unstable"` / `"has_hooks"` | No call-out needed. |

Note: `mergeable` itself can be `null` while GitHub computes — if so, the retry above applies.

## Phase 3: Gather context (parallel)

Single message, multiple `Bash` calls. **Always pass `--paginate`** on list endpoints — without it, GitHub caps results at 30 per call and the rest are silently dropped, which breaks Phase 4 (you can't check resolution against comments you never fetched).

```
# Files (--paginate; GitHub also hard-caps at 3000 files server-side regardless of pagination — note it in the review if hit)
gh api --paginate repos/<o>/<r>/pulls/<n>/files --jq '.[] | {filename, status, additions, deletions}'

# Prior reviews
gh api --paginate repos/<o>/<r>/pulls/<n>/reviews --jq '.[] | {user: .user.login, state, submitted_at, body}'

# Inline review comments
gh api --paginate repos/<o>/<r>/pulls/<n>/comments --jq '.[] | {user: .user.login, path, line, body, created_at}'

# Issue comments
gh api --paginate repos/<o>/<r>/issues/<n>/comments --jq '.[] | {user: .user.login, body, created_at}'

# Commits
gh api --paginate repos/<o>/<r>/pulls/<n>/commits --jq '.[] | {sha: .sha[0:8], date: .commit.committer.date, msg: .commit.message}'

# Review threads with resolution status (GraphQL — authoritative source for "is this thread resolved?")
gh api graphql -f query='query($owner:String!,$repo:String!,$num:Int!){repository(owner:$owner,name:$repo){pullRequest(number:$num){reviewThreads(first:100){nodes{isResolved isOutdated path line comments(first:20){nodes{author{login} body createdAt}}}}}}}' -F owner=<o> -F repo=<r> -F num=<n>

# Full diff (single object — no --paginate; falls back to per-file patches from /files if this errors due to GitHub's ~20MB diff cap)
gh api repos/<o>/<r>/pulls/<n> -H "Accept: application/vnd.github.v3.diff"
```

If PR body references a linked issue (`Closes #N`, `Fixes #N`, `Resolves #N`, with or without `owner/repo#` prefix, case-insensitive):

```
gh api repos/<o>/<r>/issues/<linked_n> --jq '{title, state, body}'
```

## Phase 4: Timeline cross-check + prior-signal inventory

Two jobs: (a) figure out what's already been resolved so you don't re-flag it, and (b) figure out what's already been said so you don't restate it as your own finding.

### 4a. Resolution check

The GraphQL `reviewThreads` query from Phase 3 is the authoritative source for thread resolution — `isResolved` is what the GitHub UI shows. Trust it over timestamp heuristics.

- **Inline review threads**: use `isResolved` directly. Resolved → don't re-flag. Unresolved → still open, in scope. Outdated (`isOutdated: true`) means the line moved or was deleted; check whether the underlying concern was addressed in the new code, not just whether the anchor still exists.
- **Issue comments and standalone review bodies** have no `isResolved` field. For these, fall back to the timestamp heuristic: compare `created_at` / `submitted_at` against commit dates, scan later commits for evidence the point was addressed, and say "ambiguous" when the evidence isn't clear.
- Never flag a comment as unresolved without verifying it's still open against the current tree.

### 4b. Prior-signal inventory

Before drafting the review, build an explicit list of **what has already been said** across these sources:

- **The PR body itself.** If the author acknowledged a caveat ("Heads-up: X still needs Y"), that point is on the record. You do not discover it.
- **Commit messages.** If a commit message explains *why* something was done, that rationale is on the record.
- **Prior reviews and inline comments.** If another reviewer flagged a blocker, that's on the record.
- **The linked issue body.** If the issue already frames a constraint or trade-off, that's on the record.

Your review contributes **net-new signal** only. Restating known information is review-theater — it pads the review, wastes the author's time, and obscures the new signal you're actually bringing.

- If it's already in the PR body → don't "discover" it. Either skip it, or explicitly acknowledge it ("the author already flagged X — agreed, not a blocker").
- If a prior reviewer already raised it → don't restate it. Skip, or reinforce with a new angle ("agree with @user that X — here's an additional angle").
- If a commit message explains it → don't treat it as a gap. The rationale is documented.
- If the linked issue already covers the trade-off → don't flag it as an oversight.

## Phase 5: Apply review rigor

**First, load the rigor doc fresh.** Call `kb_get(name="meta/review-rigor")` before proceeding — do not rely on memory of the checklist from training data or from earlier in the conversation. The checklist evolves; reading it now is the only way to apply the current version. The earlier related-doc mention in this skill's frontmatter is not a substitute.

Then follow it end-to-end against the PR's diff (combined from the `/files` patches in Phase 3, or the full unified diff). The shared doc covers:

- The truthseeker principles (the bar you're holding the diff to).
- Verifying the PR's claims (linked issue acceptance, perf claims, bug-fix tests, platform claims).
- Classifying scope (docs / deps / generated / code; language gating).
- The "don't stop at easy findings" second pass.
- Correctness / fragility / performance / tests / security / scope-discipline / conventions checklists.
- The Godot subsection (gated on `.gd` / `.tscn` / etc. files in the diff).
- Large-diff handling (>500 lines → prioritize + say so).

Skip nothing. If a subsection's scope didn't fire in the diff, note it didn't apply rather than silently dropping it from your audit.

## Phase 6: Compose review + post

**Be terse.** Every point is a single short bullet — one sentence stating *what* and *where* (file:line). If the *why* isn't obvious, add one short sub-line; otherwise stop. No paragraphs. No "I checked X and verified Y by doing Z" prose. The author can read the diff — your job is to point at things, not explain them. If a finding needs more than two sentences to land, you haven't found the core of it yet.

Structure the body:

1. **Lead** — one or two sentences. What the PR does, the verdict, and if blocking, the one-line summary of what's blocking. No more.
2. **Verified** — only if there's something the reader's confidence depends on. One bullet per item, one line each. Skip the whole section if nothing meets that bar. "Build passed, I read the diff" is not a verified item.
3. **Blockers** — only if present. One bullet per blocker, format: `**short title** — file.go:line. one-sentence what's wrong. one-sentence what unblocks (optional).` No evidence section, no preamble — the file:line + the sentence is the evidence.

   **List every blocker you found. There is no cap.** If you uncovered seven, post seven. If you uncovered fifteen, post fifteen. Silently dropping a blocker because the list "looks long" defeats the skill. Terseness applies *per item*, not to the count of items. Never trim, batch, or "save for a follow-up" a real blocker on the active review.
4. **Non-blocking observations** — **default is to omit this section entirely.** Include only if you have something the author will act on. Each candidate item must pass this bar: *would I open an issue or follow-up PR to fix this? would the author thank me for the heads-up, or roll their eyes?* If you're not sure it clears the bar, drop it.

   Specifically, **never include** an item if any of these apply:
   - It's a naming nit, formatting nit, or doc nit on code outside the PR's scope.
   - It critiques a commit message, PR body wording, or unchecked test-plan checkboxes (the diff is what matters).
   - It's a pre-existing issue the PR didn't introduce.
   - You've already qualified it in the same breath as *"standard for this repo"*, *"pre-existing"*, *"not a finding"*, *"acknowledged status quo"*, *"out of scope"*, or *"trivial"* — those qualifiers are you telling yourself to drop it. Listen.
   - It restates information visible from the diff or PR body without adding analysis.
   - It's "no test for this" when the surrounding code has no test surface to extend (file the follow-up issue separately, don't pad the review).

   Two items is a lot. Four is almost always padding. Zero is the right answer more often than the structure suggests.

   Same terse format as blockers — `**title** — file:line. one sentence.` No paragraphs.

**Calibration — what tight looks like:**

Bad (verbose):

> **Commented-out log left behind** at `monitor.go:95-96` (`// spammy log` + `// log.Println(...)`). Either delete the line entirely (the `continue` on line 96 is the actual behavior change), or rate-limit it. Carrying a commented-out line as the way to "silence" a log invites the next person to uncomment it without thinking. Trivial cleanup.

Good (tight):

> **Commented-out log** — `monitor.go:95-96`. Delete it; if rate-limiting was the intent, do that instead.

The author doesn't need the rationale lecture. They can read the diff.

Decide the event. **The mission is to decide — APPROVE or REQUEST_CHANGES.** A `COMMENT`-only review on a finished review pass defeats the purpose: it leaves the PR in limbo, doesn't gate the merge, and signals that you couldn't commit to a position. Don't do it.

| Finding | `event` |
|---|---|
| No blockers, diff verified | `APPROVE` |
| One or more evidenced blockers | `REQUEST_CHANGES` |
| You couldn't actually finish the review (build broken, repo unreachable, partial coverage) | `COMMENT` — and say explicitly that this is a partial review, not a verdict |

`REQUEST_CHANGES` is the correct event for a blocker-bearing review even on internal-team PRs — it tells the author the change isn't ready in its current form. Don't soften it to `COMMENT` to avoid the appearance of friction; that turns the review into a suggestion box.

If the only thing you have is a non-blocker observation that you genuinely think is worth raising, the right verdict is still `APPROVE` with the observation included — not a `COMMENT` that forces the author to interpret your stance.

Before choosing `APPROVE`, run through this checklist silently:

- Did I read the full diff (or deliberately prioritize within it and say so)?
- For every performance/behavior claim in the PR body, can I point to evidence in the diff or tests that substantiates it?
- Have I checked the linked issue's acceptance criteria against the diff?
- Have I searched for callers of changed functions (if the repo is local)?
- Have I looked for the fragility patterns from `meta/review-rigor`?
- Have I verified prior comments are resolved via Phase 4a?
- Have I filtered my findings against the prior-signal inventory in Phase 4b? (Nothing I'm about to post is restated from the PR body / prior reviews / commit messages / linked issue.)
- After drafting, did I re-read every non-blocker item and ask *"would the author act on this?"* — and delete the ones where the honest answer is no?

If any answer is "no" or "I'm not sure", either do the work, or downgrade to `COMMENT` and say what you didn't verify.

Post. Always pin `commit_id` to the `head_sha` captured in Phase 2 — if the author pushes between read and post, an unpinned review silently reattaches to the new HEAD even though it was written against the old tree.

```
gh api -X POST repos/<o>/<r>/pulls/<n>/reviews \
  -f event=<APPROVE|COMMENT|REQUEST_CHANGES> \
  -f commit_id=<head_sha> \
  -f body="$(cat <<'EOF'
<review body here>

---
🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

If the POST fails with `422 Pull request has been updated since review was started` (the head moved between Phase 2 and Phase 6), do **not** silently retry against the new HEAD — re-run from Phase 3 against the new SHA so prior-signal inventory and resolution checks reflect the current state.

Capture `html_url` from the response.

## Phase 7: Report and return to default branch

Print the review URL on its own line. Follow with one sentence naming the verdict. Nothing else — no recap of the phases, no offer of next steps.

Then return the working tree to the repo's default branch so the user is left on a clean slate (the PR's branch is not their work). Read the default branch from the metadata fetched in Phase 2.

- Skip the checkout if the user is already on the default branch.
- Skip the checkout if cwd is not a git repo (e.g. `/gh-pr` was invoked with a fully-qualified `<owner>/<repo>#<n>` from outside any clone).
- Otherwise: run `git checkout <default_branch>`. Plain checkout — no stash, no force.
  - If it succeeds: confirm with one short line ("← back on master").
  - If git refuses (conflict between uncommitted changes and master): leave the user on the current branch and surface git's exact error message. Do not stash, do not force-discard. Git's refusal is the safety net; the user decides what to do with their tree.

## Guardrails

- Never approve a draft, closed, or self-authored PR.
- Never flag an existing comment as unresolved without verifying against the current tree (Phase 4).
- Never accept a PR body claim as evidence — verify per `meta/review-rigor`.
- Never approve based on "looks good" — approval is a positive claim; back it or don't make it.
- Never flag a concern without evidence — cite the line, the caller, the missing test, or drop it.
- `REQUEST_CHANGES` is the correct event when there are evidenced blockers — do not soften to `COMMENT` to avoid friction.
- Never post a `COMMENT`-only review on a finished review pass — decide. `COMMENT` is reserved for partial reviews where you couldn't actually finish (build broken, repo unreachable), and you must say so explicitly in the body.
- In-scope cleanups (commented-out code, stale comments referencing removed symbols, vestigial config from a removal, debug prints added in this PR) are blockers, not non-blockers. Do not downgrade them.
- Never pad the review with filler strengths to cushion criticism, filler blockers to look thorough, or filler non-blockers to look diligent. "Nothing to flag" is a valid finding.
- Never cap the blocker list. List every blocker you found, however many that is.
- Never include a non-blocker you've qualified with "pre-existing", "standard for this repo", "out of scope", "trivial", etc. — those qualifiers are you arguing against your own item.
- Never restate a point already made in the PR body, a prior review, a commit message, or the linked issue (Phase 4b).
- Never omit the attribution footer.
- Never quote secrets inline even when flagging — point at the line and describe the class.
- Never ask the user something researchable. The repo, the KB, and the GitHub API are all reachable.
- Never leave the user on the PR's branch when the skill ends if a plain `git checkout <default_branch>` would succeed. Don't stash, don't force — if git refuses, leave them put and report.
- If `gh auth status` fails, surface the error and stop.
