---
description: Platform-specific scheduling and watch adapters for the recurring-loop commands (/janitor, /smith) and the event-driven watches (/babysit's merge-gate watch, /gh-pr's review-watch). Keeps Codex, Claude Code, GitHub Actions, Cursor, Windsurf, and generic scheduler/watch details out of the core workflows.
triggers:
  - janitor platform
  - janitor platforms
  - janitor scheduling
  - platform scheduling
  - event watch
  - pr watch adapter
  - codex janitor
  - claude janitor
  - claude code janitor
  - cursor janitor
  - windsurf janitor
  - github actions janitor
when: /janitor or /smith needs to create a recurring schedule on a specific platform, or /babysit (merge-gate watch) or /gh-pr (review-watch) needs an event-watch adapter for the host — or a flow needs the nearest supported scheduler/watch behavior explained.
related:
  - core/loop
  - flows/build/janitor
  - flows/build/smith
  - flows/github/babysit
  - flows/github/gh-pr
  - flows/github/gh
  - flows/github/gh-self-review
---

# Loop Platform Adapters

This document owns platform-specific scheduling behavior for the recurring-loop commands (`/janitor` and `/smith`), plus the **event-watch adapter** consumed by the event-driven watches — `/babysit` (the merge-gate watch) and `/gh-pr` (the review-watch). Both are event-watch consumers; they share this one adapter and differ only in what the woken actor does (`/babysit` fixes-and-merges; `/gh-pr` re-reviews and re-posts a verdict). Do not duplicate these details in `core/loop`, `flows/build/janitor`, `flows/build/smith`, `flows/github/babysit`, or `flows/github/gh-pr`.

The shared loop fundamentals (scratchpad layout, durability rule, target-scoped state, cadence guideline, skip-streak guardrail, `/gh` delivery) live in `core/loop`. Each command's own doc supplies its specific loop steps, audit-agent contract, safety boundaries, and report shape. The adapter's job is only to schedule the loop — or, for an event-watch, arm the watch — on the host platform.

## Shared Adapter Rules

- Speak to the user in human terms: `every 5 minutes`, `hourly`, `overnight`, `weeknights`.
- Translate internally to the platform's scheduler syntax only when needed.
- Do not show cron strings, session IDs, lock files, API tokens, or low-level flags unless the user asks.
- Preserve the janitor loop exactly; platform adapters do not redefine what work is allowed.
- Start one run immediately when the platform supports it.
- If the requested cadence is below the platform's minimum, round up to the minimum and report the effective cadence; do not ask the user a clarifying question for this case.
- If the requested cadence is unsupported in a way that materially changes behavior (not just rounded up), choose the nearest safe option and say what changed.
- Every **poll-per-tick maintenance adapter** (the `/janitor` / `/smith` schedulers) must preserve target-scoped non-overlap: a wake checks existing branches, issues, PRs, or state before starting fresh audit work. The **event-watch adapter** (`/babysit`, `/gh-pr`) does not participate in this — it watches a single named target and carries no scratchpad; non-overlap does not apply.

## `--platform auto`

Detect the current host when possible and choose the closest supported behavior:

1. Codex app: prefer a thread heartbeat for short intervals such as `5min`.
2. Codex app with hourly/weekly cadence: use a cron/worktree automation when the requested schedule fits detached work.
3. Claude Code: default to **Desktop Routines** via `/schedule` when the user can keep the Desktop app open during wake windows, or to the **external scheduler fallback** (`claude -p` triggered by cron / launchd / systemd / Task Scheduler) when always-on durability is required and Agent-SDK billing is acceptable. When the user wants usage-limit resilience without `claude -p` billing and will keep a Claude REPL open, use the **in-session durable cron** (`CronCreate durable:true`) — seat-billed, standing-schedule, survives limit kills where `/loop` does not. All three run against the actual local checkout and can read/write the gitignored scratchpad — matching `/janitor`'s workspace-scoped intent. Use **Cloud Routines** only when the janitor is explicitly opting into GitHub-state-only operation against a remote repository, not the user's workspace; Cloud Routines clone the repo fresh per tick and cannot see local state. Never default to `/loop` for `/janitor` — it is fragile under usage limits (a limit-killed tick ends the whole loop).
4. GitHub Actions: create or recommend a scheduled workflow only when repository-hosted CI automation is acceptable.
5. **GitHub Copilot (VS Code)**: `/janitor` and `/smith` are **not supported** — see the Copilot section below before proceeding.
6. Cursor or Windsurf: install the reusable command/workflow instructions, then require that recurrence be driven by platform UI support or an external scheduler if no native recurring scheduler is available.
7. Generic: provide a scheduler-ready prompt and leave recurrence to cron, launchd, systemd timers, Windows Task Scheduler, or the host orchestrator.

