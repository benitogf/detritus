---
description: Hard-review a GitHub PR under truthseeker rigor — verify the PR's own claims, hunt for fragility, demand evidence before flagging OR approving, and post an APPROVE or REQUEST_CHANGES review via `gh api`. Auto-posts without a confirmation gate, then keeps watching and re-reviewing the PR until it is merged or closed (`--once` for a single verdict).
triggers:
  - gh-pr
  - review pr
  - review this pr
  - review the pr
  - code review
  - review pull request
  - hard review
argument-hint: "[pr] [--once]"
when: User wants a rigorous review posted to a GitHub PR. By default the skill posts the verdict then keeps watching and re-reviewing the PR until it is merged or closed; `--once` (or "review once" / "just review it") posts one verdict and stops. Accepts a full PR URL, `<owner>/<repo>#<n>`, or bare `#<n>` when cwd is a clone of the target repo.
related:
  - flows/github/gh
  - flows/github/gh-self-review
  - flows/github/babysit
  - core/review-rigor
  - core/janitor-platforms
  - flows/principles/truthseeker
---

# /gh-pr — Hard PR review under truthseeker rigor

Posts an APPROVE or REQUEST_CHANGES review on a posted GitHub PR. The analysis itself — principles, claim verification, second-pass checklist, correctness / fragility / performance / tests / security / scope / conventions / Godot subsections — lives in `core/review-rigor` and is **shared verbatim** with `/gh-self-review`. Same checklist, applied to whichever diff is in scope.

Auto-posts the review — no confirmation gate. The review decision (APPROVE vs. REQUEST_CHANGES vs. COMMENT) is yours to own.

**The default is to keep watching.** After posting the verdict, the skill does not stop — it arms an event-watch on the PR and **re-reviews on each new push or discussion**, re-posting a fresh `commit_id`-pinned verdict **only when the verdict materially changes** — a tier flip or a changed blocker set (Phase 8 defines it) — until the PR is merged or closed. This is the reviewer-seat analogue of `/babysit`'s watch-to-merge: same out-of-band event-watch primitive, but this skill **re-reviews and re-posts a verdict — it never merges, never fixes** (merging is `/babysit`'s job). Pass **`--once`** to post one verdict and stop — today's one-shot behavior; Phases 1–7 run identically, only the watch (Phase 8) is skipped and Phase 7 returns to the default branch immediately.

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
- **Cross-repo references** (citing a sibling repo's PR or issue) default to explicit markdown links — bare `<owner>/<repo>#<n>` shortcuts are unsafe whenever the org slug contains another repo name in the same org as a substring. GitHub's autolinker mangles those org-slug fragments by relinking the inner repo name (e.g. `idnerdidx/bulk#311` smears into nested autolinks via the `idx` substring). Write `[bulk PR #311](https://github.com/idnerdidx/bulk/pull/311)` with the `<owner>/<repo>` pattern kept out of the label. Same-repo bare `#<n>` is unaffected. See the cross-repo-refs convention in `flows/github/gh`.

## Phase 0: Track progress

Initialize a `TodoWrite` list mirroring phases 1–7 (add Phase 8 under the default watch; omit it under `--once`). Update in real time.

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

**First, load the rigor doc fresh.** Call `kb_get(name="core/review-rigor")` before proceeding — do not rely on memory of the checklist from training data or from earlier in the conversation. The checklist evolves; reading it now is the only way to apply the current version. The earlier related-doc mention in this skill's frontmatter is not a substitute.

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

**Cross-repo refs in the review body** default to explicit markdown links — bare `<owner>/<repo>#<n>` shortcuts are unsafe whenever the org slug contains another repo name in the same org as a substring. GitHub's autolinker mangles those org-slug fragments by relinking the inner repo name (e.g. `idnerdidx/bulk#311` smears into nested autolinks via the `idx` substring). When citing a sibling repo's PR or issue from a review, write `[bulk PR #311](https://github.com/idnerdidx/bulk/pull/311)`, keeping the `<owner>/<repo>` pattern out of the label. File:line code refs and same-repo bare `#<n>` are unaffected. See the cross-repo-refs convention in `flows/github/gh`.

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

   **Excluded ≠ discarded.** The list above governs the **posted review body** only. Every verified finding it excludes (pre-existing, out of scope, nit-class) is still reported to the user in the session summary alongside the review, and each real, actionable defect among them gets its own tracked issue via `/gh-issue-create`, labeled `icebox` as an out-of-scope finding parked mid-work (`core/icebox`; `core/review-rigor` → RV-F5). A finding may be kept out of the review; it may never silently vanish.

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
- Have I looked for the fragility patterns from `core/review-rigor`?
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

**Return-to-default timing depends on the mode.** The checkout below returns the working tree to the repo's default branch so the user is left on a clean slate (the PR's branch is not their work). Read the default branch from the metadata fetched in Phase 2.

