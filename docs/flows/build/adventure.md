---
description: An open-ended freeseeking loop in the candyland sidecar — the same quest machinery with per-finding delivery: each accepted finding becomes its own child run and its own PR. Perpetual until stopped or discovery runs dry. The sidecar homologue of /janitor.
argument-hint: "[objective lens] [folder ...]"
triggers:
  - adventure
  - freeseeking loop
  - open-ended maintenance in the sidecar
  - sidecar janitor
when: User wants an open-ended discovery loop (maintenance, bug-hunting, compliance) running out-of-process in the candyland sidecar, delivering a PR per accepted finding over time, until stopped or the well runs dry.
related:
  - core/flows
  - core/sidecar
  - flows/build/janitor
  - flows/build/quest
  - core/loop
  - core/build
  - core/completion
---

# /adventure — open-ended freeseeking in the sidecar

An adventure is the **open-ended** sibling of `/quest`: the same machinery (a quest-lead ticking discover→triage→launch child runs) with a **per-finding** delivery policy — each accepted finding is a **distinct deliverable** → its own child run → **its own PR**. It is perpetual: it keeps ticking until stopped from the dashboard or discovery runs dry. This is the ONLY multi-PR shape (`core/flows` → *PR policy*), and the split from `/quest` is explicit at invocation — never inferred at runtime from objective wording. `/adventure` is `/janitor`'s sidecar homologue.

## The objective is a lens

An adventure's objective is not a terminating condition — it is a **lens** that steers discovery (safe maintenance, bug-hunting, compliance, test debt). Acceptance criteria are **self-managed**: triage decides per finding whether it is in-lens, in-boundary, and worth a run. Settle the same four loop-intent pieces as `/quest` (objective lens, scope, safety boundary, verification command), written to the objective file passed on argv.

## Steps

1. **Settle the loop intent** (lens, scope, safety boundary, verification) and write the objective file.
2. **Launch**: `detritus --adventure-run <objective-file> [folder ...]` — ensure-up, REST create + begin on the quest API with the per-finding policy (`core/sidecar`).
3. **Hand off to the dashboard**, printing the launch output contract (`core/sidecar`).

## Delivery

One PR per **accepted finding**, per impacted repo — one per distinct item, never repeated PRs for the same one; a later attempt at an already-PR'd finding delivers onto that PR (`core/build` → *One deliverable, one PR — converge, don't spray*). Triage never surfaces the adventure's **own delivery artifacts** (its open PRs, its branches) as new findings.

## Control

Observe + stop only, per `core/sidecar` — an adventure has no natural terminal; Stop is the intended ending.

**Incident hook.** A self-acknowledged mistake/doctrine violation ("you are right, I …", "I didn't follow …", "I ignored /…") or a blocker on a delivered PR is an incident per `core/ego`. This is a **sidecar** flow: the agent **records** the incident in its unit report — it never routes in the sidecar; the user's session routes it post-delivery (→ `/grow` / `/absorb`), never in the sidecar.