## Codex

Use the Codex app automation API.

- Codex execution modes map to *Durability* in `core/loop` (under *Durable Cross-Tick State*): `local` is durable; `worktree` is disposable. The specifics for each Codex automation type follow.
- For `5min` and other short intervals, create a heartbeat automation attached to the current thread. This preserves continuity and lets each wake continue pending work.
- For hourly, daily, or weekly detached maintenance, use a cron automation only when the janitor can run from a durable workspace. Prefer `local` execution against the resolved checkout when the loop needs gitignored `.janitor/` state across cold starts.
- Use `worktree` execution only for janitors that can fully reconstruct state from GitHub-side branches, issues, PRs, and the automation prompt. Do not assume uncommitted or gitignored `.janitor/` files from a previous worktree tick will exist on the next tick.
- `/smith` additionally requires a durable runner (mode 1): `worktree` stays fine for `/janitor`, but not for `/smith`. See `flows/build/smith` → *Build Phase Durability*.
- Start one run immediately after creating the schedule so the user can confirm it works.
- The automation prompt must include the full janitor loop contract, target, topics, verification expectations, and concise reporting requirements.
- The runtime-derived scratchpad slug comes from the invocation root and topic, not from the Codex thread or automation id. Heartbeat and local cron ticks should read and update `.janitor/<slug>.md` in that resolved root; detached worktree ticks must either use a durable local checkout or report that the scratchpad cannot be persisted under the requested execution mode.
- Per-tick report files follow the same durability rule as the scratchpad: they are temporary handoff files inside `.janitor/`, and the main tick must fold them into the scratchpad before the wake ends. Worktree ticks that cannot preserve `.janitor/` between runs must not leave the only copy of audit detail in a disposable worktree.

## Claude Code

Claude Code Routines are the first-party scheduling primitive. Two variants share the same `/schedule` entry point:

- Default for `/janitor` on Claude Code is **Desktop Routines** when the user can keep the Desktop app open during wake windows, or the **external scheduler fallback** (`claude -p` triggered by cron / launchd / systemd / Task Scheduler) for always-on durability. Both run against the user's actual local checkout and can read/write the gitignored scratchpad — matching `/janitor`'s workspace-scoped intent. Cloud Routines clone the repo fresh per tick and only see GitHub-side state; they are an explicit opt-in for janitors operating in *Durability* mode 2 (GitHub-state-only) against a remote repository, not the workspace.
- **Desktop Routines** (`/schedule` → Routines → Local): run locally against the actual workspace checkout. Minimum interval is 1 minute. Default for `/janitor` when the user keeps the Desktop app open during wake windows. Desktop Routines do not fire while the app is closed — if the user needs always-on durability, use the external scheduler fallback instead.
- **External scheduler plus Claude Code CLI** (cron / launchd / systemd / Task Scheduler driving `claude -p`): durable sibling to Desktop Routines — fires regardless of Desktop app state, against the same local checkout. Default for `/janitor` when the user wants always-on durability without keeping the Desktop app open, or when first-party Routines are unavailable.
- **Cloud Routines** (`/schedule` → Routines → Cloud): run on Anthropic-managed infrastructure, clone the selected repository at the start of each run, and survive without the user's machine. Minimum interval is **1 hour**; cron expressions evaluating to a sub-hour cadence are rejected at routine-creation time. Opt-in for `/janitor` when the loop is explicitly GitHub-state-only — no workspace, no local scratchpad, no rolling metric history beyond what survives in issue / PR bodies.
- **In-session durable cron** (`CronCreate` with `durable: true`): a standing schedule that fires inside the running interactive session against the local checkout. Seat-billed — it never spawns `claude -p`, so it does not draw on the Agent-SDK credit pool the way the external scheduler does. Standing-schedule, so it is **usage-limit resilient** (see *core/loop* → *Usage-Limit Resilience*) where `/loop` is not. Choose this when the user wants limit-resilient durability without `claude -p` billing and is willing to keep a Claude REPL open. Details below.
- **`/loop`**: session-scoped self-rescheduling polling inside the current conversation. **Fragile under usage limits** — it re-arms the next tick at the end of each tick, so a tick that dies on a seat/token limit stops the whole loop, not just that tick (*core/loop* → *Usage-Limit Resilience*). Stops when the session ends. Office-hours / attended use only; never present it as unattended-durable.
- **`Monitor` event-watch**: an out-of-band background poll that wakes the model only on change, rather than every interval. Unlike all of the above — which spend a full model turn each wake even when nothing changed — the model is idle between events. Fits **event-driven watches over a single remote target** (the PR-watch shape — `/babysit` / `/gh-pr`), not the poll-per-tick maintenance loops. Session-scoped and attended (dies with the session). Details below.

