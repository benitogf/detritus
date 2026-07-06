---
description: Ship every locally-distilled lesson into a repo's shared lessons/ directory via a normal PR — a transport gateway, not a gate.
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

# /contribute — Lesson Gateway

Run `detritus --contribute [--repo owner/name] [--dir path] [--dry-run]`.

The gateway is a **vehicle, not a bouncer.** It gathers **every** local lesson
(from the learned-memory store) and ships them into a single shared place — a
`lessons/` directory in the target repo — through the ordinary PR flow. Any
incident is a possible lesson; more data is better, so nothing is filtered on
the way in.

## What it does

1. **Gather all lessons.** Reads every `<id>.md` under the local memory store
   with **no eligibility filter** — active or stale, confirmed or not, expired
   or not. The maturity fields (`status`, `confirmed`, `validity`, …) travel
   inside each file **as data** for downstream curation; they are never a
   promotion gate here.
2. **Redact obvious secrets (transport hygiene, best-effort).** A conservative
   redactor masks common credential formats — API-key/token prefixes,
   `Authorization`/`Bearer` headers, bare JWTs, `password:`/`api_key=`
   assignments, connection-string passwords, AWS secret keys, and PEM
   private-key blocks — before writing. Coverage is **best-effort, not
   exhaustive**: it masks the credential span only, **never drops a lesson**,
   and does not touch ordinary prose or code. The **PR review is the gate**, not
   this redactor.
3. **Open a PR.** Resolves the target repo (`--repo`, else the current repo via
   `gh repo view`), creates a fresh `lessons-contribution-<timestamp>` branch,
   writes `<dir>/<id>.md` for every lesson, commits, pushes, and opens a PR with
   `gh`.

## The PR review is the only gate

There is no threshold, quorum, vote, or LLM generalization in the gateway. Every
lesson ships raw; the human reviewing the PR decides what to accept, reject, or
curate. Cross-user curation of `lessons/` into `docs/` by a central "janitor" is
a **future step**, deliberately out of scope here.

## Flags

| Flag | Meaning | Default |
| --- | --- | --- |
| `--repo owner/name` | Target repo for the contribution PR | current repo (`gh repo view`) |
| `--dir path` | Directory in the target repo to write lessons into | `lessons` |
| `--dry-run` | Print the resolved repo, branch, file list, and PR title/body; make **no** git/gh changes | off |

Use `--dry-run` first to see exactly which lessons would ship and where the PR
would land, without touching git or GitHub.
