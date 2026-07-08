---
description: Hard-review a posted GitHub PR with the same truthseeker rigor as /gh-pr and auto-post an APPROVE or REQUEST_CHANGES review, but never mutate the working tree of the clone it runs from. Reads via `gh api` or read-only git and isolates any build or test in a throwaway detached worktree; use it instead of /gh-pr when reviewing from a clone you actively work in.
triggers:
  - gh-pr-safe
  - safe pr review
  - workspace-safe review
  - review pr without touching workspace
  - read-only pr review
when: User wants a /gh-pr review from a clone they are actively working in, where a branch switch or a regenerated build cache would disrupt in-progress work. Same inputs as /gh-pr — a full PR URL, `<owner>/<repo>#<n>`, or bare `#<n>` when cwd is a clone of the target repo.
related:
  - flows/github/gh-pr
  - flows/github/gh
  - core/review-rigor
  - flows/principles/truthseeker
---

# /gh-pr-safe — /gh-pr that never touches the working tree

This is `/gh-pr` plus one guarantee: the clone it runs from is left **exactly as it was found**. Identical review rigor, identical auto-post verdict, zero mutation of a working tree you may be editing concurrently. Use it instead of `/gh-pr` on a machine where you actively work in the clones you review from — `/gh-pr` itself may `git checkout` the repo's default branch on the way out, which moves your HEAD and can rewrite generated caches mid-session.

## Step 1 — run the full /gh-pr flow

Call `kb_get` with `name="flows/github/gh-pr"` and follow it **end to end**: resolve target, fetch metadata + short-circuit checks, gather context, timeline cross-check + prior-signal inventory, apply `core/review-rigor`, compose the terse review, and post the `APPROVE` / `REQUEST_CHANGES` / `COMMENT` review pinned to `head_sha` with the attribution footer. Everything in that document applies unchanged **except** where an override below contradicts it.

## Step 2 — workspace-safety overrides (these win on any conflict)

1. **Never mutate the working tree.** No `git checkout` / `git switch` / `git reset` in the clone; never leave it on a branch other than the one you found; never regenerate its build caches.

2. **Read-only by default — this covers the whole review in the common case.** Get the PR's content from `gh api` (the gather-context calls: `/pulls/<n>/files`, the diff endpoint, `contents/<path>?ref=<sha>`) or read-only git (`git -C <clone> show <sha>:<path>`, `git -C <clone> grep <sha>`). The APPROVE-checklist "search for callers of changed functions (if the repo is local)" is done this read-only way — never by checking out the clone.

3. **Local build/test → detached worktree in a temp dir, never the clone.** Only when the review genuinely needs a checked-out tree (a build or test run):
   - `git -C <clone> worktree add --detach <tmp>/pr-<n> <head_sha>`
   - run the build/test inside `<tmp>/pr-<n>`
   - clean up on **every** exit path — clean finish, abort, error (build broken → COMMENT, repo unreachable, auth failure), or a `422` re-run: `git -C <clone> worktree remove --force <tmp>/pr-<n> 2>/dev/null; git -C <clone> worktree prune`

   The worktree reuses the clone's object store (fast) and touches only throwaway metadata under `.git/worktrees`, leaving the user's branch, source, and caches untouched. It is created **at most once** per invocation on a fixed `pr-<n>` path, so it cannot accumulate into a forest.

4. **Report phase — no checkout on the way out.** Do **not** run `git checkout <default_branch>` to "return to default." Because Step 2 never switched the clone's branch, there is nothing to return — the clone is already on whatever branch the user left it on, and that checkout would itself be the mutation this skill forbids. Print the review URL on its own line + one sentence naming the verdict, then run the worktree cleanup from override 3 if you created one.

5. **Track the cleanup.** Add an explicit "remove worktree (if created)" item to the progress list so it runs even if the review short-circuits.

## Why this is a separate skill, not a change to /gh-pr

`/gh-pr`'s default-branch checkout is harmless when it runs from a throwaway clone or outside any repo, and some workflows rely on being left on a clean default branch. Forcing read-only-everywhere onto the shared skill would impose one workflow's preference on everyone. Keeping the safety behavior as an opt-in sibling lets you choose it per-invocation without changing `/gh-pr` for anyone else.

## Guardrails

- Inherits every guardrail from `flows/github/gh-pr` (decide APPROVE vs REQUEST_CHANGES, never approve a draft/closed/self-authored PR, evidence-or-drop, attribution footer, etc.).
- Never mutate the clone's working tree — read via `gh api` / read-only git; isolate any build or test in a detached worktree you remove on every exit path.
- Never leave a created worktree behind — cleanup is not optional, and runs on abort and error paths, not just the happy path.
- **Incident hook.** A self-acknowledged mistake/doctrine violation ("you are right, I …", "I didn't follow …", "I ignored /…") or a blocker surfacing on a PR this flow authored is an incident — detect and route per `core/ego` (→ `/grow` / `/absorb`), after finishing the deliverable.