`/smith` requires a durable runner (mode 1), so Cloud Routines (disposable) cannot host it — use Desktop Routines or the external scheduler. Cloud remains fine for `/janitor`. See `flows/build/smith` → *Build Phase Durability*.

### Desktop Routines (default for workspace janitors)

Default Claude Code path for `/janitor` when the user keeps the Desktop app open during wake windows.

1. Resolve target, topics, interval, and scratchpad slug from the core janitor Inputs section. The scratchpad lives at `.janitor/<slug>.md` in the resolved invocation root.
2. Compose a self-contained janitor prompt. Routines run as fresh sessions, so the prompt must inline the target, topics or default review rubric, the full janitor loop (including the step 2 scratchpad read), audit-agent contract, main-agent contract, state/non-overlap rule, durability mode (mode 1 — durable), safety boundaries, verification expectations, and `/gh` delivery rule. Routines do not expand slash commands inside the prompt.
3. Invoke `/schedule` → Routines → Local. Confirm the user will keep the Desktop app open during the requested wake windows. If they cannot, route to the external scheduler fallback below — Desktop Routines silently skip ticks while the app is closed.
4. Trigger one immediate "Run now" so the user sees a first tick in the same conversation.
5. Report the effective cadence in human terms and include the routines management URL (`https://claude.ai/code/routines`) so the user can pause / edit / delete later.

Non-overlap is platform-managed by Claude Code: only one routine fires at a time per target.

### External scheduler (default for always-on durability)

Default Claude Code path when the user wants durability without the Desktop-app-open constraint. The agent — not the user — sets up the equivalent locally: generate a `flock`-guarded entry that invokes `claude -p --maintenance --permission-mode bypassPermissions --session-id <stable-id> --output-format json "<janitor prompt>"` on the user's OS scheduler (cron, launchd, systemd timers, or Windows Task Scheduler), wire it for them when permitted, and report in human terms exactly as the Desktop Routines path would. Cron syntax, lock files, and CLI flags stay internal unless the user asks how it works.

Same durability characteristics as Desktop Routines (mode 1) — runs against the actual local checkout, can read/write the scratchpad on disk. Difference is the lifecycle: external scheduler fires regardless of whether the Claude Code Desktop app is running.

Claude Code hooks are useful for lifecycle behavior inside a tick (Setup `maintenance` matcher, Stop hooks), but hooks are not the source of recurrence.

### In-session durable cron (limit-resilient, seat-billed)

Use when the user wants a loop that survives a usage-limit hit (and Claude restarts) **without** the Agent-SDK billing of the external `claude -p` scheduler, and is willing to keep a Claude REPL open. This is the standing-schedule answer to *core/loop* → *Usage-Limit Resilience*: unlike `/loop`, the schedule is persisted data, so a tick that dies mid-run on a seat/token limit cannot end the loop — the next scheduled fire still occurs, and the first fire after the seat resets succeeds with no manual restart.

Durability is mode 1 (durable): ticks fire against the actual local checkout and read/write the gitignored `.janitor/<slug>.md` scratchpad on disk.

