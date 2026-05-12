---
description: Deep self-audit of pending local changes (committed + uncommitted) under truthseeker rigor before commit/push/PR. Hands the diff to a fresh sub-agent so the review is uncontaminated by the conversation that produced it. Local-only — produces a triage block of blockers and non-blockers. Does not write code, does not post anywhere.
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
  - meta/gh-pr
  - meta/review-rigor
  - meta/truthseeker
---

# /gh-self-review — Pre-flight self-audit (delegated to a fresh agent)

The same rigor `/gh-pr` applies to a posted PR, applied to local changes that haven't been committed, pushed, or PR'd. The wrapping skill collects the scope and the diff; the actual review work is **delegated to a fresh sub-agent via the `Agent` tool** so the audit runs without the conversational context that produced the code. The author's blind spots stay with the author; the sub-agent sees only the diff and the stated intent.

The analysis itself — principles, claim verification, second-pass checklist, correctness / fragility / performance / tests / security / scope / conventions / Godot subsections — lives in `meta/review-rigor` and is shared verbatim with `/gh-pr`. The sub-agent loads it via `kb_get` and applies it end-to-end.

## Limits

- Self-audits still share the author's blind spots about the *design* — the sub-agent reviews mechanics, not architecture choices.
- Doesn't run tests or builds. The sub-agent reasons over the diff and the surrounding source; it doesn't compile or execute.
- Doesn't post comments, create issues, or push. Output is a triage list the dev acts on.

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

The result is the **in-scope set** passed to the sub-agent in Phase 3.

## Phase 2: Gather diff + intent signals

Collect everything the sub-agent will need — it has no conversation context, so the brief must be self-contained.

```
git diff "$base"...HEAD                          # committed scope
git log "$base"..HEAD --pretty=format:'%h %s%n%b%n---'  # commit messages WITH bodies
git diff HEAD -- <path>                          # per confirmed-modified file
git rev-parse --abbrev-ref HEAD                  # branch name (intent signal)
```

For confirmed-untracked files: `git diff` doesn't show their content. Capture each file's contents with `cat` for inclusion in the brief. Don't `git add -N` — that mutates the index and violates the no-side-effects guardrail.

If a linked issue was fetched in Phase 1, capture its body too — the sub-agent will check the diff against its acceptance criteria.

If the in-scope diff exceeds ~2000 lines, the brief should say so explicitly and instruct the sub-agent to prioritize files matching the change's stated scope.

## Phase 3: Hand off to a fresh sub-agent

Spawn the review via the `Agent` tool. The sub-agent runs with **no prior context** — the prompt is everything it sees. Use `subagent_type: "general-purpose"` so it has `kb_get`, `Bash`, `Read`, etc.

The prompt is built from this template — fill in `<...>` placeholders from Phases 1-2:

```
You are reviewing a developer's pending local changes before they commit/push/PR. The diff is the only thing you know about — there is no prior conversation. Produce the audit a real reviewer would give them, applied to their own diff, so mechanical issues get fixed before another human looks at it.

## Step 1: Load the rigor checklist
Call kb_get(name="meta/review-rigor") and follow it end-to-end against the diff below. The doc covers: truthseeker principles, claim verification, scope classification, the "don't stop at easy findings" second pass, correctness / fragility / performance / tests / security / scope-discipline / conventions checklists, the Godot subsection, and large-diff handling. Skip nothing. If a subsection's scope didn't fire (e.g., no Godot files in the diff), note it didn't apply rather than silently dropping it.

## Step 2: Verify the change's stated intent
Check each claim against the diff per the rigor doc's "Verify the change's claims" section:

- Branch name: <branch>
- Base branch: <base>
- Linked issue body: <issue body, or "none">
- Commit messages (with bodies):
<full git log output>

## Step 3: Output a triage block

## Self-review on <branch> vs. <base>
<N> files audited (<C> committed + <M> uncommitted-included + <U> untracked-included; <X> excluded as leftover).

### Blockers — fix before commit
- **<title>** — <file:line>. <Evidence>. Fix: <what would unblock>.

### Non-blockers — fix in this change
- **<title>** — <file:line>. <Evidence>. Why now: <reason>.

### Non-blockers — separate issue candidates
- **<title>** — <file:line>. <Evidence>. Why deferred: <reason>.

Omit any section that's empty. Don't pad. "Nothing to flag" is a valid finding.

## The diff
Repo: <local repo path, so you can grep callers, read surrounding source, run `kb_search` against the relevant patterns docs>

<paste the full unified diff here — `git diff <base>...HEAD`, plus `git diff HEAD -- <path>` for each confirmed-modified file, plus contents of each confirmed-untracked file with `=== untracked: <path> ===` headers>

## Guardrails for your review
- Do NOT edit code, stage files, commit, push, or post anywhere. Output is text only.
- Do NOT flag anything without evidence — cite file:line, the caller, or the missing test. If you can't cite, drop it.
- Do NOT quote secrets inline even when flagging — describe the class.
- Do NOT skip subsections of the rigor doc. If a class didn't fire, say so explicitly.
- A green self-review is not a substitute for a real reviewer — note this once if the diff is non-trivial.
```

The wrapping skill passes the diff inline. The sub-agent CAN `cd` into the repo path and use `Bash` / `Read` to follow up (grep callers, inspect surrounding source, read sibling files for conventions) — that's expected. What it cannot do is rely on context from this conversation.

## Phase 4: Present the sub-agent's triage to the dev

The sub-agent returns the triage block. Don't re-edit it; the freshness is the point. Print it as-is, then ask the dev to triage each item: fix-now, separate-issue, or dismiss-with-reason.

If the sub-agent returned with empty sections only ("nothing to flag"), report that — don't manufacture findings to fill space.

## Why hand off to a fresh agent

Self-review by the author who wrote the change shares the author's blind spots — the same mental model that produced the bug fails to catch it. The conversation that led to the diff also carries justifications ("we agreed this was fine") that bias the audit toward acceptance. Delegating to a fresh sub-agent:

- The sub-agent never saw the conversation that produced the code.
- It loads `meta/review-rigor` fresh and applies it without contamination.
- It has no investment in any decision encoded in the diff.

This doesn't eliminate the blind spot — both agents share training and reasoning patterns — but it eliminates the *conversational* bias, which is the largest source of self-review false negatives.

## Guardrails (for the wrapping skill)

- Never edit code, stage files, commit, push, or post.
- Never bypass the sub-agent — the wrapping skill collects scope and presents triage; it does NOT do the review itself.
- Never trim the sub-agent's findings. If it surfaced 12 blockers, all 12 reach the dev.
- Never add findings of your own to the sub-agent's output. If you noticed something during scope-detection, surface it separately as a wrapping-skill observation, clearly labeled.
- Never quote secrets inline (including in the brief passed to the sub-agent — describe the class).
