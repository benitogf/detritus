---
description: Upstream your local lessons — generalize each via /grow, stage them, and ship them into a repo's shared lessons/ directory via a normal PR. A transport gateway, not a gate.
triggers:
  - contribute
  - contribute lessons
  - share lessons
  - ship lessons
  - upstream lessons
  - lesson gateway
  - pool lessons
  - contribute learned memory
when: User wants to gather their local learned lessons and share them into a shared repo for review/curation, or invokes `detritus --contribute`.
related:
  - flows/maintainer/grow
  - flows/maintainer/learn
  - flows/github/gh
---

# /contribute — Lesson Contribution Flow

> ## /contribute IS /grow APPLIED TO THE LOCAL LESSON STORE, FOR UPSTREAMING
>
> `/contribute` is to the local lesson store what `/grow` is to a session correction:
> **same generalization step, different destination.** `/grow` distils a correction into a
> KB delta and ships it via `/gh`; `/contribute` takes the lessons already sitting in your
> local store, **generalizes each one with /grow's generalization**, and ships them upstream
> into a shared repo's `lessons/` directory via a normal PR.
>
> The generalization is a **/grow flow step composed by reference, not restated** (this KB
> forbids duplicated prose). The `detritus --contribute` CLI is only the **transport** — it
> gathers staged `*.md` files and opens the PR. It does **no** filtering and **no** content
> transform of its own.

---

## The gateway is a vehicle, not a bouncer

`detritus --contribute` gathers the lesson `*.md` files in a local source dir and ships them
into a single shared place — a `lessons/` directory in the target repo — through the ordinary
PR flow. It applies **no eligibility filter** (active or stale, confirmed or not, all ship)
and **no special-case scrubbing**. Maturity fields (`status`, `confirmed`, `validity`, …)
travel inside each file **as data** for downstream curation; they are never a promotion gate.

**The same PR review loop that gates every other PR is the filter and correction gate here.**
There is no bespoke redactor, threshold, quorum, or vote in the CLI. Reviewers accept, reject,
or curate — exactly as they do for any other PR.

---

## The flow

### Step 1: Generalize each local lesson (the /grow step)

For **each** lesson in your local store, generalize it past the one incident that produced it
into the reusable, incident-independent principle — using **/grow's generalization** verbatim
(`kb_get flows/maintainer/grow`, Step 3 "Generalize past the trigger"):

- Encode the **underlying rule**, not the specific incident. The originating case is at most
  one illustrative `e.g.`, never the framing. If a lesson only fires for the exact situation
  that produced it, it is **overfit** — widen it until a different instance of the same problem
  still matches.
- Write for **agent retrieval** (keywords, "if X then Y" rules, structured lists), not human
  prose — same content-style requirements as `/grow` Step 3.

Do **not** restate `/grow`'s steps here — read them from `flows/maintainer/grow`.

> **Raw incident-bound lessons must NOT be shipped un-generalized.** A lesson full of
> one-off names, IPs, or "that time X broke" framing is noise upstream. Generalize first.

### Step 2: Stage the generalized lessons to a temp dir

Write the generalized lessons as `<id>.md` files into a scratch staging directory
(e.g. `mktemp -d`). This staging dir — not the live memory store — is what you contribute, so
the originals in your store are untouched and you ship only the reviewed, generalized forms.

### Step 3: Ship via the CLI

```
detritus --contribute --from <staging-dir> [--repo owner/name] [--dir path] [--dry-run]
```

The CLI resolves the target repo (`--repo`, else the current repo via `gh repo view`), creates
a fresh `lessons-contribution-<timestamp>` branch, writes `<dir>/<id>.md` for every staged
lesson **verbatim**, commits, pushes, and opens a PR with `gh`.

Run with `--dry-run` first to see exactly which lessons would ship and where the PR would land,
without touching git or GitHub.

### Step 4: The PR review is the gate

The PR goes through the same review loop as every other PR. That review — not any code in the
CLI — is where lessons are corrected, curated, or rejected.

---

## Flags

| Flag | Meaning | Default |
| --- | --- | --- |
| `--from dir` | Local source dir of staged lesson `*.md` files (where the /grow step dropped generalized lessons) | the memory lesson store (`memory.LessonsDir()`) |
| `--repo owner/name` | Target repo for the contribution PR | current repo (`gh repo view`) |
| `--dir path` | Directory **in the target repo** to write lessons into | `lessons` |
| `--dry-run` | Print the resolved repo, branch, file list, and PR title/body; make **no** git/gh changes | off |

`--from` is a local source directory, so an absolute path is allowed. `--dir` is an in-repo
target path and must stay relative (no `..`, no absolute). If `--from` does not exist or holds
no `*.md`, the run is a no-op — it prints a nothing-to-contribute message and opens no PR.

---

## The CLI transports; generalization is the /grow step

The division of labour is deliberate and settled:

- **Generalization is a model/flow step**, not a Go function. detritus has no in-Go LLM; the
  generalization from raw incident to reusable principle happens the same way `/grow` does it —
  the agent, following the flow. Doing it in code would mean baking an LLM into the CLI.
- **The CLI is a dumb, safe transport.** It reads staged files and opens a PR. No filtering, no
  scrubbing, no eligibility logic.
- **The PR review loop is the filter** for these PRs as for all others.
- **Cross-user curation** of `lessons/` into `docs/` by a central "janitor" is a **future
  step**, deliberately out of scope here.