1. Resolve target, topics, interval, and scratchpad slug from the core janitor Inputs section.
2. Compose a self-contained janitor prompt. Each fire is a fresh session, so the prompt must inline the target, topics or default review rubric, the full janitor loop (including the step 2 scratchpad read), audit-agent contract, main-agent contract, state/non-overlap rule, durability mode (mode 1), safety boundaries, verification expectations, and `/gh` delivery rule — same inlining requirement as Desktop Routines.
3. Create the schedule with `CronCreate`, `durable: true`, `recurring: true`, and a cron expression matching the requested cadence (use an off-:00/:30 minute per the tool's guidance). Persists to `.claude/scheduled_tasks.json`.
4. Trigger one immediate tick so the user sees a first run.
5. Report the effective cadence in human terms.

Constraints to state to the user up front:

- Fires only while a Claude REPL is **idle** (not mid-query) and **running** — it does not fire while Claude is fully closed. For always-on-while-closed durability the user must accept the external `claude -p` scheduler (and its Agent-SDK billing) instead.
- `recurring` cron jobs **auto-expire after 7 days**, firing one final time then deleting. For a perpetual janitor, re-create on expiry or keep a lightweight weekly refresh.
- Non-overlap is not platform-guaranteed; rely on the target-scoped state check (loop step 3) at every wake, as with Cloud Routines.

### `Monitor` event-watch (event-driven, token-cheap)

An event-watch primitive, distinct from the poll-per-tick options above. Where a standing schedule / self-arm / `/loop` wakes the *model* every interval — spending a full model turn even on an interval where nothing changed — a `Monitor` run `persistent: true` polls a single remote target **out of band** and emits **one notification per gate-state change**. Each emitted stdout line is one event, i.e. one model wake; between events the model is idle, so **cost is ∝ the number of events, not wall-clock**. Watching becomes near-free; you pay only when there is something to react to.

**When to use it.** Event-driven watches over a **single remote target** where the caller reacts to changes — the PR-watch shape (watch one PR, act on each change). It is *not* the maintenance-loop model: it carries no scratchpad and does not participate in janitor's target-scoped-non-overlap discipline. Keep it to single-target event watches; the poll-per-tick options remain the fit for `/janitor` / `/smith`.

**Two consumers, one adapter — different acted-on states.** The adapter serves both PR-watch consumers; the emit filter is the union of what either acts on:

- **`/babysit` — merge-gate watch.** Acts on each new review / check-conclusion / **comment** / mergeable transition, fixing feedback and merging on a SHA-pinned approval. Its emit set: `gate: mergeable`, `gate: ci-failed`, `gate: changes-requested`, new-comment, merged, closed.
- **`/gh-pr` — review-watch.** Re-reviews and re-posts a verdict (never merges, never fixes). Its acted-on states are: a **new push / HEAD move**, a **new discussion (comment)**, **merged**, and **closed** — so for this consumer the emit filter must cover **HEAD-move + new-discussion**, not only the merge-gate transitions. A push that moves `head.sha` (the review-watch's re-review trigger) must yield a new `cur` line → an emitted event, exactly as the two comment sources already do.

**Availability — verify, don't assume (before dropping to a degraded fallback).** A degraded fallback (self-arm / on-demand re-invoke) fires *only when the preferred primitive genuinely cannot be armed* — and that is a fact to **confirm**, never a default to assume. Before concluding the event-watch is unavailable, actually check that the `Monitor` primitive can't be armed, **including tools that must be loaded on demand**: a deferred tool surfaced by name is *loadable* (fetch its schema via `ToolSearch` / the host's tool-loading path), so a name in that list means **available**, not absent. Asserting "no event-watch primitive in this session" without that check — then self-arming a per-interval scheduler as if it were the default — is the exact miss this note prevents. The rule generalizes past `Monitor`: **any fallback keyed on "no X available" must verify X's absence before firing; "I don't see it loaded" is not "it isn't there."**

**Durability: session-scoped (attended).** The `Monitor` process dies when the session ends — `/clear`, a closed terminal, the machine sleeping. Its only state is the in-process previous-poll diff; nothing survives session death, and it carries no scratchpad. This is the honest counterpart to `/loop`'s attended-only framing (*core/loop* → *Durability*). For a watch that must survive session death, fall back to the **in-session durable cron** (or another standing schedule) above — but that re-polls per fire (a model turn per fire, so the token win does **not** apply) and must carry the last-seen state forward across fires so it still acts only on *new* changes.

**The watch is notify-only.** It performs no mutating actions — read-only fetches only, no merge / comment / body-edit / branch-update. The woken caller (the actor flow) performs any mutation. This holds for **both** consumers: under `/gh-pr`'s review-watch the watch itself still mutates nothing — the woken caller posts the verdict, and even that caller is notify-only in the review sense (it re-posts a review only; it never merges, comments outside a review, or edits the PR body/branch).

Generalized script shape — a `persistent: true` poll loop that emits only what changed since the last poll and ends only on the target's terminal state:

```sh
# persistent Monitor; each emitted line -> one notification (one model wake); runs until the target is terminal
prev=""
while true; do
  # Assemble the FULL cur set from all sources; ANY sub-fetch failing makes the whole poll inconclusive.
  cur=$(
    gh api repos/<owner>/<repo>/pulls/<n> --jq '...head_sha/mergeable/mergeable_state/merged/state...' &&
    gh api ... pulls/<n>/reviews          --jq '... per-reviewer latest state, sorted ...' &&
    gh api ... commits/<head_sha>/check-runs --jq '... newly-terminal conclusions, sorted ...' &&
    # comment sources folded into cur so a comment-only change produces a NEW line -> an emitted event:
    gh api ... pulls/<n>/comments         --jq '... latest created_at / id per inline comment ...' &&
    gh api ... issues/<n>/comments        --jq '... latest created_at / id per issue comment ...'
  ) 2>/dev/null || { sleep 45; continue; }           # partial/failed poll is inconclusive -> skip; do NOT touch prev
  comm -13 <(echo "$prev") <(echo "$cur")            # emit ONLY newly-changed lines; never re-emit an unchanged state
  prev="$cur"                                        # updated ONLY after a fully-successful cur assembly
  # emit gate: mergeable @<head7> on the strict composite; gate: ci-failed on a failed required check;
  # emit gate: changes-requested; emit head-moved @<head7> (review-watch's re-review trigger); emit new comment;
  # emit terminal: merged / terminal: closed AND break on merged/closed
  sleep 45                                           # 30s+ for remote APIs (rate-limit guidance)
done
```

`cur` must fold in **all three feedback sources** — the reviews set (keyed on state), inline review comments, and issue-thread comments — not just check conclusions and reviews. A plain issue-comment or inline reply changes no check and no review state; if it is not in `cur`, `comm -13` emits nothing and the comment sits invisible on a quiet PR forever. Key the two comment sources so a new comment yields a new `cur` line (a latest-`created_at`, or an id set) → an emitted event.

Rules the watch script must carry:

- **`gh api` only** — never `gh pr view` / `gh pr checks` in the script (`flows/github/gh` #2: the Projects-classic GraphQL deprecation breaks them on some repos even where the REST call works).
- **Coverage — silence is not success** (the `Monitor` contract's own rule): the emit filter must cover **every** state the caller acts on. For `/babysit`: CI *failure* conclusions, `CHANGES_REQUESTED`, a **new comment** (inline or issue), merged, **and** closed. For `/gh-pr`'s review-watch: a **new push / HEAD move**, a **new discussion (comment)**, merged, **and** closed. A watch that emits only good news goes silent through a CI crash — or a review-comment on a quiet PR, or **a fixing push the review-watch needed to re-review** — and every such miss looks identical to "still waiting." The `head.sha` line already in `cur` covers the HEAD-move emit; the two comment sources cover new-discussion.
- **Terminal = merged/closed only.** The loop `break`s (ends the watch) only when the target is merged or closed; `gate: mergeable`, `gate: ci-failed`, `gate: changes-requested`, new-comment, and per-check / per-review events are all **emit-and-continue** — the woken actor decides what to do on each.
- **Inconclusive polls never emit and never advance `prev`.** `mergeable_state == unknown` / `mergeable == null` (GitHub still computing, typically right after a push) → do not emit a false event; let the next poll settle it. A failed / partial `gh api` → the **whole** `cur` assembly is inconclusive: `continue` without touching `prev`. Guard the entire multi-source assembly, not just the first fetch — a partial `cur` that overwrote `prev` would drop lines and then re-emit them as "new" on the next full poll.
- **`comm -13` newly-changed diff** — emit only what changed since the previous poll; never re-emit an unchanged state. A too-chatty watch is auto-stopped by `Monitor`.
- **Poll interval 45–60s** (30s+ for remote APIs per `Monitor` guidance); honor `core/loop` cache-window guidance.

### Cloud Routines (GitHub-state-only opt-in)

Use only when the user explicitly opts into a janitor that operates without local workspace state — for example, a maintenance bot on a GitHub repo the user does not have checked out locally.

1. Resolve target, topics, and interval from the core janitor Inputs section. The scratchpad does not persist on this path; the loop operates under *Durability* mode 2 (GitHub-state-only).
2. Compose a self-contained janitor prompt. Routines run as fresh sessions, so the prompt must inline the target, topics or default review rubric, full janitor loop, audit-agent contract, main-agent contract, state/non-overlap rule, durability mode (mode 2 — GitHub-state-only; no scratchpad), safety boundaries, verification expectations, and `/gh` delivery rule.
3. Invoke `/schedule` → Routines → Cloud. The platform will prompt for repository access; pass the resolved target so the selection is pre-filled when possible.
4. If the requested cadence is below the 1-hour minimum, round up to 1 hour and state the effective cadence in the report.
5. Trigger one immediate "Run now" so the user sees a first tick in the same conversation.
6. Report the effective cadence in human terms and include the routines management URL (`https://claude.ai/code/routines`).

Non-overlap on Cloud Routines is not automatic — the platform may fire overlapping ticks. The janitor loop handles this by inspecting open branches, draft PRs, and open issues on the target at every wake (loop step 3), since each run is a fresh session with no in-memory state from prior ticks. Blockers, feature-splits, and rolling state that would have lived in the scratchpad live in issue or PR bodies instead.

## GitHub Actions

Use only when repository-hosted automation is acceptable and the work can run unattended in CI.

- Scheduled workflows use cron syntax and can be set as frequently as every 5 minutes, but GitHub Actions cron is best-effort and runs may be delayed or skipped under runner contention. Treat the interval as a floor, not a guarantee.
- Add `workflow_dispatch` so developers can start a tick manually.
- Use one concurrency group per target branch or workspace and set `cancel-in-progress: false` to avoid overlapping janitor runs.
- Prefer opening issues or PRs over pushing directly to protected branches.
- Treat secrets and permissions narrowly. Grant only the permissions required to create branches, issues, or PRs.

GitHub Actions is best for remote, auditable janitor ticks. It is a poor fit for interactive local context, local-only files, or IDE-specific state.

## Cursor

Use Cursor background agents for long asynchronous work when available.

- Start a background agent with the janitor prompt and target.
- Use an external scheduler or platform UI support for recurrence if available.
- Preserve the same janitor loop contract in the prompt.
- Require branch/PR review before merging any changes.

## Windsurf

Use a Windsurf workflow for the reusable janitor procedure.

- Store the workflow as `/janitor` instructions.
- Windsurf workflows are manually invoked; use an external scheduler or platform support for recurrence if available.
- Keep the workflow body platform-neutral and refer back to the core janitor loop.

## GitHub Copilot (VS Code)

**`/janitor` and `/smith` are not supported on GitHub Copilot.**

Both commands require *session-continuation* between ticks: each tick must be the same logical agent reading the scratchpad from the prior tick, accumulating context, and picking up where it left off. GitHub Copilot (VS Code) has no mechanism for this:

- There is no `/schedule`, no Routines API, no heartbeat mechanism.
- The `copilot` shim in the VS Code extension path is not a standalone session-resuming CLI.
- An external cron job would start a fresh, disconnected session with no context from previous ticks — that is the Generic adapter, not the loop model.
- The loop only advances when the user explicitly sends a message in the same conversation.

**What Copilot can do within the janitor scope:**
- Execute a single tick on demand — audit, fix, test, and route to `/gh`. All the code-work steps function correctly.
- Persist the `.janitor/<slug>.md` scratchpad across user-triggered conversations (the user must re-enter the same chat thread or reference the scratchpad explicitly).
- Run the full `/gh` delivery flow end-to-end within a single session.

**What to tell the user when asked to start `/janitor` on Copilot:**
State clearly that the recurring loop is not supported. Offer to run one tick now. If the user wants automation, recommend Claude Code (Desktop Routines or external scheduler) or GitHub Actions instead.

## Generic

Use the host's scheduler to run the agent CLI, SDK, or automation entry point.

- Linux/macOS: cron, launchd, or systemd timers.
- Windows: Task Scheduler.
- CI/orchestrators: scheduled pipeline, queue worker, or durable job runner.
- Always configure a non-overlap lock when available.
- Always rely on the janitor loop's target-scoped state check before starting new audit work.
