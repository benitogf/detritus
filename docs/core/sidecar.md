---
description: The candyland sidecar contract — ensure-up lifecycle, the REST launch surface for runs/quests/adventures/campaigns, ports, stop-only control, the launch output contract, and the gh-mirror intake mapping. Do not invoke directly; composed by the sidecar launcher flows.
triggers:
  - sidecar
  - candyland sidecar
  - ensure-up
  - sidecar launch
  - gh-mirror intake
when: Internal. Loaded via kb_get by /candyland, /quest, /adventure, and /campaign for the shared launcher mechanics — lifecycle, REST surface, ports, control, launch output, and PR/issue intake classification.
related:
  - core/flows
  - flows/build/candyland
  - flows/build/quest
  - flows/build/adventure
  - flows/build/campaign
  - flows/github/gh
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
