---
description: Draft a GitHub issue from the current conversation, confirm with the user unless the post was already directed, post it with the Claude Code attribution footer, then offer next steps (/gh-issue-work, refine, or leave).
triggers:
  - gh-issue-create
  - create issue
  - open issue
  - file issue
  - report this
  - track this as an issue
when: User invokes /gh-issue-create to capture something being discussed (bug, feature, follow-up) as a GitHub issue on the active repo.
related:
  - flows/github/gh-issue-work
  - flows/github/gh-feedback-work
---

# /gh-issue-create — Draft & File a GitHub Issue

Capture something from the current conversation as a GitHub issue. Always draft first and show the draft; confirm before posting only when the post wasn't already directed (see Phase 4 — an explicit instruction in the triggering message, `/gh` args, or an upstream `/gh`/`/grow` handoff is the authorization). Always append the Claude Code attribution footer so it's clear the issue was filed by an agent on the user's behalf.

## Posting to GitHub as the user

When posting anything to GitHub via `gh` or the REST API on the user's behalf, the body MUST end with:

```
---
🤖 Generated with [Claude Code](https://claude.com/claude-code)
```

This applies to issue bodies, PR bodies, comment bodies, release notes. It does NOT apply to commit messages (`Co-Authored-By:` handles commits) or to git push output.

## Plane sync — mandatory `plane` label

Every issue this skill creates MUST carry the `plane` label. The Plane management app mirrors GitHub issues into its workspace based on this label, so an issue posted without it will not sync. Apply the label on the POST (Phase 5) and surface it in the draft preview (Phase 4) so the user sees it before confirming.

If the repo doesn't have the label yet, create it first (idempotent — 422 "already_exists" is fine to swallow):

```
gh api --method POST repos/<owner>/<repo>/labels \
  -f name=plane \
  -f color=8B5CF6 \
  -f description="Synced to Plane management app" \
  2>/dev/null || true
```

## Inputs (all optional)

- `<owner>/<repo>` — target a specific repo. Otherwise default to the cwd repo.
- Free-text topic hint — focus the draft on a specific aspect of the conversation.
- Nothing at all — use the current conversation + the cwd repo.

## Phase 0: Locate target repo

- Default: the cwd repo (`git remote get-url origin` → parse to `<owner>/<repo>`).
- If cwd is not a git repo, or the conversation references a different repo than cwd, ASK the user which repo to target.
- Read default branch and canonical repo name from:
  ```
  gh api repos/<owner>/<repo> --jq '{name, default_branch, full_name}'
  ```

## Phase 1: Extract issue content from the conversation

Scan the most recent turns for:

- a concrete problem or ask the user raised ("we need X", "Y is broken", "follow-up: Z")
- relevant constraints the user stated (platform, deadline, stakeholder, preference)
- any reference to a past change — phrasing like "since the refactor", "this broke after X was merged", "used to work before", or a direct commit/PR reference

Rules:
- Synthesize; do **not** quote transcript verbatim.
- Do **not** include the agent's own deliberations or speculation.
- Do **not** reference code identifiers, file paths, or function names. Issues are product-level.
- **Cross-repo references** (citing PRs, issues, or related work in another repo) default to explicit markdown links — bare `<owner>/<repo>#<n>` shortcuts are unsafe whenever the org slug contains another repo name in the same org as a substring. GitHub's autolinker mangles those org-slug fragments by relinking the inner repo name (e.g. `idnerdidx/bulk#311` smears into nested autolinks via the `idx` substring; Plane mirrors the broken HTML). Write `[bulk PR #311](https://github.com/idnerdidx/bulk/pull/311)` or `[bulk #311](https://github.com/idnerdidx/bulk/issues/311)`, keeping the `<owner>/<repo>` pattern out of the label. Same-repo refs (`#<n>` with no `<owner>/<repo>/` prefix) stay valid bare. See the cross-repo-refs convention in `flows/github/gh`.
- If the conversation doesn't contain a concrete ask, STOP and ask the user what the issue should be about.

### Regression causation — when the user references a past change

If the user's description traces the problem to a prior change, find the commit before drafting:

```
git log --oneline -- <affected-area>
git log --grep="<keyword>"
git show --stat <sha>
```

Capture the short SHA and a one-line product-level description of what changed (not the technical diff). This goes in the `## Context` section of the body template below. A SHA is product-level causation — "behavior drifted after `abc123`" — not an implementation detail, so it belongs in the body even under the no-code-identifiers rule.

If the user references a past change but you cannot find the commit, note that in the context section ("user reports this started after a recent change; specific commit not yet identified") rather than omitting the context entirely.

## Phase 2: Draft (product-focused, non-technical)

Title:
- ≤70 chars, plain-language, issue-style (not conventional-commits).
- Describe the outcome, not the implementation.

Body template:
```
## Summary
<1–2 sentences on what needs to happen and why>

## Motivation
<what's the product or user impact today, and why it matters>

## Context
<OPTIONAL — include only when Phase 1 identified a past change as causation.
One or two lines naming the short SHA and the product-level description of what
drifted. Example: "Behavior drifted after abc123 (trendboard layout swap), March 2026."
Omit this section entirely when there's no regression lineage to cite.>

## Acceptance
- [ ] <plain-language check #1>
- [ ] <plain-language check #2>

---
🤖 Generated with [Claude Code](https://claude.com/claude-code)
```

## Phase 3: Duplicate check

