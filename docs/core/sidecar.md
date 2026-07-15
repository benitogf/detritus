---
description: The candyland sidecar contract — ensure-up lifecycle, the REST launch surface for runs and quests, ports, stop-only control, worktree hygiene, the launch output contract, and the gh-mirror intake mapping. Do not invoke directly; composed by the sidecar launcher flows.
triggers:
  - sidecar
  - candyland sidecar
  - ensure-up
  - sidecar launch
  - gh-mirror intake
  - worktree hygiene
  - coder worktree
when: Internal. Loaded via kb_get by /candyland and /quest for the shared launcher mechanics — lifecycle, REST surface, ports, control, worktree hygiene, launch output, and PR/issue intake classification.
related:
  - core/flows
  - flows/build/candyland
  - flows/build/quest
  - flows/github/gh
  - flows/github/babysit
  - core/build
  - core/coordination
  - roles/tech-lead
---

# Sidecar Core — launching and driving candyland

> ## ⛔ Do not invoke directly
> No slash command. Loaded via `kb_get` by the sidecar launcher flows; it holds the mechanics so no launcher doc restates them.

## Ensure-up lifecycle

detritus owns the sidecar lifecycle in Go (candyland is not an MCP server): health-check `GET /api/health`; if down, start the installed binary **detached, inheriting the environment** so `gh`/`HOME`/`GH_*` credentials propagate to the spawned agents; poll until it answers; if it can't come up, fail honestly. If the binary isn't installed, say so and fall back to the in-process flow rather than pretending.

## REST launch surface

Each launcher writes its input to a file (keeps large text off argv), then detritus creates and begins the work over REST. Folders default to the **cwd**; every folder is a **candidate repo** — candyland branches and delivers in each folder that receives changes (`core/build` → *Multi-repo delivery*).

| Flow | Command | REST |
|---|---|---|
| run | `detritus --candyland-run <plan-file> [folder ...]` | `POST /api/runs` + `/begin` |
| quest | `detritus --quest-run <objective-file> [--per-finding] [folder ...]` | `POST /api/quests` + `/begin` (`--per-finding` → per-finding policy; else converge) |

Requests carry the full input plus a derived short `Title`, and `deliver`/`targetPr` when intake resolved them (below).

## Ports

API on **:8888**, UI on **:8080** by default — the UI loads from the SPA port but reads its data from the API port. These are defaults, not invariants: when a foreign app holds a default port, the launcher picks free ports and the sidecar advertises its actual endpoint in `~/.candyland/endpoint.json` (ports/pid/version, written at bind, removed on clean shutdown) — read that file to find a running sidecar rather than assuming :8888. On a remote/WSL host forward **BOTH** resolved ports; printing only the UI URL leaves the dashboard blank.

## Control — observe + stop only

No per-agent control, no *manual* resume. The dashboard's Stop kills the whole spawned process tree. Watch live state in the dashboard rather than polling. (The conductor's automatic pause/resume on a usage limit or connection loss — below — is a runtime behavior, not a control surface: the user never resumes an agent by hand.)

## Auto-pause and resume — usage limits and connection loss

A spawned agent that dies because the Claude seat hit its **usage limit** or the API became **unreachable** is not a fault of the agent's work, so the conductor does not fail it. Both classes are caught at the single spawn choke point every coordinator and run spawn routes through, classified from the terminal death signal (the process's stderr/exit, or a non-success result subtype — never a successful spawn's own result text, so an agent that merely mentions a limit or a network error is not misread), and handled by pausing rather than blocking:

- **Truthful non-terminal status.** The affected run/quest goes to `paused` with a `PauseReason` that names the cause — `usage limit — auto-resume at <t>` or `connection lost — retrying` — and no postmortem. A limit/connection pause is distinct from a user Stop (also `paused`, but no auto-resume) and from `blocked` (a real capability failure, postmortem-backed).
- **Conductor-wide gate.** A usage limit is account-global, so on detection every subsequent spawn waits until the window resets. A connection loss arms the same fleet-wide gate only after **consecutive** deaths (a sustained outage); a single blip pauses just its own run.
- **Auto-resume in place, losing minimal work.** When the window reopens (the parsed reset time for a limit; an escalating backoff — minutes to an hour — for a connection loss), each interrupted agent is resumed in its own session (`--resume`, cold-boot fallback if the session is gone) and continues where it was; persisted stage/gate/branch state is untouched. Resume is always on and unbounded (a limit or outage can recur); Stop during a pause still cancels cleanly.
- **Durable across restarts.** `resumeAt` is persisted on the paused entity, and a conductor restart during the wait re-arms the gate from storage, so the pause survives a sidecar bounce and resumes on schedule.
- **Concurrency is preserved.** The gate pauses and resumes *all* concurrent fleets together on a shared-seat or outage event — running many quests/runs at once stays safe under exhaustion; it is never a reason to serialize them.

