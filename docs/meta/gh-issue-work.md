---
description: Take a GitHub issue end-to-end — branch, fix, test, commit, push, self-review the diff, confirm with the user, then open PR with a product-focused summary and the Claude Code attribution footer.
category: meta
triggers:
  - gh-issue-work
  - work issue
  - handle issue
  - fix issue
  - implement issue
  - create pr from issue
when: User invokes /gh-issue-work with a GitHub issue URL or number and wants the full fix→PR cycle executed.
related:
  - meta/gh-issue-create
  - meta/gh-feedback-work
---

# /gh-issue-work — Issue → Branch → Fix → Self-Review → PR

Take a GitHub issue end-to-end: branch from the default base, implement the fix, run tests, commit, push, self-review the diff and confirm with the user, then open a PR whose body is product-focused (no code identifiers). Always append the Claude Code attribution footer on the PR body so reviewers can tell it was filed by an agent on the user's behalf.

## Posting to GitHub as the user

When posting anything to GitHub via `gh` or the REST API on the user's behalf, the body MUST end with:

```
---
🤖 Generated with [Claude Code](https://claude.com/claude-code)
```

This applies to PR bodies, issue bodies, comment bodies, release notes. It does NOT apply to commit messages (`Co-Authored-By:` handles commits) or to git push output.

## Inputs

- `<owner>/<repo>#<n>` — fully qualified reference.
- Full issue URL — parsed to `<owner>/<repo>#<n>`.
- Bare `#<n>` — valid only when cwd is already inside the target repo.

## Phase 0: Track progress

Initialize a `TodoWrite` list mirroring phases 1–9 so the user can see where the flow is at a glance. Update in real time — mark in-progress before starting each phase, completed immediately after. Skip this only if the entire flow will finish in under two tool calls (rare).

## Phase 1: Fetch issue

Use the REST API (not `gh issue view`, which can fail on repos still using Projects classic):

```
gh api repos/<owner>/<repo>/issues/<n> --jq '{number, title, body, labels: [.labels[].name]}'
```

Parse title, body, labels.

## Phase 2: Locate repo

- Verify cwd matches the target repo via `git remote get-url origin`.
- If not: search the workspace roots for a clone at `**/github.com/<owner>/<repo>` or `**/<repo>`. Never clone remotely.
- If still not found, STOP and ask the user where the clone lives.

## Phase 3: Understand scope

Delegate broad exploration to an `Explore` subagent so the main context stays clean. Prompt it to find:
- files that the issue likely touches
- existing patterns / helpers to reuse instead of reinventing
- tests to extend

No new abstractions unless the issue explicitly demands one. No cleanup drive-bys.

## Phase 4: Branch from the default base

Read the default branch:
```
gh api repos/<owner>/<repo> --jq .default_branch
```

Fetch and branch **from the default base, never from the current working branch**:
```
git fetch origin
git checkout -b <kebab-scoped-branch> origin/<default_branch>
```

The explicit `origin/<default_branch>` base matters. If you run `git checkout -b <new>` without a base, git branches from whatever is currently checked out — which might carry unrelated WIP into the PR. Always branch from the fetched default.

Branch-name convention: derive from issue title.
- Conventional-commits–style prefix matches the planned commit type: `feat/`, `fix/`, `refactor/`, `docs/`, `chore/`.
- Scope is a short kebab-case slug of the issue topic.
- Example: issue titled "baccarat new PairPlus based results" → `feat/baccarat-pair-plus-sidebets`.

## Phase 5: Implement + test

- Edit code directly (small, scoped changes).
- Run the package's tests (`go test ./...` for Go, equivalent for other languages).
- Do NOT proceed to commit if tests regress.
- If the issue is ambiguous, ask the user a targeted question before implementing.

## Phase 6: Commit