- **`--once`** — do the return-to-default checkout now, immediately after the single post. This is today's behavior; the skill ends here.
- **Default (watch)** — do **not** `git checkout <default_branch>` yet. The watch (Phase 8) keeps running against this PR, so the return happens when the watch **terminates** (merged / closed / hand-back), not after the first post. Under watch, this phase reports the first verdict and hands to Phase 8; the branch return is the last thing the watch does on termination.

- Skip the checkout if the user is already on the default branch.
- Skip the checkout if cwd is not a git repo (e.g. `/gh-pr` was invoked with a fully-qualified `<owner>/<repo>#<n>` from outside any clone).
- Otherwise: run `git checkout <default_branch>`. Plain checkout — no stash, no force.
  - If it succeeds: confirm with one short line ("← back on master").
  - If git refuses (conflict between uncommitted changes and master): leave the user on the current branch and surface git's exact error message. Do not stash, do not force-discard. Git's refusal is the safety net; the user decides what to do with their tree.

## Phase 8: Watch & re-review (default; skipped under `--once`)

Skip this phase entirely under `--once` (the skill ended at Phase 7). Otherwise, after the first verdict is posted, **keep watching and re-reviewing the PR until it is merged or closed**. This is the reviewer-seat analogue of `/babysit`'s watch-to-merge — same event-watch primitive, but this skill **re-reviews and re-posts a verdict; it never merges, never fixes, never comments outside a review**.

> ### ⛔ The watch must never stall
>
> While the PR is open, a live event-watch MUST be armed — the automatic event-watch (the default), or, *only* when no watch primitive is available, an explicit labeled **degraded-fallback** re-invoke hand-off. "Waiting for the next push" is the watch's normal state, not a stop: the watch stays armed and wakes on the next change. Yielding on an open PR with **neither** an armed watch **nor** a stated degraded hand-off is the stall this contract forbids (mirrors `flows/github/babysit` → *The loop must never stall*). Control returns to the user only on a terminal (merged / closed) or a hand-back.

**Arm the watch — delegate the primitive.** Load `core/janitor-platforms` and arm its **event-watch adapter** on this PR — the same out-of-band poll `/babysit` uses, which wakes the model only on a change and is idle (token-cheap) between events. Do **not** name or hardcode a watch primitive here; the adapter owns it (portability rule — `/babysit` delegates the same way). Report the effective cadence in human terms (e.g. "re-reviewing on each push; checking every 60s"). The adapter's review-watch emit set — a new push / HEAD move, a new discussion (comment), merged, closed — is documented in `core/janitor-platforms`' event-watch adapter section.

