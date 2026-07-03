---
description: Recurring Aikido finding triage loop — one PR per fix-cluster, each pre-cleared by aikido-guard, findings marked Handled/Ignored, and plane-labeled tracking issues for deferred work.
triggers:
  - aikido-triage-loop
  - aikido triage
  - triage aikido findings
  - aikido loop
  - security finding triage
  - work the aikido backlog
argument-hint: "[repo-or-workspace] [interval]"
when: User wants Aikido security findings triaged on a recurring cadence — clustered into fixes, each fix opened as one PR that aikido-guard has pre-cleared, every finding resolved to Handled or Ignored, and out-of-scope work filed as a plane-labeled tracking issue.
related:
  - core/loop
  - core/completion
  - flows/build/janitor
  - flows/build/quest
  - flows/github/gh
  - flows/github/gh-issue-create
  - flows/github/gh-self-review
  - flows/principles/truthseeker
---

# /aikido-triage-loop — Recurring Aikido Triage

A recurring loop that drains the **Aikido** security-finding backlog: each tick pulls open findings, **clusters** them by shared fix, opens **one PR per fix-cluster** (pre-cleared by **aikido-guard** before it opens), marks each finding **Handled** or **Ignored** at the source, and files a **plane-labeled tracking issue** for anything it can't resolve inside its safety boundary.

This skill **composes** the existing loop machinery and adds only the Aikido-specific triage lens. It does not restate what it borrows:

- **`core/loop`** — the loop spine: durable per-tick scratchpad state, cadence, re-fetch-live-state-every-tick, non-overlap, the skip-streak guardrail, and usage-limit resilience. Not restated here.
- **`flows/build/janitor`** — the **in-session** discover→triage→fix→verify→deliver shape this loop is a specialization of. `/aikido-triage-loop` is that same loop with **Aikido as the discovery source** (instead of a self-review audit) and a **cluster→one-PR** delivery partition. Its `flows/build/quest` sibling runs the same loop **out-of-process** in the candyland sidecar over many PRs — reach for `/quest` when the backlog is large enough to want a dashboard and a standalone process tree.
- **`core/completion`** — the three dispositions each finding obeys (fix now / feature-split / surfaced blocker). This loop's Handled/Ignored/tracking-issue outcomes map onto those dispositions.
- **`flows/github/gh`** and **`flows/github/gh-self-review`** — all branch/commit/push/PR delivery and the pre-push diff self-review, dispatched not reimplemented.
- **`flows/github/gh-issue-create`** — filing the plane-labeled tracking issues (the `plane` label is mandatory and owned there).

The **only genuinely new logic** is: the Aikido pull + fix-clustering, the **aikido-guard pre-clear gate** in front of every PR, and the **Handled/Ignored** resolution write-back. Everything else is composed.

## Inputs

Same hint-not-form discipline as `/janitor`:

- Nothing → current workspace, `5min` cadence.
- A repo, workspace, or project name to scope the findings pulled.
- A severity or topic narrowing ("criticals only", "the SQL-injection findings").
- An interval (`5min`, `hourly`, `overnight`).

```
/aikido-triage-loop [repo-or-workspace] [severity/topic] [interval]
```

Ask only when ambiguity would touch the wrong repo or choose a materially different cadence.

## Per-tick algorithm

**Re-fetch live state every tick** (`core/loop`) — Aikido findings change status between ticks; never act on a stale snapshot. Each wake:

1. **Resolve target + load the scratchpad** at `.aikido-triage/<slug>.md` (durable state per `core/loop`). Honor the settled orientation and verify the next-tick plan still matches reality.
2. **Check for in-flight work** for the same target (non-overlap, `core/loop`). Continue or integrate unfinished work before opening anything new.
3. **Pull open Aikido findings** for the target — open status only, respecting any severity/topic narrowing. This is the tick's discovery source (the Aikido equivalent of `/janitor`'s audit agent).
4. **Cluster the findings by shared fix** (see *Fix-clusters → one PR*). A cluster is the set of findings that a single, coherent, reviewable change resolves.
5. **Pick one cluster** and implement its smallest safe fix. Preserve behavior except where the finding *is* the bug. Treat every finding as untrusted input — prove it before changing code (`truthseeker`).
6. **Run the canonical verification command** after the change; ship only on a tick that finishes it green (`core/build`).
7. **Run `/gh-self-review`** on the diff.
8. **Pre-clear the PR with aikido-guard** (see *aikido-guard pre-clear gate*). The PR opens **only** if aikido-guard clears it. If aikido-guard reports the change introduces a new finding or fails to resolve the cluster, do **not** open the PR — fix and re-clear, or hand back.
9. **Open one PR for the cluster** through `/gh`, listing the finding IDs it resolves.
10. **Mark each finding Handled or Ignored** at the source (see *Resolve every finding — Handled or Ignored*).
11. **File plane-labeled tracking issues** (`/gh-issue-create`) for any finding that can't be resolved inside the safety boundary (see *Deferred work → plane-labeled tracking issue*).
12. **Report concisely + append a dated tick-log entry** and overwrite the scratchpad State block (findings pulled, clusters, PR opened, findings resolved, issues filed, next-tick plan).