Conventional-commits message, HEREDOC, with the `Co-Authored-By:` footer. Example:
```
git commit -m "$(cat <<'EOF'
feat(<scope>): <short imperative summary>

<1–3 lines of context: what changed, why, any non-obvious trade-off>

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

One logical change per commit. Stage specific files (`git add <path> ...`), not `git add -A`.

## Phase 7: Push

```
git push -u origin <branch>
```

## Phase 8: Self-review + confirm

Before opening the PR, review the diff yourself and surface findings to the user. This is the symmetric gate to `gh-issue-create`'s confirm-before-post step.

Inspect what's about to be proposed:
```
git log origin/<default_branch>..HEAD --oneline
git diff origin/<default_branch>...HEAD --stat
git diff origin/<default_branch>...HEAD
```

Read the full diff, then produce a short review:
- **What changed** — 2–4 bullets describing the behavior change the PR lands, not a file-by-file recap.
- **Findings** — anything noteworthy in the diff: files touched outside the issue's scope, TODOs or debug leftovers, missing tests, generated artifacts that don't match the hand-written edits, formatting noise, commented-out code, config drift.
- Say "nothing to flag" explicitly when the diff is clean. Don't invent concerns.

Then ask via `AskUserQuestion`:
- **Open PR as-is** — proceed to the next phase.
- **Edit first** — stop, collect the user's notes, amend or add commits on the branch, then re-enter this phase from the top (re-run the diff, re-summarize, re-ask).
- **Cancel** — stop. The branch stays pushed; no PR is opened.

Never open the PR without an explicit "Open PR as-is".

## Phase 9: Open PR

Title: conventional-commits style, ≤70 chars.

Body — product-focused, no file paths / line numbers / function names / symbol names. Describe what a user / operator / dealer experiences that they didn't before.

```
gh pr create \
  --repo <owner>/<repo> \
  --base <default_branch> \
  --head <branch> \
  --title "<title>" \
  --body "$(cat <<'EOF'
## Summary
Closes #<n> — <one-sentence product description of what now works>.

<2–4 short bullets describing user-visible impact>

## Test plan
- [ ] <plain-language acceptance check #1>
- [ ] <plain-language acceptance check #2>

---
🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

`gh pr create` may print a Projects-classic GraphQL warning on some repos but the PR still gets created; capture the returned URL from stdout.

## Phase 10: Report back

Print the PR URL on its own line, then a one-sentence summary of what was done. No emoji elsewhere in the reply.

## Phase 11: Handle chat follow-up

After the PR is open, the user may give additional input in the same chat session (not as a GitHub review comment). Handle it based on scope:

- **Same scope** (refinement, clarification, correction of the original ask) — edit the issue body in place via `gh api --method PATCH repos/<owner>/<repo>/issues/<n> -f body=...` to reflect the final state. Add follow-up commits to the existing PR branch. Do NOT leave chat-originated changes in the issue's comment thread or as PR comments — the issue body stays the single source of truth.
- **Out-of-scope** (a new problem surfaced while reviewing the first) — open a separate issue via `/gh-issue-create` and a separate PR. Do not expand the current PR.
- **Ambiguous** — ask the user which bucket the input falls into before touching the issue body or adding commits.

GitHub-review-comment feedback (posted on the PR itself) is handled by `/gh-feedback-work`, not here.

## Guardrails

- Don't reference code paths, symbols, or line numbers in the PR body. That belongs in the diff. The body is for the non-technical reader.
- Don't open the PR without an explicit "Open PR as-is" from the user in Phase 8. A pushed branch is recoverable; an open PR pings reviewers.
- Don't force-push, don't rebase shared branches, don't skip hooks.
- Don't post issue/PR comments from this skill — the PR body carries all narrative.
- Don't include the attribution footer on commits (`Co-Authored-By:` already handles commits). Footer is GitHub-UI-only.
- Don't expand scope. If "related" work is spotted while implementing, open a new issue for it instead of piling more commits into this PR — one issue, one PR.
- Don't branch from the current working branch. Always branch from the fetched default (`origin/<default_branch>`). Carrying unrelated WIP into the PR is a silent failure mode.
- The issue body is the single source of truth. Chat follow-ups edit the body in place; don't leave a trail of PR comments or issue comments that duplicate what the body already says.
- If anything blocks (tests fail, scope unclear, repo not found), STOP and surface the blocker to the user. Don't paper over it.
