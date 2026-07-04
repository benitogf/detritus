---
description: The candyland sidecar contract — ensure-up lifecycle, the REST launch surface for runs/quests/adventures/campaigns, ports, stop-only control, worktree hygiene, the launch output contract, and the gh-mirror intake mapping. Do not invoke directly; composed by the sidecar launcher flows.
triggers:
  - sidecar
  - candyland sidecar
  - ensure-up
  - sidecar launch
  - gh-mirror intake
  - worktree hygiene
  - coder worktree
when: Internal. Loaded via kb_get by /candyland, /quest, /adventure, and /campaign for the shared launcher mechanics — lifecycle, REST surface, ports, control, launch output, and PR/issue intake classification.
related:
  - core/flows
  - flows/build/candyland
  - flows/build/quest
  - flows/build/adventure
  - flows/build/campaign
  - flows/github/gh
  - flows/github/babysit
  - core/build
  - core/coordination
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
| quest | `detritus --quest-run <objective-file> [folder ...]` | `POST /api/quests` + `/begin` (converge policy) |
| adventure | `detritus --adventure-run <objective-file> [folder ...]` | quest API with the per-finding policy |
| campaign | `detritus --campaign-run <input-file> [folder ...]` | `POST /api/campaigns` + `/begin` |

Requests carry the full input plus a derived short `Title`, and `deliver`/`targetPr` when intake resolved them (below).

## Ports

API on **:8888**, UI on **:8080** — the UI loads from :8080 but reads its data from the API on :8888. On a remote/WSL host forward **BOTH** ports; printing only the UI URL leaves the dashboard blank.

## Control — observe + stop only

No per-agent control, no resume. The dashboard's Stop kills the whole spawned process tree. Watch live state in the dashboard rather than polling.

## Worktree hygiene

Each coder builds in its **own git worktree** off the run's branch (`core/coder`), so parallel siblings never collide. detritus does **not** own this lifecycle — candyland's conductor does — but the doctrine is fixed and the same idempotent rules bind every driver:

- **One worktree per coder task; the add is idempotent.** Before adding a worktree the driver clears any leftover at the same path and detaches every *other* worktree still registered on the target branch — a quick stop→edit→begin, a reused id, or a sibling child's leftover integration worktree can otherwise hold the branch and make a plain `worktree add` fail with "already used by worktree". `-B` (create-or-reset) then makes the branch a clean slate. This is what lets a run be re-begun or restarted without hand-cleanup.
- **A dirty holder of the branch at another path is never detached.** When the target branch is checked out in a *different* worktree with uncommitted changes, that holder is left untouched — the add then fails honestly rather than destroying unsaved work; only a *clean* other-holder's worktree registration is removed, and its commits survive on the branch. The reused path itself gets no such grace: the leftover at the coder's own path is cleared unconditionally, and `-B` resets the branch ref to the base — which is why a shared-branch driver resolves its base to the accumulated tip's SHA before adding, so earlier siblings' commits carry forward.
- **Shared-branch runs coordinate on the branch, not the path.** Campaign/quest children share one branch (`quest/<id>` for a standalone quest's children, `campaign/<id>` for a campaign's); because a branch can be checked out in only one worktree, the detach-other-holders step — not per-path clearing — is what keeps concurrent children from deadlocking on each other's leftover worktrees.
- **Teardown is best-effort and path-scoped.** Removing a worktree is `--force` (handles a dirty tree) and touches only that worktree's registration; it deliberately does not delete the branch, so a later restart can re-add it.

Because the lifecycle is idempotent — and dirty-safe toward every holder except the task's own reused path, which is reset by design — no flow needs a manual "clean the worktrees" step; leaving that management to the conductor is the contract, not an omission (`roles/tech-lead` → TL-F6).

## Watch-to-merge is in-session, never in the sidecar

The sidecar **never merges** — merge is irreversible and gated on a human review (`core/flows` → *PR-watch — the universal terminal phase*), while the sidecar surface is observe-and-stop-only. When a launched run/quest/campaign converges and opens its per-repo PR(s), the launcher reports each PR URL and points the user at `/babysit` (`flows/github/babysit`) to carry it to merge **in-session**: `/babysit` watches one PR, folds in reviewer feedback each tick, and merges the moment a SHA-pinned human `APPROVED` review covers HEAD. The watch loop runs in the user's session, not as a sidecar agent — it is the same universal terminal phase the in-session flows use, applied to the sidecar's delivered PRs.

## Launch output contract

Before handing off, print: the **id**, the **dashboard URL**, the **deliver mode**, a one-line **what this will do**, and **both ports**.

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
