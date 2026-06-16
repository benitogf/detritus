---
description: Address open review feedback on a PR, push fixes, and update the PR body in place — never posts issue/PR comments.
triggers:
  - gh-feedback-work
  - address pr feedback
  - pr feedback
  - address review
  - update pr summary
when: User invokes /gh-feedback-work on a PR with outstanding review comments or issue comments asking for changes.
related:
  - flows/github/gh-issue-create
  - flows/github/gh-issue-work
  - flows/github/gh-self-review
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

Initialize a `TodoWrite` list mirroring phases 1–7 (including the Phase 1.5 issue-link preflight) so the user can see where the flow is at a glance. Update in real time — mark in-progress before starting each phase, completed immediately after.

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

## Phase 1.5: Verify the PR has a linked issue

Per `flows/github/gh` cross-skill convention #9, no PR should exist without a linked issue. The PR-creation skills enforce this on the way in; this phase enforces it for PRs that bypassed the rule (older PRs, PRs opened directly with `gh pr create`, the pre-fix `/smith` flow, etc.).

Read the PR body:

```
gh api repos/<owner>/<repo>/pulls/<pr> --jq .body
```

Check for any GitHub-recognized closing keyword followed by an issue reference — case-insensitive match against this regex:

```
(?i)(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)[:\s]+(?:(?:[\w.-]+/[\w.-]+)?#[0-9]+|https?://github\.com/[\w.-]+/[\w.-]+/issues/[0-9]+)
```

The keyword alternation enumerates GitHub's nine recognized closing forms exactly: `close, closes, closed, fix, fixes, fixed, resolve, resolves, resolved`. The `[:\s]+` between keyword and reference accepts both `Closes #42` and the conventional-commits `Closes: #42` form. The reference itself can be:

- `#N` — same-repo issue
- `owner/repo#N` — cross-repo close
- `https://github.com/owner/repo/issues/N` — full URL form (also recognized by GitHub's autolinker)

Plain-prose `#N` references without a closing keyword do NOT count — GitHub only auto-links via closing keywords, so the convention check has to match GitHub's own definition.

If a match is found, proceed to Phase 2.

If no match is found, STOP and `AskUserQuestion` with these options:

- **Create an issue retroactively and link it** (default) — invoke `/gh-issue-create`. The sub-skill scans the recent conversation for a user-raised ask; if it can't find one (likely, since the conversation context here is just the feedback-work invocation), it will prompt for the issue subject. Paste the PR title + body when it prompts. After the issue is posted, PATCH the PR body to add `Closes #<n>` to the Summary section. Then proceed to Phase 2.
- **Proceed without an issue (one-off override)** — record the decision in the Phase 7 report; the rule #9 violation remains on the PR.
- **Cancel** — stop the feedback-work flow.

The default is always **Create retroactively**, regardless of how full the PR body is. An empty-body PR signals "this PR hasn't been described yet" — which is a separate problem but does not change the answer to "should this PR be linked to an issue?"; the answer is still yes.

## Phase 2: Classify

For each post-last-commit feedback item, pick exactly one label:

| Label | Meaning | Action |
|---|---|---|
| **actionable** | asks for a concrete code change | implement in Phase 3 |
| **in-body** | question answerable by clarifying the PR body | answer in Phase 6 rewrite |
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

## Phase 4: Self-review the fixes before pushing

Invoke `/gh-self-review` **once** against the post-fix tree. The inner skill detects its own scope — normally committed-only at this point (Phase 3 commits each addressed item), but if uncommitted or untracked files remain it will surface them. The inner skill owns the loop: it spawns a fresh sub-agent per iteration (the dev addresses items between iterations; the skill re-audits), applies the rigor checklist, and exits on its own stop conditions (empty triage, all remaining items deferred by the dev, or 3 iterations exhausted).

**Why this phase exists**: addressing review feedback can introduce regressions in code paths the feedback's tests don't exercise. The reviewer caught the original issue; their tests covered that. Your fix's tests cover the fix. Neither catches "did this fix break adjacent behavior?". A fresh sub-agent reading the post-fix diff catches that locally — saves a reviewer round-trip and avoids compounding regressions across review rounds.

**Push is gated on a clean exit from `/gh-self-review`.** Concretely:

- **Triage empty** → proceed to Phase 5.
- **Blockers addressed across iterations** → the dev fixed items between fresh-sub-agent passes; the inner skill re-audited until clean. Proceed to Phase 5.
- **Surviving items after 3 iterations** → the inner skill surfaces these per its own accept / defer / escalate exit. Resolve there. Proceed to Phase 5 only after the dev explicitly accepts the remaining items (folded into the PR body's "Known non-blockers" section per Phase 6) or escalates.
- **None of the above** → do not push. Return to Phase 3.

This skill does NOT wrap `/gh-self-review` in a second outer loop — that would push past the inner cap and duplicate logic the inner skill already owns. Cap ownership lives in `/gh-self-review`; this phase is a single delegated call.

**Composition**: this phase delegates entirely to `/gh-self-review` — do not re-implement scope detection, sub-agent spawning, the rigor checklist, or the iteration cap here. The wrapping skill's job is to insert the call at the right point in the feedback-work flow; the review work itself lives in its own skill.

## Phase 5: Push

```
git push
```

The branch is already tracking upstream from `gh pr checkout`; no `-u` needed.

## Phase 6: Rewrite PR body in place (not via comments)

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
- **Cross-repo refs** in the rewritten body default to explicit markdown links — bare `<owner>/<repo>#<n>` shortcuts are unsafe whenever the org slug contains another repo name in the same org as a substring. GitHub's autolinker mangles those org-slug fragments by relinking the inner repo name (e.g. `idnerdidx/bulk#311` smears into nested autolinks via the `idx` substring). If the existing body still has bare cross-repo shortcuts, rewrite them to `[<repo> PR #<n>](https://github.com/<owner>/<repo>/pull/<n>)` form during this pass, keeping the `<owner>/<repo>` pattern out of the label. Same-repo `#<n>` references stay valid bare. See the cross-repo-refs convention in `flows/github/gh`.

Write via the REST API — `gh pr edit` can surface the Projects-classic GraphQL deprecation as a failure on some repos even when the PATCH succeeds:

```
gh api --method PATCH repos/<owner>/<repo>/pulls/<pr> \
  -f body="$(cat /tmp/pr-body-new.md)" \
  --jq .html_url
```

## Phase 7: Report back

Print, in the terminal (not to GitHub):
- PR URL on its own line.
- A one-line summary of which feedback items were addressed and in which commits.
- If any items were **out-of-scope**, list them and offer to run `/gh-issue-create` to capture each one.

## Guardrails

- Never call `gh api .../comments` with `--method POST` from this skill. No exceptions.
- Never post on the user's GitHub account without the attribution footer. (This skill doesn't post at all, but the rule stands for any future skill that borrows this workflow.)
- If the user explicitly asks for a comment reply after the skill finishes, PRINT the intended text (including the footer) and let the user paste it themselves.
- Don't resolve review conversations — only the reviewer can do that meaningfully. Pushing fixes + rewriting the body is enough signal.
- Don't leave bare `<owner>/<repo>#<n>` cross-repo shortcuts in the rewritten PR body when the org slug contains another repo name in the same org as a substring. Replace with `[<repo> PR #<n>](https://github.com/<owner>/<repo>/pull/<n>)`, keeping the `<owner>/<repo>` pattern out of the label, or GitHub's autolinker will mangle the render. Same-repo `#<n>` is unaffected.
- Don't force-push, don't rebase, don't skip hooks.
- If classification is uncertain or feedback is contradictory, ASK the user before implementing.