The dashboard and the run/quest workspaces show the paused indicator with its cause and, when known, the resume time.

## Worktree hygiene

Each coder builds in its **own git worktree** off the run's branch (`core/coder`), so parallel siblings never collide. detritus does **not** own this lifecycle — candyland's conductor does — but the doctrine is fixed and the same idempotent rules bind every driver:

- **One worktree per coder task; the add is idempotent.** Before adding a worktree the driver clears any leftover at the same path and detaches every *other* worktree still registered on the target branch — a quick stop→edit→begin, a reused id, or a sibling child's leftover integration worktree can otherwise hold the branch and make a plain `worktree add` fail with "already used by worktree". `-B` (create-or-reset) then makes the branch a clean slate. This is what lets a run be re-begun or restarted without hand-cleanup.
- **A dirty holder of the branch at another path is never detached.** When the target branch is checked out in a *different* worktree with uncommitted changes, that holder is left untouched — the add then fails honestly rather than destroying unsaved work; only a *clean* other-holder's worktree registration is removed, and its commits survive the detach (they persist on the branch only when the driver bases the add on the accumulated tip's SHA). The reused path itself gets no such grace: the leftover at the coder's own path is cleared unconditionally, and `-B` resets the branch ref to the base — which is why a shared-branch driver resolves its base to the accumulated tip's SHA before adding, so earlier siblings' commits carry forward.
- **Shared-branch runs coordinate on the branch, not the path.** A quest's child runs share one branch (`quest/<id>`); because a branch can be checked out in only one worktree, the detach-other-holders step — not per-path clearing — is what keeps concurrent children from deadlocking on each other's leftover worktrees.
- **Teardown is best-effort and path-scoped.** Removing a worktree is `--force` (handles a dirty tree) and touches only that worktree's registration; it deliberately does not delete the branch, so a later restart can re-add it.

Because the lifecycle is idempotent — and dirty-safe toward every holder except the task's own reused path, which is reset by design — no flow needs a manual "clean the worktrees" step; leaving that management to the conductor is the contract, not an omission (`roles/tech-lead` → TL-F6).

## Watch-to-merge is in-session, never in the sidecar

The sidecar **never merges** — merge is irreversible and gated on a human review (`core/flows` → *PR-watch — the universal terminal phase*), while the sidecar surface is observe-and-stop-only. When a launched run/quest converges and opens its per-repo PR(s), the launcher reports each PR URL and points the user at `/babysit` (`flows/github/babysit`) to carry it to merge **in-session**: `/babysit` watches one PR, folds in reviewer feedback as it arrives, and merges the moment a SHA-pinned human `APPROVED` review covers HEAD. The watch loop runs in the user's session, not as a sidecar agent — it is the same universal terminal phase the in-session flows use, applied to the sidecar's delivered PRs.

## Launch output contract

Before handing off, print: the **id**, the **dashboard URL**, the **deliver mode**, a one-line **what this will do**, and **both ports**.

## No input is not an input — never launch on ambient context

Every launcher requires an **explicit target**: a plan file/slug, a PR/issue reference, or a described objective present in the conversation. When the invocation carries **none** — a bare `/candyland` / `/quest` with no argument and nothing in the conversation naming the work — **ask which target; do not infer one.** Ambient IDE state (the currently-open editor file, a text selection) is flagged "may or may not be related" and is **never** a launch argument. A launcher spawns a branching, out-of-process agent tree that opens PRs; inferring its target from an open file is an unrequested launch, and stopping it after the fact still burns the spawn. This is the launch-side counterpart to `core/build`'s converge-don't-spray — the cheapest wrong PR is the one never launched.

## PR-link intake mirrors /gh

`flows/github/gh` Phase 1 is the ONE canonical classifier; the launchers implement its **outcome** in Go against freshly-fetched state (`gh api`) whenever the input references a PR/issue:

| /gh classification | Launcher outcome |
|---|---|
| open PR with unaddressed CHANGES_REQUESTED (even if the user said "review") | `deliver: feedback` + targetPr |
| open PR + review-intent text | `deliver: review` + targetPr |
| open PR with comments after last commit, no review text | `deliver: feedback` + targetPr |
| open PR, no post-commit comments, no review text | ask once (review / feedback / cancel) |
| open issue | `deliver: pr` (objective references the issue) |
| merged/closed PR or closed issue | STOP and ask — never silently launch |

Never open a duplicate PR for a feedback/review intent; never silently default a feedback/review input to `pr`.