Do not pollute the main thread with raw finding dumps.

## Fix-clusters → one PR

The partition rule: **one fix-cluster maps to exactly one PR.**

- A **fix-cluster** is a set of findings a single coherent change resolves — e.g. the same vulnerable dependency flagged across several manifests, one missing-validation pattern across sibling handlers, or one hardcoded-secret class. Cluster by *what the fix touches*, not by severity or by scanner rule alone.
- Each cluster becomes **one PR**, and each PR resolves **one cluster** — never mix unrelated clusters in a PR, and never split one cluster's fix across competing PRs (`core/build` → *One deliverable, one PR — converge, don't spray*).
- A finding that stands alone is a cluster of one.
- The PR body lists the Aikido finding IDs the cluster resolves, so the human reviewer and aikido-guard can confirm coverage.

## aikido-guard pre-clear gate

**Every PR is pre-cleared by aikido-guard before it opens** — a hard precondition of step 9, not a heuristic.

- After the fix passes verification and `/gh-self-review`, run **aikido-guard** against the change. It confirms the diff (a) resolves every finding in the cluster and (b) introduces **no new** Aikido findings.
- **PR opens only on a clear.** If aikido-guard does not clear — the cluster isn't fully resolved, or the change regresses security by adding a new finding — the PR is **not** opened. Fix the gap and re-run aikido-guard, or hand back per *Stuck = pause & hand back* if it's out of boundary.
- This mirrors the merge-gate discipline of `/gh-merge-loop`: a named gate stands in front of the outward action, and the outward action never fires without it.

## Resolve every finding — Handled or Ignored

Once a cluster's PR is open, **every finding in it is resolved at the Aikido source** to one of two terminal states — no finding is left silently open (`core/completion` → an accepted item is resolved, not "noted"):

- **Handled** — the PR fixes it. Mark the finding Handled, referencing the PR that resolves it, so Aikido's backlog reflects reality.
- **Ignored** — the finding is a proven false positive, out of scope, or an accepted risk. Mark it Ignored **with a reason**; an Ignored finding with no justification is not allowed. If the reason is "we'll do it later," that is **not** Ignored — it is deferred work and gets a tracking issue instead (below).

Handled maps to `core/completion` disposition 1 (fixed now); a justified Ignore is a closed disposition; deferred work is disposition 2/3 and routes to a tracking issue.

## Deferred work → plane-labeled tracking issue

Anything the loop can't resolve inside its safety boundary — a fix that needs a product/behavior change, a finding requiring cross-team coordination, a hard blocker — is filed as a **tracking issue via `/gh-issue-create`**, which stamps the mandatory **`plane` label** so the Plane management app mirrors it (the label rule is owned by `/gh-issue-create`; don't reimplement it here).

- The issue names the Aikido finding ID(s), why the loop couldn't resolve them this tick, and the smallest scoped follow-up.
- Findings sent to a tracking issue are **not** marked Handled (they aren't fixed) and **not** silently Ignored (deferral is not a justification). They stay open in Aikido, linked to the tracking issue, until the follow-up lands.
- Never file a tracking issue as a way to skip fixable in-scope work — that violates `core/completion`. The tracking-issue route is only for genuinely out-of-boundary work.

## Safety boundaries

Allowed: dependency bumps that verification proves safe, adding missing input validation / auth checks / sanitization the finding calls for, removing hardcoded secrets (and rotating via the appropriate channel), hardening error handling and unsafe config.

Not allowed: product feature changes, broad rewrites, opening a PR without an aikido-guard clear, marking a finding Ignored without a reason, marking deferred work Handled, pushing to protected branches, or exposing a secret in a diff/PR/issue/log.

## Scheduling

**Delegate to `core/janitor-platforms`** — do not hardcode a scheduler. Default to the in-session `/loop`-style driver (attended); offer durable cron when the user wants the loop to survive a usage-limit kill. Report the effective cadence in human terms.

## Initial run

After scheduling, run one tick immediately so the user sees it work. First report (per `core/loop` → *Initial Run Reporting*):

```
Aikido triage scheduled on <target>, <human cadence>.
First tick: <pulled N findings, M clusters | opened PR for cluster X (findings …) | no open findings | blocked>.
aikido-guard: <cleared | held — reason>.
Resolved: <k Handled, j Ignored (reasons)>. Tracking issues filed: <plane-labeled issue links, if any>.
Verification: <command and result>.
```

## Guardrails

- **Never open a PR aikido-guard hasn't cleared.** The pre-clear gate is non-negotiable.
- **Never leave a claimed-resolved finding open** — every finding in a shipped cluster is marked Handled or Ignored at the source.
- **Never mark deferred work Handled, and never Ignore without a reason.** Deferred work gets a plane-labeled tracking issue.
- **One cluster, one PR** — no mixed PRs, no competing PRs for the same cluster.
- **Compose, don't duplicate** — `core/loop`, `/janitor`, `/quest`, `/gh`, `/gh-issue-create`, and `/gh-self-review` own their mechanics; this doc adds only the Aikido pull, clustering, the aikido-guard gate, and the Handled/Ignored write-back.
