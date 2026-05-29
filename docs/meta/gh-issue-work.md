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
  - meta/gh-self-review
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

This skill is issue-only. If the request does not already include an issue reference, stop and route to `gh-issue-create` first. Do not draft or open a PR from free-form work without an issue linked to it.

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

**Pre-flight: detect wrong-branch state.** Before branching, check whether the current branch is the default branch AND has commits not yet in a PR:
```
current=$(git rev-parse --abbrev-ref HEAD)
default=$(gh api repos/<owner>/<repo> --jq .default_branch)
if [ "$current" = "$default" ]; then
  ahead=$(git rev-list origin/$default..HEAD --count)
  # if ahead > 0, we are on the default branch with unpushed or already-pushed
  # commits that bypassed the PR flow — this is the recovery scenario
fi
```
If on the default branch with commits ahead of `origin/<default>` (or recently pushed directly): **STOP**. State the situation to the user, and require them to move the work onto a feature branch before any more changes are made. Do not silently branch from a dirty default.

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

Pre-commit branch check (mandatory):

```
current=$(git branch --show-current)
default=$(gh api repos/<owner>/<repo> --jq .default_branch)
if [ -z "$current" ]; then
  STOP — detached HEAD. Do not commit. Return to Phase 4 and check out a named feature branch.
fi
if [ "$current" = "$default" ]; then
  STOP — you are on the default branch. Do not commit.
  Return to Phase 4, create a feature branch, and re-apply the changes there.
fi
```

## Phase 7: Push

```
git push -u origin <branch>
```

## Phase 8: Self-review + confirm

This phase has two sub-steps. **8a is unconditional and never skipped.** 8b is the only part that may be skipped by a prior user directive.

### 8a. Sub-agent review — ALWAYS RUN

Delegate the review to a **fresh sub-agent** so the audit runs without the conversational context that produced the code. This mirrors how `/gh-self-review` and `/gh-pr` work — the author's blind spots stay with the author; the sub-agent sees only the diff and the stated intent.

**This step is non-negotiable.** The user directing PR creation does NOT remove it. "Open the PR" is a directive about 8b, not about 8a. Skipping the sub-agent review because the user said "open the PR" is the exact failure mode this phase exists to prevent — the whole point of the fresh-agent review is that the conversation does not influence it.

Collect the full brief (the sub-agent has no conversation context — it must be self-contained):
```
git log origin/<default_branch>..HEAD --oneline
git diff origin/<default_branch>...HEAD --stat
git diff origin/<default_branch>...HEAD        # full diff
```
Also fetch the issue body if available: `gh api repos/<owner>/<repo>/issues/<n> --jq '{title, body}'`.

Launch a sub-agent (`Explore` or equivalent) with a prompt that includes:
- The full diff text
- The issue title + body
- The branch name and commit messages
- Instruction to load `meta/review-rigor` via `kb_get` and apply it end-to-end
- Instruction to return a triage block: **blockers** (must fix before PR) and **non-blockers** (nice-to-have or future work)
- Instruction NOT to write code, NOT to post anywhere — output only

Surface the full triage block to the user. If there are blockers, stop and return to Phase 6 to fix them; do not proceed to 8b.

**The triage is INTERNAL. Never post it as a PR comment, PR review, or issue comment.** This is the key difference from `/gh-pr`: `/gh-pr` reviews someone else's PR and posts an APPROVE / REQUEST_CHANGES verdict; `/gh-self-review` (and this Phase 8a, which embeds it) reviews your own pending change and feeds the findings back into your own work. Posting your own triage to the PR pollutes the review surface with author noise that real reviewers must wade through, and it conflates self-audit (drives edits before the PR settles) with peer-review (the verdict on a settled PR). Use the triage to drive fixes — amend commits, add new commits, or, for non-blockers that real reviewers should still know about, fold them into the PR body as a "Known non-blockers" section (this is the only sanctioned channel; never a comment or review). Then re-run 8a on the updated diff. Never `gh api ... comments` or `gh pr review` from this phase. See [/gh-self-review](gh-self-review.md) for the full self-review-vs-/gh-pr contract.

### 8b. Confirmation gate

Default behavior — ask via `AskUserQuestion`:
- **Open PR as-is** — proceed to the next phase (only valid when no blockers remain).
- **Edit first** — stop, collect the user's notes, amend or add commits on the branch, then re-enter this phase from the top.
- **Cancel** — stop. The branch stays pushed; no PR is opened.

**Skip 8b only when ALL of the following hold:**
- 8a has run on the *current* diff and returned no blockers, AND
- The user's latest message — in the same conversation, with no `/clear` and no re-entry through `/gh` since — explicitly told you to open / push / create the PR, AND
- The diff has not changed since that directive (no new commits, no amends, no fixes from 8a's findings), AND
- An issue already exists and is linked.

If 8a forced any amendments (even non-blocker fixes the user is unaware of), the user's prior "open it" is stale — re-ask. When all four hold, proceed straight to Phase 9. Otherwise ask. Asking the user to confirm what they just told you to do is the failure mode 8b's escape exists to prevent — but the escape applies only to 8b. Never open the PR with unresolved blockers, and never skip 8a.

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
