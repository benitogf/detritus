---
description: Router for GitHub issue/PR workflows — reads conversation context and dispatches to gh-issue-create, gh-issue-work, or gh-feedback-work.
category: meta
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
  - meta/gh-issue-create
  - meta/gh-issue-work
  - meta/gh-feedback-work
---

# /gh — Router for GitHub Issue & PR Workflows

One entry point for the three `gh-*` skills. Reads the conversation + any arguments, decides which sub-skill fits, and hands off. The three sub-skills stay focused; this file is the dispatcher and the home for cross-skill conventions so they live in one place.

## Cross-skill conventions (inherited by all three sub-skills)

These apply to every sub-skill this router dispatches to. The sub-skill docs also state them, but the router is the canonical place.

1. **Attribution footer on every body posted to GitHub.** Issue bodies, PR bodies, comment bodies, release notes — all end with:
   ```
   ---
   🤖 Generated with [Claude Code](https://claude.com/claude-code)
   ```
   NOT on commit messages (`Co-Authored-By:` handles commits) and NOT on raw git output.
2. **Use `gh api` for reads and writes, not `gh issue view` / `gh pr view` / `gh pr edit`.** The `gh` subcommands can surface the Projects-classic GraphQL deprecation as a failure on some repos even when the underlying REST call would succeed. `gh api repos/<owner>/<repo>/...` is the stable path.
3. **Product-focused bodies.** Issue bodies, PR bodies, and body rewrites in feedback flow contain no code identifiers / file paths / line numbers / function names. The diff is the technical record; the body is for non-technical reviewers. The one exception: a short SHA in a `## Context` section of an issue body when citing regression causation (`gh-issue-create` handles this).
4. **The GitHub body is the single source of truth.** The issue body describes the ask; the PR body describes the final state; neither is a changelog. Chat follow-ups edit the relevant body in place via `PATCH`, not via comments. Comments exist only when there is an open question or decision that can't live in the body.
5. **One issue, one PR.** If related work is spotted mid-flow, open a separate issue — don't expand the current one. This applies equally when creating, working, or handling feedback.
6. **Branch from the fetched default, never from the current working branch.** Applies to `gh-issue-work` specifically but is worth restating every time.

## Inputs

- `<issue-or-pr-url>` — full GitHub URL. Router parses it, fetches the resource, and routes based on type + state.
- `#<n>` (bare) — valid only when cwd is inside the target repo. Router fetches and inspects.
- `<owner>/<repo>#<n>` — fully qualified reference to an issue or PR.
- Free-text description — no reference to an existing issue/PR. Router routes to `gh-issue-create`.
- Nothing at all — router scans the recent conversation for a concrete ask; if it finds one, routes to `gh-issue-create`; if ambiguous, asks the user.

## Phase 0: Locate target repo

- Default: cwd repo (`git remote get-url origin` → parse to `<owner>/<repo>`).
- If cwd is not a git repo, or the conversation references a different repo than cwd, ASK via `AskUserQuestion`.
- Read canonical repo metadata once:
  ```
  gh api repos/<owner>/<repo> --jq '{name, default_branch, full_name}'
  ```

## Phase 1: Classify the input

Apply the first matching rule:

| Input | Route to |
|---|---|
| URL / ref resolves to an **open PR** with comments posted after the PR's last commit | `gh-feedback-work` |
| URL / ref resolves to an **open PR** with no post-last-commit comments | STOP and report "No feedback posted since the last commit — nothing to address." Offer `gh-feedback-work` anyway if the user insists. |
| URL / ref resolves to an **open issue** | `gh-issue-work` |
| URL / ref resolves to a **closed issue or merged/closed PR** | STOP and ask the user whether to reopen, reference it in a new issue, or abandon. Do not silently dispatch. |
| Free-text problem description, no existing issue referenced | `gh-issue-create` — then offer to chain into `gh-issue-work` after posting |
| Free-text + user references a past commit / regression | `gh-issue-create` with the `## Context` SHA-citation path activated |
| Conversation contains neither a clear problem nor a GitHub reference | Ask via `AskUserQuestion`: "Create new issue / work existing issue / address PR feedback / cancel?" |

Resolution helpers:

```
# Fetch and check issue/PR type + state in one call:
gh api repos/<owner>/<repo>/issues/<n> --jq '{number, state, pull_request, title}'
# If .pull_request is non-null, this is a PR; use pulls endpoint for commits:
gh api repos/<owner>/<repo>/pulls/<n>/commits --jq '.[-1].commit.committer.date'
```

## Phase 2: Hand off

Call the selected sub-skill with the resolved context (repo, issue/PR number, original user prompt, any extracted SHA). Do NOT re-do phases the sub-skill will re-do — let the sub-skill fetch the issue/PR body itself. The router's only job after classification is to hand the sub-skill a clean entry point.

If the user confirmed `gh-issue-create` and the issue gets posted, honour the sub-skill's existing offer to chain into `gh-issue-work`. Don't override that flow from here.

## Phase 3: Report

After the sub-skill returns, print:
- The final issue or PR URL on its own line.
- A one-sentence summary of what the sub-skill did.

No summary of the routing decision itself — the result is what matters, not the dispatch.

## Guardrails

- Don't dispatch to a sub-skill without a clear classification. Ambiguous input → ask the user.
- Don't bypass a sub-skill's confirmation gates. `gh-issue-create` requires explicit "post as-is" — the router doesn't override that.
- Don't accumulate state across sub-skill calls. Each sub-skill is a unit; the router hands off and reports, nothing more.
- Don't change repos mid-flow. If the user pivots, re-enter `/gh` from the top.
