---
description: Shared planning core — what a settled plan is, how to know it's ready, and the .plan/<slug>.md contract that hands it to implementation. Do not invoke directly; composed by /plan (developer intake) and /dream (executive intake).
triggers:
  - planning core
  - settled plan
  - plan contract
  - acceptance criteria
  - readiness check
when: Internal. Loaded by /plan or /dream to define the settled-plan deliverable, the readiness/consistency checks, and the plan-contract artifact. Both wrappers add only their intake style and role.
related:
  - flows/plan/plan
  - core/dream
  - core/completion
  - flows/build/forge
  - flows/build/smith
  - core/todo-import
---

# Planning Core — the settled plan and its contract

`/plan` and `/dream` are two intakes over **one destination**: a settled, buildable, verifiable plan. This core holds what that plan *is*, how to know it's *ready*, and the artifact that hands it to implementation. The wrappers differ only in intake style and who owns technical decisions — `flows/plan/plan` (developer: open-ended, the user steers tech) and `core/dream` (executive: multiple-choice, the architect owns tech). Neither restates what's below.

> ## ⛔ Do not invoke directly
> No slash command. Loaded via `kb_get` by `/plan` or `/dream`.

## The deliverable — a settled plan

A settled plan captures, in plain language:

- **Feature spec** — what is being built and why.
- **Acceptance criteria** — a checklist, each item objectively verifiable by test, command output, UI check, or a documented manual check — and **safely agent-executable**: the delivering agent can run every criterion without mutating live user infrastructure (spawning real agent trees, sending external messages, touching production data). When only a live path proves the behavior, the plan names the harness (stub executor, create-then-cancel, dry-run flag). A criterion the agent must skip for safety is a planning defect.
- **Surroundings survey** — the existence check and composition map per the *Surroundings survey* section below: who already owns this responsibility, and which shared mechanisms the feature composes.
- **User-stated rules** — verbatim constraints from the conversation.
- **Decisions made on the user's behalf** — each non-trivial technical choice with a one-line why (the audit trail a reviewer needs in place of a conversation they didn't see).
- **Feature-splits & blockers** — genuinely separate features and hard blockers per `core/completion`'s three dispositions. This is **not** a parking lot for in-scope work, which is always built (disposition 1). Disposition is intake-specific: `/plan` records genuinely-separate features for the developer to triage; `/dream` (autonomous, non-technical path) resolves in-scope concerns into the plan and splits genuinely-separate features into their own plans rather than handing the user a note they can't action (`core/dream` → *Hazards — deal with them, never defer*).

## Surroundings survey

A plan is built *from* the codebase, not beside it. Before designing anything, the survey fixes its target, answers the existence question, and maps the composition:

- **Target surface first.** Before the existence search can mean anything, pin *which* codebase it runs against — the repo, project, and branch the change actually lands in and deploys from. Verify that target against **real evidence** (the deploy/run config, the entrypoint, the device/service that consumes the artifact, sibling code that ships there) — never fix it from the task's phrasing, a repo *name* that appears in the ticket, or a recalled memory. A memory note, a ticket that says "the X board" or names a module, or a familiar-looking sibling clone are all **hints to verify, never authority to build on** — a name is not a target. Two same-named projects in different repos, or a feature whose UI half and display half live in different repos, are the trap this catches: getting the target wrong means the entire plan — existence search, composition map, branch — is grounded in the wrong tree, and every later gate passes against code that will never run it. When the target is ambiguous or the evidence is thin, it is a **question to settle**, not an assumption to proceed on.
- **Source surface too, whenever the plan ports from a reference.** Adapting, porting, or "doing what X does" adds a **second** surface to verify, and getting it wrong fails the same way as a wrong target: every later gate passes against the wrong original. When several candidate references exist (per-customer skins, per-tenant forks, vendored copies, an old tree beside its replacement), **size and completeness are not evidence of canonical** — the largest, most-featured, or first-found copy is routinely the wrong one. Verify the source by the artifacts it **consumes**: the correct reference is the one wired to the same assets, config keys, schema, or vocabulary the *target* ships. Run the check in that direction — *what consumes the thing I am building against?* — because the decisive tell is cheap and available: **an artifact in the target with no consumer anywhere in the tree you are reading from means you are reading the wrong tree.** Cite the chosen reference by path and by the artifact link that selected it; near-duplicate filenames across trees (the same module name in a dozen places) make an uncited pick indistinguishable from a correct one. Where the source's own constants contradict its behaviour, **behaviour wins** — a stale path constant, an inverted comment, or a dead default is not the contract; the code that actually runs is.
- **Existence first.** In the verified target, does this responsibility already have an owner — built, shipped, or in flight? Search four sources, none optional: the **working tree**; **git history** (removed or renamed components pending restore); **open and merged PRs + issues**; **`.plan/` contracts**. A legitimate settled outcome is **no work** — the plan resolves to adopting the existing component, or closing the request as superseded.
- **Composition map.** For the feature that survives the existence check: name each cross-cutting mechanism it composes — service/server skeleton, auth/authorization gates, transport & state delivery (REST/WS), messaging & event-stream consumption (broker clients, consumer loops), persistence & rehydration, configuration surface, logging/audit & observability, CI & lint policy, docs home & conventions, deploy/gateway registration, frontend data access (shared client lib vs hand-rolled fetch/WS), dev harnesses & tooling — **each cited from a named sibling** ("service X does this via Y").
- **New mechanisms are decisions.** Every mechanism or convention the plan introduces carries a recorded decision naming the existing alternative and why it doesn't fit. A convention is repo-owned: a plan never establishes one unilaterally as a side effect of a feature.

Reinvention doesn't just duplicate — it re-opens bug classes the shared mechanism already paid for, and its internal test/review rigor cannot detect the misfit because the module is only ever reviewed against itself.

## Readiness check

A plan is ready only when:

- **Intent clarity** — desired outcome, audience, and scope boundary are clear.
- **Acceptance clarity** — every criterion is objective enough to verify.
- **Constraint capture** — user-stated rules, repo guidance, KB guidance, and known external constraints are captured as rules or decisions, not left as chat-only context.
- **One-feature boundary** — the spec fits one coherent feature; independent features split into separate plans.
- **Delivery packaging is independent** — every PR the plan emits passes the independence gate below; no criterion requires the agent to touch live infrastructure to verify (name the harness instead).
- **Target surface is verified** — the repo/project/branch the plan builds against is confirmed from real deploy/run evidence, not inferred from the task's wording or a recalled memory (per *Surroundings survey* → *Target surface first*). A plan grounded on an unverified target is **not ready**, however complete the rest looks.
- **Source surface is verified, when the plan ports from a reference** — the reference is cited by path and by the artifact link that selected it (it consumes what the target ships), not chosen for being the largest or first-found candidate (per *Surroundings survey* → *Source surface too*). A port grounded on an unverified reference is **not ready** — its geometry, vocabulary, and mappings all inherit from the wrong original.
- **Surroundings survey holds** — a plan that overlaps an existing component's responsibility, or introduces a parallel instance of an existing mechanism, without the corresponding recorded decision, is **not ready**.

## Delivery packaging — split only what is fully independent

A plan emits only PRs that are **independent and simultaneously launchable**: disjoint files, and no PR consumes another's changes. The same rule governs every split at every level (PRs here, coder tasks in `roles/tech-lead`):

- **Coupled work within a repo = ONE PR.** If two pieces touch the same files or one builds on the other's output, they are one unit.
- **Work that requires a merged predecessor belongs to a LATER planning round**, run after the merge — never a simultaneous launch with a prose sequencing note.
- **Cross-repo companions** (one feature spanning N repos) keep the merge-together pattern from `core/build` → *Multi-repo delivery* — that is one feature with one PR per repo, not a dependency chain.

