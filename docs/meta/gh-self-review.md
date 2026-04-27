---
description: Deep self-audit of pending local changes (committed + uncommitted) under truthseeker rigor before commit/push/PR. Local-only — produces a triage block of blockers and non-blockers. Does not write code, does not post anywhere.
category: meta
triggers:
  - gh-self-review
  - self-review
  - self review
  - review my changes
  - review my diff
  - audit my changes
  - audit my diff
  - preflight
  - check before pr
when: User wants a deep audit of their own working-tree + branch changes before committing, pushing, or opening a PR — same rigor a reviewer would apply, applied early so the mechanical issues get fixed before they reach the reviewer.
related:
  - meta/gh
  - meta/gh-issue-work
  - meta/truthseeker
---

# /gh-self-review — Pre-flight self-audit

The same rigor `/gh-pr` applies to a posted PR, applied to local changes that haven't been committed, pushed, or PR'd. Catches mechanical issues a reviewer would otherwise raise — missing tests, unverified claims, fragility, scope drift — so the human reviewer's time is spent on design, not cleanup.

## Principles

- **Prove before flagging.** A blocker without evidence is noise. Cite file:line, the caller, the missing test.
- **Reject fragility.** Hunt for setups that require multiple things to go right.
- **Make invisible visible.** "Fixes X" / "improves Y" claims need a test, benchmark, or trace.
- **Output is a triage list, not a fix.** Don't edit code. The dev decides what to change.

## Limits

- Self-audits share the author's blind spots; not a substitute for a fresh reviewer on design.
- Doesn't run tests or builds.
- Doesn't post comments, create issues, or push.

## Phase 1: Resolve scope

Detect base (upstream tracking branch → `origin/HEAD` → `main`):

```
upstream=$(git rev-parse --abbrev-ref --symbolic-full-name @{u} 2>/dev/null)
base=$(echo "$upstream" | sed 's|^origin/||')
base=${base:-$(git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed 's|^refs/remotes/origin/||')}
base=${base:-main}
git rev-parse --verify "$base" || git fetch origin "$base":"$base"
```

Three buckets:
- **Committed**: `git diff --name-only "$base"...HEAD`
- **Modified**: `git diff --name-only HEAD`
- **Untracked**: `git ls-files --others --exclude-standard`

Stop with "nothing to audit" if all three are empty, or if not in a git repo.

**Auto-include `committed`** — that's the certain scope.

**Confirm `modified` + `untracked` with the dev** when either is non-empty. First, fetch the linked issue if any (`#N` from branch name or commit messages → `gh api repos/<owner>/<repo>/issues/<n> --jq '{title, body}'`; skip silently if unavailable). Then for each ambiguous file give a one-line reason ("matches keyword 'X' from issue", "no obvious link") and ask via `AskUserQuestion`:

- **All of them** — include in audit.
- **None — committed only** — treat as leftover.
- **Pick per file** — yes/no for each.
- **Cancel** — stop.

Default to **Pick per file** unless every file's path matches an issue / branch keyword.

The result is the **in-scope set** for subsequent phases.

## Phase 2: Gather diff

```
git diff "$base"...HEAD                       # committed scope
git log "$base"..HEAD --pretty=format:'%h %s'  # commit messages for intent
git diff HEAD -- <path>                       # for each confirmed-modified file
```

For confirmed-untracked files: `git diff` doesn't show their content. Read each via the `Read` tool or `cat`. Don't `git add -N` — that mutates the index and violates the no-side-effects guardrail.

If the in-scope diff exceeds ~2000 lines, prioritize files in the change's stated scope and say so in the report.

## Phase 3: Verify the change's claims

The dev's intent is in the branch name, commit messages, and the linked issue (if fetched in Phase 1). For each claim:

- **"Fixes #N"** — does the diff satisfy the issue's acceptance criteria, or just adjacent?
- **Performance claims** — benchmark, repro, telemetry, or code-level explanation? Unverified perf is a blocker.
- **Bug-fix claims** — regression test? If not, default position is blocker.
- **"Works on all platforms"** — unless the diff or CI shows it, it's a claim, not a fact.

## Phase 4: Analyze the diff

For diffs >500 lines, prioritize files in the stated scope and say so.

### Correctness
Invariants preserved? Callers checked (`grep -rn`)? Failure modes — silent corruption is worse than a crash. Errors returned with context or silently swallowed? Nil derefs, off-by-one, leaks, context cancellation gaps, goroutine leaks, races.

### Fragility
- Package-level globals not keyed by instance.
- Ordering dependencies that nothing enforces.
- Dead code / vestigial config — if removing it doesn't break anything, it shouldn't exist.
- "Works on my machine" — uncommitted fixtures, hard-coded paths, defaulted env vars.
- Silent fallbacks — catch-all `except`, `|| true`, swallowed errors.

### Tests
Tests are evidence. Missing tests on hot-path / cache / concurrency / bug-fix changes are usually blockers.

### Security
Auth bypass, credential exposure, injection, unsafe file ops. Never quote secrets inline; describe the class.

### Scope discipline
Files outside the stated scope — why? Compare to the linked issue body when available. Commented-out code, TODOs, debug prints, formatting noise — flag.

### Conventions
Read the repo's `CLAUDE.md` and `.claude/rules/*.md` if present. Search detritus KB with `kb_search` for relevant patterns. Grep sibling files before asserting "non-conventional" — `grep -rn` is evidence; "I think Go usually..." is not.

## Phase 5: Output triage block

```
## Self-review on <branch> vs. <base>
<N> files audited (<C> committed + <M> uncommitted-included + <U> untracked-included; <X> excluded as leftover).

### Blockers — fix before commit
- **<title>** — <file:line>. <Evidence>. Fix: <what would unblock>.

### Non-blockers — fix in this change
- **<title>** — <file:line>. <Evidence>. Why now: <reason>.

### Non-blockers — separate issue candidates
- **<title>** — <file:line>. <Evidence>. Why deferred: <reason>.
```

Omit any section that's empty. Don't pad. Then ask the dev to triage each item: fix-now, separate-issue, or dismiss-with-reason.

## Guardrails

- Never edit code, stage files, commit, push, or post.
- Never flag without evidence — cite file:line, caller, or missing test, or drop it.
- Never pad with filler. "Nothing to flag" is a valid finding.
- Never substitute "looks good" for verification.
- Never quote secrets inline.
- A green self-review is not a substitute for a real reviewer. Say so once if the diff is non-trivial.
