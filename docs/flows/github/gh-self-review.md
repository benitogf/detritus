---
description: Deep self-audit of pending local changes (committed + uncommitted) under truthseeker rigor before commit/push/PR. Hands pointers to the change (repo, base, head SHA, scope) to a review sub-agent that pulls the diff live — fresh on the first pass so the audit is uncontaminated by the conversation that produced it, continued on re-review iterations. Local-only — produces a triage block of blockers and non-blockers. Does not write code, does not post anywhere.
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
  - flows/github/gh
  - flows/github/gh-issue-work
  - flows/github/gh-pr
  - core/review-rigor
  - flows/principles/truthseeker
---

# /gh-self-review — Pre-flight self-audit (delegated to a fresh agent)

The same rigor `/gh-pr` applies to a posted PR, applied to local changes that haven't been committed, pushed, or PR'd. The wrapping skill collects the scope pointers and the intent; the actual review work is **delegated to a sub-agent via the `Agent` tool** — spawned fresh on the first pass so the audit runs without the conversational context that produced the code, and continued on later iterations per `core/review-rigor` → *Re-review continuity*. The author's blind spots stay with the author; the sub-agent pulls the diff live and sees only it and the stated intent.

The analysis itself — principles, claim verification, second-pass checklist, correctness / fragility / performance / tests / security / scope / conventions subsections, plus the engine/language-specific review reference (loaded via `kb_get` when the diff matches) — lives in `core/review-rigor` and is shared verbatim with `/gh-pr`. The sub-agent loads it via `kb_get` and applies it end-to-end.

## Limits

- Self-audits still share the author's blind spots about the *design* — the sub-agent reviews mechanics, not architecture choices.
- Doesn't run tests or builds. The sub-agent reasons over the diff and the surrounding source; it doesn't compile or execute.
- Doesn't post comments, create issues, or push. Output is a triage list the dev acts on.

## Not a substitute for a posted review — reconcile it first

If the changes belong to an **open PR that already carries an unaddressed `CHANGES_REQUESTED` review**, this is the wrong skill: route to `gh-feedback-work` and fix what the reviewer named. A fresh self-review that returns *clean* while a posted review says *blocked* is incoherent — the self-review shares the author's blind spot and will happily miss the very finding the reviewer already proved. Before spawning the audit, fetch posted reviews (`gh api repos/<owner>/<repo>/pulls/<n>/reviews`); a self-review may NOT clear a finding a reviewer has posted on the same head — that finding stands until it is fixed, not until a self-audit fails to re-find it.

## Internal-only — never post the triage

The triage produced by this skill (and by `gh-issue-work` Phase 8a, which embeds it) is **for the author's eyes**. It must never be posted to the PR or issue as a comment or review. This is the contract that separates self-review from `/gh-pr`:

| Skill | Reviewer | Subject | Output destination |
|---|---|---|---|
| `/gh-self-review` (and `gh-issue-work` 8a) | You, on your own diff | Pre-merge mechanics | **Local triage → drives your own edits.** Never posted. |
| `/gh-pr` | You, on someone else's PR | Settled PR | **`gh api ... /reviews` (APPROVE or REQUEST_CHANGES).** Posted. |

Posting your own self-review to your own PR pollutes the review surface with author noise that real reviewers must filter past, and it conflates self-audit (drives fixes before the PR settles) with peer-review (the verdict on a settled PR). If the triage surfaces a **genuinely out-of-scope** item a reviewer should know about, fold it into the PR body as a "Known non-blockers" section (and file a tracked issue for it, labeled `icebox` as an out-of-scope finding parked mid-work — `core/icebox`) — do not post it as a comment / review. This is only for items out of scope for the change; anything you can fix in-scope must be fixed before opening, not parked here (see *Phase 5* stop conditions).

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

## Phase 2: Gather scope pointers + intent signals

Collect what the sub-agent needs — it has no conversation context, but it DOES have the repo. The brief carries **pointers and intent, never the diff body** — the sub-agent pulls the diff live per `core/review-rigor` → *Consume the diff from live git*; do not dump it to a scratch file and do not paste it into the brief.