**On each emitted event** — the event is a hint, not authority: **re-fetch live PR state** (`gh api`, never `gh pr view` — `flows/github/gh` convention #2), reading the current `head.sha` plus reviews / comments the way Phase 3 does. Then:

1. **Merged or closed?** → terminal. Stop the watch, do the deferred Phase-7 return-to-default checkout, and report the outcome (merged / closed-without-merge). This mirrors `/babysit`'s terminal.
2. **HEAD moved (new push) or new discussion landed since the last review?** → **re-run the review rigor (Phases 3–6) against the new head SHA**, reusing `core/review-rigor` → *Re-review continuity*: held context (the diff understanding, brief, and evidence trail already established), fresh evidence (re-diff the new commits, re-verify against the live tree). This is a **delta re-verify, not a cold re-derive** — the verdict-integrity rules apply unchanged.
3. **Post only when the verdict materially changes.** The "verdict" here is the whole review outcome — **the tier (`APPROVE` / `REQUEST_CHANGES`) *and* its blocker set**, not the tier alone. Compute the new verdict and compare it to the last posted one; re-post a fresh `commit_id`-pinned verdict (Phase 6 post template, pinned to the *new* head SHA) whenever **either** changed materially:
   - **Tier flip** — e.g. a standing `REQUEST_CHANGES` whose blockers a fixing push now clears → **auto-flip to `APPROVE`** and post.
   - **Same tier, different blocker set** — the tier stays `REQUEST_CHANGES` but the blockers changed: a push fixed blocker A and introduced regression B, so the standing review now lists a *resolved* blocker and is silent on a *new* one. This **must re-post** — the outcome the author reads has materially changed. Treating same-tier as "nothing new" is the trap: it leaves a misleading standing review (still citing the now-fixed A) and lets a push-introduced regression (B) go unreported.

   Re-review **silently — no post** **only** when the new verdict is *identical* to the last posted one — same tier **and** an unchanged blocker set (a resolved item is a change; a still-open item carried verbatim is not). That is the sole anti-review-spam case: an identical re-review adds nothing. Record the last-posted verdict **including its blocker set** + the SHA it covered, so the next event compares the full outcome, not just the tier.
4. **Otherwise idle** — the event brought no HEAD move and no new discussion (nothing to re-review), or a silent re-review (step 3) produced an *identical* verdict (same tier, same blocker set) → **confirm the event-watch is still armed** and yield. This is the watch's normal state, not a terminal.

**Notify-only — the watch re-reviews and re-posts a review, nothing more.** It NEVER merges, NEVER comments outside a review, NEVER edits the PR body or branch, NEVER updates-from-base. Merging is `/babysit`'s job; this is the reviewer seat. (Mirrors the adapter's own notify-only rule — `core/janitor-platforms`.)

**No-stall / hand-back rules (adapted from `flows/github/babysit`):**

