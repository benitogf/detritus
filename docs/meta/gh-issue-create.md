---
description: Draft a GitHub issue from the current conversation, confirm with the user, post it with the Claude Code attribution footer, then offer next steps (/gh-issue-work, refine, or leave).
category: meta
triggers:
  - gh-issue-create
  - create issue
  - open issue
  - file issue
  - report this
  - track this as an issue
when: User invokes /gh-issue-create to capture something being discussed (bug, feature, follow-up) as a GitHub issue on the active repo.
related:
  - meta/gh-issue-work
  - meta/gh-feedback-work
---

# /gh-issue-create — Draft & File a GitHub Issue

Capture something from the current conversation as a GitHub issue. Always draft first, confirm with the user, then post. Always append the Claude Code attribution footer so it's clear the issue was filed by an agent on the user's behalf.

## Posting to GitHub as the user

When posting anything to GitHub via `gh` or the REST API on the user's behalf, the body MUST end with:

```
---
🤖 Generated with [Claude Code](https://claude.com/claude-code)
```

This applies to issue bodies, PR bodies, comment bodies, release notes. It does NOT apply to commit messages (`Co-Authored-By:` handles commits) or to git push output.

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

Print title + body exactly as they will be posted. Then ask via `AskUserQuestion`:
- **Post as-is** — proceed to Phase 5.
- **Edit title / body first** — collect the user's edits, redraft, re-display, and re-ask.
- **Cancel** — stop, print nothing to GitHub.

Never post without an explicit "post as-is" confirmation.

## Phase 5: Post

Use the REST API (not `gh issue create`, which can surface the Projects-classic GraphQL warning as a failure on some repos):

```
gh api --method POST repos/<owner>/<repo>/issues \
  -f title="<title>" \
  -f body="$(cat /tmp/issue-body.md)" \
  --jq '{number, html_url}'
```

Capture the returned `number` and `html_url`.

## Phase 6: Offer next steps

After posting, ask via `AskUserQuestion`:
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
- Don't post without explicit confirmation. Ever.
- Don't open an issue in a repo the user didn't authorize (ask if ambiguous).
- Don't open obvious duplicates — warn on near-match titles.
- The attribution footer goes on the body, never the title.
- The issue body is the single source of truth for this ask. If the user refines scope in later turns, edit the body in place (`gh api --method PATCH .../issues/<n>`) — don't leave a comment trail that duplicates what the body already says.