```
git diff "$base"...HEAD --stat                   # shape + size (wrapper sizes the review; body stays unread)
git log "$base"..HEAD --pretty=format:'%h %s%n%b%n---'  # commit messages WITH bodies
git status --porcelain                           # modified/untracked candidates for scope confirmation
git rev-parse --abbrev-ref HEAD && git rev-parse HEAD   # branch name (intent signal) + head SHA (tree pin)
```

Untracked in-scope files are listed by path — the sub-agent reads their contents from the tree itself. Don't `git add -N` — that mutates the index and violates the no-side-effects guardrail.

If a linked issue was fetched in Phase 1, capture its body too — the sub-agent will check the diff against its acceptance criteria.

If the `--stat` exceeds ~2000 changed lines, the brief should say so explicitly and instruct the sub-agent to prioritize files matching the change's stated scope.

**Driving intent (required brief slot).** Resolve the ask that drove this change, in order: the `.plan/<slug>.md` contract when one drove the work (/forge delivery passes it explicitly) → the linked issue body → the stated ask from the conversation, condensed faithfully. Include it in the brief verbatim (or the contract's feature spec + acceptance checklist for long contracts). When genuinely nothing recorded the intent (ad-hoc tree changes), say so in the brief — the sub-agent notes the absence and reviews mechanics only. The existing signals (branch name, commit messages, linked issue) stay as corroboration, not as the intent itself.

## Phase 3: Hand off to a fresh sub-agent

Spawn the review via the `Agent` tool. The sub-agent runs with **no prior context** — the prompt is everything it sees. Use `subagent_type: "detritus-reviewer"` — the role definition `detritus --setup` installs at `~/.claude/agents/detritus-reviewer.md`, which pins the reviewer's model and effort per role (`roles/reviewer` → *Model and effort*). If the definition is absent (setup not run), fall back to `subagent_type: "general-purpose"` so the review still runs — it then inherits the session model. If the reviewer spawn returns or dies with a banner naming its pinned model's limit (not an account-wide limit), re-spawn it per `roles/reviewer` → *Model and effort* (In-session model-limit fallback).

The prompt is built from this template — fill in `<...>` placeholders from Phases 1-2:

```
You are reviewing a developer's pending local changes before they commit/push/PR. The change described below is the only thing you know about — there is no prior conversation. Produce the audit a real reviewer would give them, applied to their own diff, so mechanical issues get fixed before another human looks at it.

## Step 1: Load the rigor checklist
Call kb_get(name="roles/reviewer") and kb_get(name="core/review-rigor") and follow them end-to-end against the change described below (pull it live per "The change (pull it live)"). Together they cover: truthseeker principles, claim verification, scope classification, the **mandatory diff↔repo checks R1–R11**, the "don't stop at easy findings" second pass, correctness / fragility / performance / tests / security / scope-discipline / conventions checklists, the engine/language-specific review reference (loaded via kb_get when the diff matches), and large-diff handling. Skip nothing. If a subsection's scope didn't fire (e.g., no matching engine/language files in the diff), note it didn't apply rather than silently dropping it.

**Resolve every applicable R-check explicitly and report the ledger.** R1–R11 are the checks that reason about the *relationship* between the diff and the rest of the repo, which is precisely what an author-side review is worst at seeing — so name each one's outcome (`pass` / `blocker` / `n/a`, per the rigor doc's closed enum) rather than letting the narrative checklists stand in for them. An enumerated brief is read as the definition of the job: a check absent from the report was not silently fine, it was **not run**, and a self-review that returns clean without the ledger cannot distinguish those two. The ledger is what makes a *sampled* review visibly different from a complete one (`core/review-rigor` → *Review stamps*, and R2b's coverage-ledger rule) — the defect that survives to a posted reviewer is routinely one an applicable check would have named, in a pass that never asked it (e.g. a claim of universal coverage cleared without R3, or a green suite credited for wiring no test ever executed).

## Step 2: Verify the change's stated intent
Check each claim against the diff per the rigor doc's "Verify the change's claims" section:

- Driving intent (what was asked; the diff must satisfy it — a missing, partial, or contradicted commitment is a Blocker): <driving intent from Phase 2, or "none recorded — review mechanics only">
- Branch name: <branch>
- Base branch: <base>
- Linked issue body: <issue body, or "none">
- Commit messages (with bodies):
<full git log output>

## Step 3: Output a triage block

Open the block with the provenance line from core/review-rigor → "Review stamps" so the reader knows which model produced this audit and at what effort (this flow spawns the pinned agent; it never posts, so no coverage-ledger requirement — the spawned reviewer returns it anyway via roles/reviewer's verdict contract):

`detritus <version> · detritus-reviewer (claude-fable-5 / high) · head <sha8>`

If the pinned definition was absent and the review ran on the `general-purpose` fallback (Phase 3), substitute the general-purpose variant from *Review stamps* — the model segment reads the session model and effort is `unpinned`. (This flow always spawns; it has no inline-session path.)

## Self-review on <branch> vs. <base>
<N> files audited (<C> committed + <M> uncommitted-included + <U> untracked-included; <X> excluded as leftover).

### Blockers — fix before commit
- **<title>** — <file:line>. <Evidence>. Fix: <what would unblock>.

### Non-blockers — fix in this change
- **<title>** — <file:line>. <Evidence>. Why now: <reason>.

### Non-blockers — separate issue candidates
- **<title>** — <file:line>. <Evidence>. Why deferred: <reason>.

Each candidate here that survives to a filed issue is an out-of-scope finding parked mid-work — filed as an icebox issue (`core/icebox`).

Omit any section that's empty. Don't pad. "Nothing to flag" is a valid finding.

## The change (pull it live)
Repo: <local repo path — grep callers, read surrounding source, run `kb_search` against the relevant patterns docs>
Base: <base>   Head SHA: <git rev-parse HEAD — verify it matches before reviewing; a moved tree invalidates the pass>
In scope: <committed file list from --stat> + uncommitted-included: <modified paths, review via `git diff HEAD -- <path>`> + untracked-included: <paths — read their contents from the tree>

Pull the diff yourself per core/review-rigor → "Consume the diff from live git": `git diff <base>...HEAD --stat` for the map, then per-file diffs for what you examine. Do not dump the diff to a file.

## Guardrails for your review
- Do NOT edit code, stage files, commit, push, or post anywhere. Output is text only.
- Do NOT flag anything without evidence — cite file:line, the caller, or the missing test. If you can't cite, drop it.
- Do NOT quote secrets inline even when flagging — describe the class.
- Do NOT skip subsections of the rigor doc. If a class didn't fire, say so explicitly.
- A green self-review is not a substitute for a real reviewer — note this once if the diff is non-trivial.
```

The wrapping skill passes pointers, not the diff — the sub-agent `cd`s into the repo path and pulls the diff live with `Bash` / `Read` (per-file diffs, callers, surrounding source, sibling conventions) — that's expected and required. What it cannot do is rely on context from this conversation.

## Phase 4: Present the sub-agent's triage to the dev

The sub-agent returns the triage block. Don't re-edit it; the freshness is the point. Print it as-is, then ask the dev to triage each item: fix-now, separate-issue, or dismiss-with-reason.

If the sub-agent returned with empty sections only ("nothing to flag"), report that — don't manufacture findings to fill space.

## Phase 5: Loop until clean

A single sub-agent pass can miss regressions a fix introduces. Fixing one blocker often perturbs adjacent code in ways the first review didn't flag — the canonical failure shape this loop exists to catch. After the dev addresses items from Phase 4, **re-run the review against the updated tree** by **continuing the same reviewer sub-agent** (`core/review-rigor` → *Re-review continuity*): send it a delta brief — which findings were addressed, the fix commits, the new head SHA — and it re-reviews per that section (fresh evidence, regression sweep of the fix commits). Phase 1 scope handling still re-runs in the wrapper (below).

Stop conditions:

- Sub-agent's triage is empty.
- The only remaining items are **genuinely out of scope** for this change — a separate feature, a repo-wide cleanup the change didn't touch. A finding you *can* address in-scope is **not** deferrable: fix it and re-loop. Do not converge with handle-able findings unaddressed, and do not park them in a "Known non-blockers" PR section to ship around them — that is the deferral `core/completion`'s exit gate forbids (disposition 1: handle-able in-scope work is done now), applied to your own delivery. Out-of-scope items that survive get a **tracked issue** (via `/gh-issue-create`), labeled `icebox` as an out-of-scope finding parked mid-work (`core/icebox`), not just a loose body line.
- Loop has run 3 iterations. A finding still standing after three review rounds — held open by the continued reviewer against three fix attempts — is not being resolved by iteration; surface the persistence to the dev for accept / defer / escalate.

The test for "can I defer this?" is **not** "is it a blocker?" — it is "is it out of scope for this change?" If you could fix it with the same tools in the same diff, it is in scope, and shipping it as a known non-blocker is a punt.

Iteration mechanics: the FIRST pass is a fresh `Agent` spawn (the Phase 3 freshness contract — independence from the authoring conversation). Iterations 2+ **continue that same sub-agent** with the delta brief; anchoring on its own prior conclusions is guarded by the rigor doc's verdict-integrity rules (held context is reused, held conclusions must be re-proven against the fix commits). If the sub-agent cannot be continued (context lost, agent expired), fall back to a fresh spawn with the full Phase 3 brief, per *Re-review continuity*'s fallback rule.

**Iteration 2+ scope handling.** Phase 1 re-runs to pick up new commits the dev made between iterations — committed scope evolves naturally. Only re-prompt the dev about modified/untracked files if the set *changed* since the prior iteration (new untracked files appeared, or files in the prior in-scope set are no longer in the working tree). If the modified/untracked set is unchanged, carry the prior iteration's in-scope decision forward silently — don't re-ask the same question.

**The verdict is bound to the tree it ran against.** A clean exit certifies *that exact tree* — nothing more. ANY mutation of the tree after the review reopens it, even when the loop had already exited clean: a refactor the review itself prompted (extracting a shared helper, a rename), a follow-up fix, `commit --amend`, a squash, a rebase/merge, conflict resolution. Each produces a tree no sub-agent has seen. The rule: **the tree that ships is the tree the last review ran on.** If you change anything between the clean review and the push/PR, run one more review pass against the final tree before shipping (continuing the same reviewer per Phase 5; fresh spawn if it can't be continued) — the 3-iteration cap counts review rounds, not a budget that excuses skipping the audit of what actually lands. A "no blockers" result from before a squash/refactor is stale the moment the tree moves.

## A blocker that survives this gate is a gate miss (incident)

Every detritus PR ships through this loop-until-clean gate. So a blocker that later surfaces on a PR this self-review passed *clean* — a reviewer `CHANGES_REQUESTED`, a `/gh-pr` finding, babysit-detected feedback — is by definition a **gate miss**: this audit was supposed to catch it and didn't. Per `core/ego` trigger ② it is an incident and fires immediately (routing to `/absorb`, whose fix leg closes the blocker first, so "immediate" never drops the deliverable). The lesson to distill is **"why did self-review miss this?"** — feed it back so the next pass catches the class.

## Why hand off to a fresh agent

Self-review by the author who wrote the change shares the author's blind spots — the same mental model that produced the bug fails to catch it. The conversation that led to the diff also carries justifications ("we agreed this was fine") that bias the audit toward acceptance. Delegating to a fresh sub-agent:

- The sub-agent never saw the conversation that produced the code.
- It loads `core/review-rigor` fresh and applies it without contamination.
- It has no investment in any decision encoded in the diff.

This doesn't eliminate the blind spot — both agents share training and reasoning patterns — but it eliminates the *conversational* bias, which is the largest source of self-review false negatives.

Freshness earns its cost exactly once: at the first pass, against the *author's* context. Re-review iterations verify fixes to findings the reviewer itself cited — there is no authoring bias to escape, so they continue the same reviewer (`core/review-rigor` → *Re-review continuity*) instead of paying a full context rebuild per round.

## Guardrails (for the wrapping skill)

- Never edit code, stage files, commit, push, or post.
- Never bypass the sub-agent — the wrapping skill collects scope and presents triage; it does NOT do the review itself.
- Never trim the sub-agent's findings. If it surfaced 12 blockers, all 12 reach the dev.
- Never add findings of your own to the sub-agent's output. If you noticed something during scope-detection, surface it separately as a wrapping-skill observation, clearly labeled.
- Never quote secrets inline (including in the brief passed to the sub-agent — describe the class).