- While the PR is open, a live watch MUST be armed (automatic event-watch default), or a clearly-labeled degraded-fallback re-invoke hand-off stated when the host can arm no watch primitive (the self-continuation contract in `flows/github/babysit` → *Self-continuation*; verify the primitive is genuinely unavailable before degrading, per `core/janitor-platforms`' availability-verify note). Never refuse to run; degrade instead.
- **No idle-timeout hand-back by default.** An open PR is never abandoned — with no time budget set, the watch stays armed indefinitely, re-reviewing every change until terminal.
- **Hand back on total silence only if the user set a time budget, and the timer RESETS on every event.** The watch hands back only after the *whole budget window* elapses with no event at all; any emitted event (a push, a comment, a mergeability transition) resets the clock. On the budget elapsing in silence: report the PR's current state and tell the user to re-invoke `/gh-pr <pr>` to resume watching — a pause, not an abandonment.

**Every event emits one status line** so the watch is observably alive — naming the event handled, whether HEAD moved, the recomputed verdict, whether it posted or re-reviewed silently, and — on any non-terminal wake — the continuation path (the live event-watch confirmed armed, or the labeled degraded hand-off). A wake that reports without an action AND without a confirmed live watch is the stall signature.

## Guardrails

- Never approve a draft, closed, or self-authored PR.
- Never flag an existing comment as unresolved without verifying against the current tree (Phase 4).
- Never accept a PR body claim as evidence — verify per `core/review-rigor`.
- Never approve based on "looks good" — approval is a positive claim; back it or don't make it.
- Never flag a concern without evidence — cite the line, the caller, the missing test, or drop it.
- `REQUEST_CHANGES` is the correct event when there are evidenced blockers — do not soften to `COMMENT` to avoid friction.
- Never post a `COMMENT`-only review on a finished review pass — decide. `COMMENT` is reserved for partial reviews where you couldn't actually finish (build broken, repo unreachable), and you must say so explicitly in the body.
- In-scope cleanups (commented-out code, stale comments referencing removed symbols, vestigial config from a removal, debug prints added in this PR) are blockers, not non-blockers. Do not downgrade them.
- Never pad the review with filler strengths to cushion criticism, filler blockers to look thorough, or filler non-blockers to look diligent. "Nothing to flag" is a valid finding.
- Never cap the blocker list. List every blocker you found, however many that is.
- Never include a non-blocker you've qualified with "pre-existing", "standard for this repo", "out of scope", "trivial", etc. — those qualifiers are you arguing against your own item. Exclusion is from the review body only: surface the excluded finding to the user and route real defects to a tracked issue labeled `icebox` as an out-of-scope finding parked mid-work (`core/icebox`) — never silently discard (`core/review-rigor` → RV-F5).
- Never restate a point already made in the PR body, a prior review, a commit message, or the linked issue (Phase 4b).
- Never omit the attribution footer.
- Never quote secrets inline even when flagging — point at the line and describe the class.
- Don't write bare `<owner>/<repo>#<n>` cross-repo shortcuts in the review body when the org slug contains another repo name in the same org as a substring. Default to `[<repo> PR #<n>](https://github.com/<owner>/<repo>/pull/<n>)`, keeping the `<owner>/<repo>` pattern out of the label, or GitHub's autolinker will mangle the render. File:line code refs and same-repo bare `#<n>` are unaffected.
- Never ask the user something researchable. The repo, the KB, and the GitHub API are all reachable.
- Never leave the user on the PR's branch when the skill ends if a plain `git checkout <default_branch>` would succeed. Don't stash, don't force — if git refuses, leave them put and report.
- If `gh auth status` fails, surface the error and stop.
- **The watch is notify-only.** Under the default watch (Phase 8) the skill re-reviews and re-posts a REVIEW only — it NEVER merges, NEVER comments outside a review, NEVER edits the PR body or branch. Merging is `/babysit`'s job; this is the reviewer seat.
- **Re-post on a material verdict change; stay silent only on an identical one.** The verdict is the tier **and** its blocker set. Post a fresh `commit_id`-pinned verdict whenever either changed — a tier flip (blockers cleared → auto-flip to APPROVE) **or** the same tier with a different blocker set (A fixed / B introduced still stays `REQUEST_CHANGES`, but the outcome changed, so it re-posts — otherwise the standing review misleads and a push-introduced regression goes unreported). Re-review **silently** only when the new verdict is *identical* (same tier, same open blockers) — that is the sole anti-review-spam case.
- **The watch must never stall.** While the PR is open, a live event-watch MUST be armed (automatic default) or a labeled degraded-fallback re-invoke stated — never yield on an open PR with no watch and no hand-off. No idle-timeout hand-back by default (an open PR is never abandoned); hand back on total silence only if the user set a time budget, and the budget timer RESETS on every event.
- **Delegate the watch primitive — never name it here.** Phase 8 arms `core/janitor-platforms`' event-watch adapter; this skill never names or hardcodes the underlying watch primitive (portability rule; `/babysit` delegates the same way).
- **`--once` is the one-shot opt-out.** `--once` (or "review once" / "just review it") posts one verdict, returns to the default branch, and stops — Phase 8 is skipped. Watch is the default otherwise.
- **Incident hook.** A self-acknowledged mistake/doctrine violation ("you are right, I …", "I didn't follow …", "I ignored /…") or a blocker surfacing on a PR this flow authored is an incident — detect and route per `core/ego` (→ `/grow` / `/absorb`), after finishing the deliverable.
