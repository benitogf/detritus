---
description: The sidecar opt-in for the autonomous build flows. /candyland alone runs /vibe's flow (dream intake → autonomous → PR) with the build delegated to the candyland sidecar (a tech-lead + coders spawned out-of-process over an ooo bus, watched in a dashboard); invoked alongside /smith it runs /smith's flow the same way. /vibe and /smith stay in-process while the sidecar is polished — they default to it later.
argument-hint: "[feature description]"
triggers:
  - candyland
  - sidecar
  - build in the sidecar
  - watch the build
when: User wants an autonomous build run AND watched out-of-process in the candyland sidecar instead of consuming this session. Invoked alone it follows /vibe; alongside /smith it follows /smith. A transitional opt-in while the sidecar is polished.
related:
  - flows/plan/vibe
  - flows/build/smith
  - flows/build/forge
  - roles/tech-lead
  - core/coordination
  - flows/plan/plan
  - core/dream
---

# /candyland — run an autonomous build in the sidecar

`/candyland` is the **sidecar opt-in** for the autonomous build flows. It does not introduce a new build loop — it reuses `/vibe` and `/smith` exactly, but delegates the **build** to **candyland**, a standalone sidecar app that spawns the tech-lead + coders as real processes over an ooo bus and shows them live in a dashboard you monitor, audit, and stop. The session settles intent and hands off; candyland runs the agents out-of-process.

## Two modes — by what it's invoked with

- **`/candyland` alone → `/vibe`, in the sidecar.** Follow `flows/plan/vibe`: the executive/`dream` intake (no go-gate), settle a `.plan/<slug>.md` contract, then — instead of running `/smith` in-process — launch the build in candyland and report the dashboard. Autonomous all the way to a PR; you watch it build.
- **`/candyland` alongside `/smith` → `/smith`, in the sidecar.** When the message also contains `/smith`, follow `flows/build/smith` (the `/plan` gate, the acceptance checklist, the build-to-audit transition, the audit phase) — but its **build phase delegates to candyland** instead of building in-process. Planning and the audit phase stay in the session; only the parallel build runs in the sidecar.

In both modes `/vibe` and `/smith` themselves are unchanged — `/candyland` is a wrapper that swaps their in-process build for the sidecar.

> **Rollout note.** `/candyland` is a **transitional** opt-in. The intended end-state is that `/vibe` and `/smith` default to the sidecar with no separate command. Until the sidecar is polished, `/candyland` is the explicit way in, so the in-process `/vibe` and `/smith` keep working unchanged.

## Prerequisites (handled by the detritus install)

The detritus install puts the **candyland binary** on the machine and registers its **trigger MCP** (`candyland control-mcp`, exposing `launch_run` / `run_status` / `stop_run`) alongside detritus, so this session can call it. You do not start the sidecar by hand: `launch_run` is **resilient** — it health-checks the sidecar, starts it (detached) if it's down, and handshakes (polls until it actually answers) before delegating; if it can't come up it fails honestly. If the candyland MCP isn't available at all (older install), say so and fall back to the in-process flow (`/vibe` or `/smith`) rather than pretending.

## Steps

1. **Settle intent** for the active mode: `/candyland` alone → run `dream`'s intake to a settled `.plan/<slug>.md`; alongside `/smith` → run `/smith`'s `/plan` pass-through to the settled checklist. `/candyland` never invents the plan — it reuses the mode's intake.
2. **Launch.** Call the candyland MCP `launch_run`:
   - `prompt`: the settled plan (the `.plan/<slug>.md` contents, or the agreed instruction) — candyland's tech-lead partitions it into fork-safe tasks.
   - `folders`: omit to use this session's working directory (the repo you're in); `folders[0]` is the git repo it branches and opens its PR in.
   `launch_run` brings the sidecar up if needed, then starts the run and returns its id.
3. **Hand off to the dashboard.** Report the run id and that it's building in the candyland dashboard — that's where the live agents, task graph, and the per-task verification **audit** show, and where the run is stopped. In `/smith` mode, resume the session's audit phase once candyland's PR lands.

## Control (stop only)

candyland is lean: **observe + audit + stop**, no per-agent control, no resume. Halt a hung or wrong run with `stop_run <id>` (or the dashboard's Stop) — candyland owns the spawned processes, so it genuinely kills the tech-lead + coder tree. Use `run_status <id>` on demand; don't poll (the dashboard already shows live state).

## How it relates to the in-process flows

Same `roles/tech-lead` + `core/build` + `core/coder` choreography and `core/completion` definition of done — `/candyland` only changes **where the build runs and where you watch**:

- **`/forge`** — in-process, parallel, plan-first. This session is the tech-lead; coders are sub-agents. No dashboard.
- **`/smith`** — in-process, sequential, `/plan`-gated, autonomous to a PR then an audit loop. `/candyland` + `/smith` runs this with the build in the sidecar.
- **`/vibe`** — in-process, `dream` + `/smith`, no go-gate. `/candyland` alone runs this with the build in the sidecar.
- **candyland (the app)** — the out-of-process driver `launch_run` hands off to: it spawns + coordinates the tech-lead + coders over the ooo bus (`core/coordination` Realization B) and visualizes them. `/candyland` is the detritus seam that triggers it.
