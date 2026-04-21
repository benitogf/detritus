---
description: Address open review feedback on a PR, push fixes, and update the PR body in place — never posts issue/PR comments.
category: meta
triggers:
  - gh-feedback-work
  - address pr feedback
  - pr feedback
  - address review
  - update pr summary
when: User invokes /gh-feedback-work on a PR with outstanding review comments or issue comments asking for changes.
related:
  - meta/gh-issue-create
  - meta/gh-issue-work
---

# /gh-feedback-work — Address PR Feedback, Update PR Body In Place

Read open review feedback on a PR, implement the requested changes, push, and rewrite the PR body so it reflects the current state of the PR. Never post comments on the user's GitHub account from this skill — stale comment threads clutter review, and any post that looks like it came from the user but was actually Claude is confusing.

## Posting to GitHub as the user

When posting anything to GitHub via `gh` or the REST API on the user's behalf, the body MUST end with:

```
---
🤖 Generated with [Claude Code](https://claude.com/claude-code)
```

This applies to PR bodies, issue bodies, comment bodies, release notes. It does NOT apply to commit messages (`Co-Authored-By:` handles commits) or to git push output.

**This skill never calls `POST .../comments`. It writes only to the PR body.**

## Inputs

- `<owner>/<repo>#<pr>` — fully qualified reference.
- Full PR URL — parsed to `<owner>/<repo>#<pr>`.
- Bare `#<pr>` — valid only when cwd is already inside the target repo.

## Phase 0: Track progress

Initialize a `TodoWrite` list mirroring phases 1–6 so the user can see where the flow is at a glance. Update in real time — mark in-progress before starting each phase, completed immediately after.

## Phase 1: Collect feedback

First, find the timestamp of the last commit on the PR branch. This is the cutoff for what counts as "unaddressed":

```
gh api repos/<owner>/<repo>/pulls/<pr>/commits --jq '.[-1].commit.committer.date'
```

Any comment **after** this timestamp is unaddressed feedback and must be handled by this skill. Any comment **before** it was implicitly addressed by a commit (or is stale context). This is simpler and more accurate than classifying every comment in the thread by hand.

Pull from three sources:

```
gh api repos/<owner>/<repo>/pulls/<pr>/comments       # inline review comments
gh api repos/<owner>/<repo>/pulls/<pr>/reviews        # review body text + state
gh api repos/<owner>/<repo>/issues/<pr>/comments      # issue-thread comments on the PR
```

Filter each to `created_at > <last-commit-timestamp>`, then dedupe by author + timestamp.

Also ignore comments authored by the current user themselves (they're signal for context, not action items).

If the filtered set is empty, STOP and report: "No feedback posted since the last commit — nothing to address." Do not proceed to classification.

## Phase 2: Classify

For each post-last-commit feedback item, pick exactly one label:

| Label | Meaning | Action |
|---|---|---|
| **actionable** | asks for a concrete code change | implement in Phase 3 |
| **in-body** | question answerable by clarifying the PR body | answer in Phase 5 rewrite |
| **out-of-scope** | valid but belongs in a separate issue/PR | capture as a follow-up; offer to run `/gh-issue-create` afterwards |

Present the classification to the user. If more than 2 items are **actionable**, WAIT for confirmation before touching code.

(There is no "already-addressed" bucket — Phase 1's timestamp filter handles that implicitly.)

## Phase 3: Address in code

```
gh pr checkout <pr>
```

Then implement each actionable item. Run the package's tests (`go test ./...` for Go, equivalent elsewhere). Do NOT commit if tests regress.

Commit convention:
- One commit per logically grouped feedback item (not one per feedback bullet — group what's cohesive).
- Conventional-commits message (`fix(<scope>): …`, `refactor(<scope>): …`).
- `Co-Authored-By: Claude …` footer in every commit.

## Phase 4: Push

```
git push
```

The branch is already tracking upstream from `gh pr checkout`; no `-u` needed.

## Phase 5: Rewrite PR body in place (not via comments)

Fetch the current body:
```
gh api repos/<owner>/<repo>/pulls/<pr> --jq .body > /tmp/pr-body-current.md
```

Rewrite, don't append. The PR body should read as if it always described the PR's current state — not as a changelog of what changed since the last review, and not a narrative of "I considered X, decided Y". The body is a final-product description, the same way the code diff is. Specifically:

- Keep the existing section structure (`## Summary`, `## Test plan`, etc.).
- Rewrite bullets to describe the latest behavior, not the original proposal.
- Tick any `- [ ]` acceptance checkboxes that now pass.
- If any **in-body** feedback asked a question, answer it in the relevant section inline — don't add a "Q&A" section.
- If any items were classified **out-of-scope**, note them briefly (one line each) so the reviewer knows they were seen; do not expand the PR to cover them.
- Do not include a "self-review" or "steps to get here" section. The body describes the PR's final state, not how it got there.
- Always preserve / re-append the attribution footer:
  ```
  ---
  🤖 Generated with [Claude Code](https://claude.com/claude-code)
  ```

Write via the REST API — `gh pr edit` can surface the Projects-classic GraphQL deprecation as a failure on some repos even when the PATCH succeeds:

```
gh api --method PATCH repos/<owner>/<repo>/pulls/<pr> \
  -f body="$(cat /tmp/pr-body-new.md)" \
  --jq .html_url
```

## Phase 6: Report back

Print, in the terminal (not to GitHub):
- PR URL on its own line.
- A one-line summary of which feedback items were addressed and in which commits.
- If any items were **out-of-scope**, list them and offer to run `/gh-issue-create` to capture each one.

## Guardrails

- Never call `gh api .../comments` with `--method POST` from this skill. No exceptions.
- Never post on the user's GitHub account without the attribution footer. (This skill doesn't post at all, but the rule stands for any future skill that borrows this workflow.)
- If the user explicitly asks for a comment reply after the skill finishes, PRINT the intended text (including the footer) and let the user paste it themselves.
- Don't resolve review conversations — only the reviewer can do that meaningfully. Pushing fixes + rewriting the body is enough signal.
- Don't force-push, don't rebase, don't skip hooks.
- If classification is uncertain or feedback is contradictory, ASK the user before implementing.
