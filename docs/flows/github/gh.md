---
description: Router for GitHub issue/PR workflows — reads conversation context and dispatches to gh-issue-create, gh-issue-work, gh-feedback-work, gh-self-review, or gh-pr.
triggers:
  - gh
  - github workflow
  - handle github
  - handle issue
  - handle pr
  - handle this github
  - take this to github
when: User invokes /gh as the single entry point for any GitHub issue/PR workflow — creating an issue, working an existing issue, or addressing review feedback — and wants the router to pick the right sub-skill based on context.
related:
  - flows/github/gh-issue-create
  - flows/github/gh-issue-work
  - flows/github/gh-feedback-work
  - flows/github/gh-self-review
  - flows/github/gh-pr
  - flows/github/babysit
---

# /gh — Router for GitHub Issue & PR Workflows

One entry point for the five `gh-*` skills. Reads the conversation + any arguments, decides which sub-skill fits, and hands off. The sub-skills stay focused; this file is the dispatcher and the home for cross-skill conventions so they live in one place. The sidecar launchers (`/candyland`, `/quest`) mirror this router's Phase 1 classification outcome in Go, mapping routes to delivery modes — `core/sidecar` holds the mapping.

## Preconditions (hard gate — every flow that composes /gh inherits this)

Before any GitHub action, verify the CLI is present and authenticated:
```
gh auth status
```
- `gh` not installed, or `gh auth status` non-zero (not logged in / expired token) → **STOP hard.** Do not proceed, do not attempt a degraded/read-only path, do not silently skip. Report the exact remediation: install `gh` (https://cli.github.com) and run `gh auth login`, then re-invoke.
- This is a **capability precondition of detritus itself**, not a per-flow quirk: opening issues/PRs, posting reviews, fetching PR state — nearly every detritus GitHub flow requires it. Any flow composing `/gh` (or `core/kb-writeback`) inherits this gate; none restate it.

## Cross-skill conventions (inherited by all sub-skills)

These apply to every sub-skill this router dispatches to. The sub-skill docs also state them, but the router is the canonical place.

1. **Attribution footer on every body posted to GitHub.** Issue bodies, PR bodies, comment bodies, release notes — all end with:
   ```
   ---
   🤖 Generated with [Claude Code](https://claude.com/claude-code)
   ```
   NOT on commit messages (`Co-Authored-By:` handles commits) and NOT on raw git output.
2. **Use `gh api` for reads and writes, not `gh issue view` / `gh pr view` / `gh pr edit`.** The `gh` subcommands can surface the Projects-classic GraphQL deprecation as a failure on some repos even when the underlying REST call would succeed. `gh api repos/<owner>/<repo>/...` is the stable path.
3. **Product-focused bodies.** Issue bodies, PR bodies, and body rewrites in feedback flow contain no code identifiers / file paths / line numbers / function names. The diff is the technical record; the body is for non-technical reviewers. Two exceptions: (a) a short SHA in a `## Context` section of an issue body when citing regression causation (`gh-issue-create` handles this); (b) `gh-pr` review bodies, which are *for* the author and benefit from file:line refs — that skill explicitly opts out of the product-focused constraint.
4. **The GitHub body is the single source of truth.** The issue body describes the ask; the PR body describes the final state; neither is a changelog. Chat follow-ups edit the relevant body in place via `PATCH`, not via comments. Comments exist only when there is an open question or decision that can't live in the body.
5. **One issue, one PR.** If related work is spotted mid-flow, open a separate issue — don't expand the current one. This applies equally when creating, working, or handling feedback.
6. **Branch from the fetched default, never from the current working branch.** Applies to `gh-issue-work` specifically but is worth restating every time.
7. **Every issue posted carries the `plane` label.** The Plane management app mirrors GitHub issues based on this label. `gh-issue-create` owns applying it; this router just enforces that no other dispatch path bypasses it.
8. **Never commit to or push from the default branch.** Before any `git commit`, verify the current branch is not the default branch. If it is, stop immediately and branch first. There is no valid case for committing directly to the default branch.
9. **Never create a PR from a request that lacks an issue.** If the user asks to work on something and open a PR but does not specify an issue number, issue URL, or issue reference, route to `gh-issue-create` first. Only `gh-issue-work` may continue into PR creation, and only after an issue exists.
10. **Cross-repo references use explicit markdown links by default — never the bare `<owner>/<repo>#<n>` shortcut when an org slug contains another repo name in the same org as a substring.** GitHub's Markdown autolinker re-tokenizes inside rendered text and eagerly relinks any org-slug fragment that matches a different repo in the same org. The canonical failure: the `idnerdidx` org contains a repo named `idx`, so a ref like `idnerdidx/bulk#311` renders as a smear of nested autolinks running through the org slug (the autolinker matches the `idx` substring inside `idnerdidx` as a candidate cross-repo ref to the `idx` repo, on top of the outer `idnerdidx/bulk` link). GitHub itself produces the mangled HTML; downstream mirrors like Plane carry it verbatim, where it shows up as a smear of repeated label fragments before the `#<n>`. The fix is `[<repo> PR #<n>](https://github.com/<owner>/<repo>/pull/<n>)` or `[<repo> #<n>](https://github.com/<owner>/<repo>/issues/<n>)`, with a label that contains **no `<owner>/<repo>` cross-repo pattern** the autolinker could re-match. Verifying that a given org has no substring trap requires knowing the org's full repo list, so default to the markdown-link form for any cross-repo ref — the extra characters are cheap insurance; check the rendered output before assuming a bare shortcut is safe. Same-repo refs (`#<n>` with no `<owner>/<repo>/` prefix) are always safe to leave bare.
11. **Preserve provenance footers on every PR-body rewrite.** A body rewrite (`gh-feedback-work` Phase 6, or any in-place `PATCH`) must re-append not only the attribution footer (#1) but also the candyland unit-ref footer if the body already carries one:
    ```
    🍬 Opened by candyland — <kind> `<id>`
    ```
    (`<kind>` ∈ `run` / `quest`). This is the provenance link `/absorb` (`flows/maintainer/absorb`) reads to resolve the PR's producing candyland unit; dropping it during a rewrite breaks the fast-path unit resolution. Never mint one where it's absent — candyland owns stamping it.
12. **Preserve the learning-loop footer on every PR-body rewrite.** The same re-append discipline as #11 applies to KB PRs opened by the ship-leg learning flows (`/grow`, `/learn`, `/optimize` directly; `/absorb` transitively via `/grow`). Such a PR carries a learning-loop provenance footer:
    ```
    📚 Learned by detritus — <flow> `<ref>`
    ```
    (`<flow>` ∈ `grow` / `learn` / `absorb` / `optimize`; `<ref>` is the source signal — a candyland unit id, a PR URL, or a session tag). Any in-place `PATCH` that rewrites the body must re-append it alongside the attribution footer (#1). `core/kb-writeback` → "Ship a lesson" item 5 (the open-PR step) is the canonical stamping point that owns minting it; a rewrite here only preserves an existing footer and never mints one where it's absent.

## Inputs

- `<issue-or-pr-url>` — full GitHub URL. Router parses it, fetches the resource, and routes based on type + state.
- `#<n>` (bare) — valid only when cwd is inside the target repo. Router fetches and inspects.
- `<owner>/<repo>#<n>` — fully qualified reference to an issue or PR.
- Free-text description — no reference to an existing issue/PR. Router routes to `gh-issue-create`.
- Nothing at all — router scans the recent conversation for a concrete ask. **If the conversation clearly references a specific open issue/PR (most often one acted on earlier in this same turn), that is NOT ambiguous: re-fetch that resource's current state (Phase 0/1) and classify it via the issue/PR rows — do not ask the user what to do.** Only when no concrete problem and no GitHub reference exist does the router fall back to asking. A free-text concrete problem with no reference routes to `gh-issue-create`.

## Phase 0: Locate target repo

- Default: cwd repo (`git remote get-url origin` → parse to `<owner>/<repo>`).
- If cwd is not a git repo, or the conversation references a different repo than cwd, ASK via `AskUserQuestion`.
- Read canonical repo metadata once:
  ```
  gh api repos/<owner>/<repo> --jq '{name, default_branch, full_name}'
  ```

## Phase 1: Classify the input

**Always classify off freshly-fetched state — never a stale snapshot.** On every invocation (including re-invocations within an ongoing PR/issue conversation), re-run the resolution-helper fetches below before classifying. Commits, reviews, and comments fetched earlier in the same turn are stale the moment the author or a CI job touches the PR; a re-invoke almost always means something changed since you last looked. If a classification input is answerable by `gh api` (does a newer commit exist? is there a post-last-commit comment? open or merged?), fetch it — never ask the user to disambiguate what the API resolves.

Apply the first matching rule:

| Input | Route to |
|---|---|
| URL / ref resolves to an **open PR** carrying an unaddressed **CHANGES_REQUESTED** review — a reviewer named concrete findings at/after the current head — **even if the user says "review"** | `gh-feedback-work` — address the reviewer's stated findings. You do NOT run a fresh self-review "past" a posted blocker: a self-review that returns *clean* while an open review says *blocked* is incoherent. Fix what the reviewer named, then push. |
| URL / ref resolves to an **open PR**, AND user text suggests reviewing it — phrases like "review pr", "review this pr", "code review", "hard review", "review pull request" | `gh-pr` |
| URL / ref resolves to an **open PR** with comments posted after the PR's last commit, no review-intent text | `gh-feedback-work` |
| URL / ref resolves to an **open PR**, AND user text wants it watched until merged — phrases like "babysit this pr", "watch this pr to merge", "keep checking the pr until merged", "merge when approved", "/babysit" | `babysit` |
| URL / ref resolves to an **open PR** with no post-last-commit comments and no review-intent text | Ask: review the PR with `gh-pr`, force-run `gh-feedback-work` anyway, or cancel. |
| URL / ref resolves to an **open issue** | `gh-issue-work` |
| URL / ref resolves to a **closed issue or merged/closed PR** | STOP and ask the user whether to reopen, reference it in a new issue, or abandon. Do not silently dispatch. |
| Free-text asking for a self-review / preflight / audit of the **current local diff** (no PR opened yet) — phrases like "audit my changes", "review my diff", "check before PR" | `gh-self-review` |
| Free-text request to work code and open a PR, but no issue is referenced | `gh-issue-create` first, then `gh-issue-work` after the issue exists |
| Free-text problem description, no existing issue referenced | `gh-issue-create` — then offer to chain into `gh-issue-work` after posting |
| Free-text + user references a past commit / regression | `gh-issue-create` with the `## Context` SHA-citation path activated |
| Conversation contains neither a clear problem nor a GitHub reference | Ask via `AskUserQuestion`: "Create new issue / work existing issue / review PR / address PR feedback / cancel?" |

Resolution helpers:

```
# Fetch and check issue/PR type + state in one call:
gh api repos/<owner>/<repo>/issues/<n> --jq '{number, state, pull_request, title}'
# If .pull_request is non-null, this is a PR; use pulls endpoint for commits:
gh api repos/<owner>/<repo>/pulls/<n>/commits --jq '.[-1].commit.committer.date'
# Latest review decision per reviewer — a CHANGES_REQUESTED here means "address it" (gh-feedback-work),
# NOT "self-review it". Never route a PR with an unaddressed CHANGES_REQUESTED to a fresh self-review.
# group_by(.user.login)|map(last …) keeps each reviewer's most-recent decision so one reviewer's
# stale APPROVED can't mask another's standing CHANGES_REQUESTED (a plain `| last` would show only
# the single newest review across all reviewers and hide the block). The select() drops COMMENTED /
# PENDING reviews first, so a chit-chat comment left AFTER a reviewer's CHANGES_REQUESTED can't mask
# their standing block — mirroring GitHub's own "latest decisive review per reviewer" semantics.
gh api repos/<owner>/<repo>/pulls/<n>/reviews --jq '[.[]|select(.state=="APPROVED" or .state=="CHANGES_REQUESTED" or .state=="DISMISSED")]|group_by(.user.login)|map(last|{u:.user.login,state:.state,commit:.commit_id})'
```

## Phase 2: Hand off

Call the selected sub-skill with the resolved context (repo, issue/PR number, original user prompt, any extracted SHA). Do NOT re-do phases the sub-skill will re-do — let the sub-skill fetch the issue/PR body itself. The router's only job after classification is to hand the sub-skill a clean entry point.

`babysit` is the **loop variant**: the router still hands off exactly once, but that single hand-off is a long-running event-driven watch that owns its own lifecycle and delivery (it internally dispatches `/gh-feedback-work` on each change and performs the merge) rather than the usual one-shot "hand off and report." This is one hand-off whose lifecycle lives in the sub-skill — not the router accumulating state across calls.

**Propagate the authorization signal.** When the user's instruction (the `/gh` args or the message that invoked the router) explicitly directed the sub-skill's terminal action — post the issue, open the PR — carry that signal into the handoff so the sub-skill does not re-ask. A sub-skill's confirmation gate (e.g. `gh-issue-create` Phase 4, `gh-issue-work` Phase 8b) exists to confirm an action the user has NOT yet authorized; an explicit instruction routed through `/gh` already authorizes it. Conversely, when you reached the sub-skill by *inferring* a concrete ask from conversation (no explicit post/PR instruction), say so in the handoff — the gate stays live. Never strip a gate the user didn't waive; never re-raise one they did.

If the user confirmed `gh-issue-create` and the issue gets posted, honour the sub-skill's existing offer to chain into `gh-issue-work`. Don't override that flow from here. When the instruction was create-AND-open, `gh-issue-create` Phase 6 skips that offer entirely — its create-and-open branch auto-chains into `gh-issue-work` on the propagated authorization, with no re-ask, and the open-PR authorization rides through to `gh-issue-work` Phase 8b Path B.

## Phase 3: Report

After the sub-skill returns, print:
- The final issue or PR URL on its own line.
- A one-sentence summary of what the sub-skill did.

No summary of the routing decision itself — the result is what matters, not the dispatch.

## Guardrails

- Don't dispatch to a sub-skill without a clear classification. Ambiguous input → ask the user. But a clear in-conversation issue/PR reference is NOT ambiguous — re-fetch and classify it; don't ask.
- Don't classify or ask off a stale snapshot. Re-fetch live PR/issue state (commits, reviews, comments) on every invocation before classifying — a re-invoke usually means the resource changed since you last looked. Never ask the user something `gh api` answers (newer commit? post-last-commit comment? merged?).
- Don't bypass a sub-skill's confirmation gates without authorization — but a gate is not bypassed when the user already authorized the action. `gh-issue-create` gates on "post as-is" only when the post was *not* directed; an explicit instruction routed through `/gh` authorizes it and the router propagates that (Phase 2). Don't re-raise a gate the user waived, and don't strip one they didn't.
- Don't accumulate state across sub-skill calls. Each sub-skill is a unit; the router hands off and reports, nothing more.
- Don't change repos mid-flow. If the user pivots, re-enter `/gh` from the top.
- **Incident hook.** A self-acknowledged mistake/doctrine violation ("you are right, I …", "I didn't follow …", "I ignored /…") or a blocker surfacing on a PR this flow authored is an incident — detect and route per `core/ego` (→ `/grow` / `/absorb`), after finishing the deliverable.