✅ Plan emits PR A (repo X, `docs/**`) and PR B (repo Y, `server/**`) — disjoint, neither reads the other's diff → launch both now.
❌ Plan emits PR 1 and "PR 2 (after PR 1 merges)" — a prose dependency note is a planning defect caught at the readiness check, not a sequencing mechanism; PR 2 is a later planning round.

## Consistency check

Before declaring ready:

- Feature spec, acceptance criteria, verification plan, and decisions must agree with each other.
- Any requirement that cannot be verified, contradicts another, or expands beyond the feature boundary is resolved first.
- Every acceptance criterion is safely agent-executable (no live-infrastructure mutation); any criterion that would need a live path names its harness or is rewritten.
- Helpful but out-of-scope work is handled per the intake's disposition rule (`core/completion`) — a genuinely separate feature is split out, never silently folded into the plan; in-scope work is built, never parked.

### Driving-artifact discrepancies

Research will sometimes prove the driving artifact wrong — an issue/ticket/spec whose text contradicts verified code reality (wrong counts or names of topics/endpoints/variants, stale file paths, acceptance criteria referencing things that don't exist). The mismatch is a **question plus an artifact fix**, never a silent re-scope:

- Surface it as an explicit **question** with the evidence and the proposed correction — not merely as an insight the plan quietly works around. Whether the artifact or the code reflects the intent is a decision, and it is surfaced, never taken silently (the "wrong" text may encode a planned-but-unbuilt requirement).
- Offer to **correct the source artifact** (edit the issue/ticket body) so artifact and plan agree for every later reader and agent. Developer intake (`/plan`): the user confirms the fix. Executive intake (`/dream`): correct it autonomously and record it under decisions.
- ❌ Anti-pattern: the plan consumes the verified reality (e.g. 3 Kafka topics where the issue claims 4) and mentions the discrepancy only under Insights — the plan is right but the artifact stays wrong, and the decision was taken from the user silently.

## Decision-completeness

A plan or task spec is **decision-complete** when the implementer makes **zero** choices. The bar is a single question: *could a low-effort agent implement this without asking anything?* Concretely:

- **Every file is named** — the exact files/modules to create or edit, not "the relevant handler".
- **Every choice is resolved** — each non-trivial technical decision is either made in the spec or explicitly delegated **with the decision rule attached** (the rule the implementer applies to decide, not a bare "your call").
- **Acceptance checks are runnable verbatim** — the commands/tests that prove the task done are written out, not described.
- **Shared mechanisms are named** — task specs name the shared mechanisms the task composes (from the survey's composition map), so a low-effort coder never guesses infrastructure.

This applies uniformly to every artifact that hands work to an executor: `.plan/<slug>.md` contracts, tech-lead partitions, and quest work items. The system runs low-effort executors precisely because the decisions are made upstream — an underspecified spec pushes a decision onto an agent chosen for cheapness, which either stalls (escalation) or guesses (drift). Decision-completeness is what makes the executor's low effort safe.

## The plan contract — `.plan/<slug>.md`

When planning settles **and** the build will be driven by an implementation loop — `/forge`, the candyland conductor, or `/smith` via `/vibe` — write the settled plan to `.plan/<slug>.md` so the loop can ingest it without replaying the planning conversation. This is the **only artifact that crosses planning → implementation**.

- `slug` is a kebab-case summary of the feature.
- Contents are the deliverable fields above, with **acceptance criteria as a `[ ]` checklist** (the implementation loop ticks them).
- The contract is the build-phase source of truth: the loop conforms to it and never silently rewrites it.

A developer implementing in-session straight from `/plan` doesn't need the contract — the `/todo` import in `flows/plan/plan` is their resumable view. The contract is specifically what a *separate* loop reads; writing both when both apply is fine.