Before presenting the draft, list open issues cheaply:
```
gh api 'repos/<owner>/<repo>/issues?state=open&per_page=100' --jq '.[] | select(.pull_request | not) | "\(.number)\t\(.title)"'
```
If any existing title is a substring match or near-match of the draft title, warn the user and include the candidate number(s). User decides whether to override.

## Phase 4: Show draft, confirm

Print title + body exactly as they will be posted, plus the labels line: `Labels: plane`.

**Decide whether a confirmation gate is needed:**

- **The invoking instruction already directed posting** — anywhere in the chain that reached this skill the user explicitly said to create/open/file the issue (e.g. "create the issue and open a PR", "file it", "open an issue for this"). That chain includes: the user's message that triggered this skill directly; the `/gh` args; and the upstream skill that dispatched here (the `/gh` router after it found a concrete ask, or `/grow` Step 6 shipping a KB change the user told it to ship). When the router or `/grow` hands off, it propagates that authorization signal (see `flows/github/gh` Phase 2) — treat a propagated authorization the same as a direct one. That instruction IS the confirmation: show the draft for transparency, then proceed to Phase 5 without a blocking question. Re-asking whether to do the thing the user just told you to do is the redundant-confirmation failure mode — do not reproduce it.
- **The issue content was inferred from conversation with no explicit post instruction anywhere in the chain** — the user discussed a problem but neither they nor the upstream skill directed a post. Here the gate is real: the user hasn't seen the drafted text or authorized the post. Ask via `AskUserQuestion`:
  - **Post as-is** — proceed to Phase 5.
  - **Edit title / body first** — collect the user's edits, redraft, re-display, and re-ask.
  - **Cancel** — stop, print nothing to GitHub.

When in doubt about whether the instruction was explicit, ask. The gate's purpose is letting the user see the drafted content before it goes public, not extracting a yes to a request they already made.

## Phase 5: Post

First, ensure the `plane` label exists on the repo (see "Plane sync" above). Then use the REST API (not `gh issue create`, which can surface the Projects-classic GraphQL warning as a failure on some repos):

```
gh api --method POST repos/<owner>/<repo>/issues \
  -f title="<title>" \
  -f body="$(cat /tmp/issue-body.md)" \
  -f "labels[]=plane" \
  --jq '{number, html_url, labels: [.labels[].name]}'
```

Capture the returned `number` and `html_url`. Verify the response's `labels` array contains `plane` — if not, the sync to Plane will not happen; surface the failure to the user instead of pretending it worked.

## Phase 6: Next steps

**If the post was authorized by a create-AND-open instruction** — the directed action that reached Phase 4 was "create the issue *and* open a PR" (the create-and-open flow, directly or propagated via `/gh`/`/grow`) — do NOT ask "Work it now?". The user already directed the PR half; asking is the redundant-confirmation failure mode. Auto-chain straight into `/gh-issue-work #<n>`, carrying the open-PR authorization forward so its Phase 8b Path B fires (this Phase 6 hand-off is the channel that delivers that propagated signal — gating it behind a question makes Path B unreachable on this flow). Print the issue URL, then continue.

**Otherwise** — the user directed only an issue (not a PR), or the issue was inferred from conversation — ask via `AskUserQuestion`:
- **Work it now** — hand off to `/gh-issue-work #<n>` in the same session.
- **Give feedback to refine it** — collect the user's notes, rewrite the body (keep the footer), and PATCH in place:
  ```
  gh api --method PATCH repos/<owner>/<repo>/issues/<n> \
    -f body="$(cat /tmp/issue-body.md)"
  ```
  Then re-display and re-offer the three choices.
- **Leave it** — print the issue URL and stop. Another dev (or a later session) can pick it up.

## Phase 7: Report

Always end with the issue URL on its own line:
```
https://github.com/<owner>/<repo>/issues/<n>
```

## Guardrails

- Don't include code identifiers / file paths / function names in the issue body. A short SHA in the `## Context` section is the one exception — it's causation metadata, not implementation detail.
- Don't write bare `<owner>/<repo>#<n>` cross-repo shortcuts when the org slug contains another repo name in the same org as a substring. The autolinker re-tokenizes the org slug and the result renders as a smear of nested links (especially in Plane). Default to `[<repo> PR #<n>](https://github.com/<owner>/<repo>/pull/<n>)` or `[<repo> #<n>](https://github.com/<owner>/<repo>/issues/<n>)`, keeping the `<owner>/<repo>` pattern out of the label. Bare `#<n>` for same-repo refs is unaffected.
- Don't post without authorization. An explicit instruction to create/open/file the issue (in the triggering message, the `/gh` args, or an upstream `/gh`/`/grow` handoff that propagated the authorization) IS the authorization — show the draft, then post without re-asking. Only gate behind `AskUserQuestion` when the issue was inferred from conversation with no explicit post instruction. Never re-confirm a post the user already directed.
- Don't open an issue in a repo the user didn't authorize (ask if ambiguous).
- Don't open obvious duplicates — warn on near-match titles.
- The attribution footer goes on the body, never the title.
- The `plane` label is mandatory on every posted issue — without it the Plane management app won't sync the issue. Verify the POST response includes it.
- The issue body is the single source of truth for this ask. If the user refines scope in later turns, edit the body in place (`gh api --method PATCH .../issues/<n>`) — don't leave a comment trail that duplicates what the body already says.
- **Incident hook.** A self-acknowledged mistake/doctrine violation ("you are right, I …", "I didn't follow …", "I ignored /…") surfaced while drafting the issue is an incident — detect and route per `core/ego` (→ `/grow`), after finishing the deliverable.
