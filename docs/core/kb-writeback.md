---
description: How a ship-leg flow gets a writable detritus KB checkout and opens a docs-only PR — resolve or provision a clone (fork when needed), edit in a worktree, ship via /gh.
triggers:
  - kb writeback
  - writable checkout
  - no local clone
  - provision clone
  - fork detritus
  - ship a lesson
  - kb pull request
when: Internal. Loaded via kb_get by /grow, /learn, /optimize before drafting a KB change (/absorb inherits it transitively via /grow); no slash command.
related:
  - flows/github/gh
  - flows/github/gh-issue-work
  - flows/maintainer/grow
  - flows/maintainer/learn
  - flows/maintainer/optimize
  - core/completion
---

# KB Writeback — get a writable checkout, ship a docs PR

The detritus KB is public, so a lesson can ship from ANY machine — a pre-existing local clone is NOT
required. This doc owns resolving or provisioning a writable checkout and the docs-only PR recipe.
Ship conventions (body, footer, private-name scrub) compose `flows/github/gh` by reference and are
never restated here.

> ## ⛔ Do not invoke directly
> No slash command. Loaded via `kb_get` by the ship-leg flows — `/grow`, `/learn`, `/optimize`
> directly, `/absorb` transitively via `/grow`. Those docs reference this one; they never restate a
> conflicting checkout or PR recipe.

## Preconditions

Composes `flows/github/gh` → Preconditions: `gh` installed + `gh auth status` green is a HARD
requirement. On failure, STOP hard with the remediation from that section — never a degraded or
partial mode. (Reference it; do not restate the gh-auth check here.)

## Resolve the base checkout (ladder — first match wins)

1. **Local clone in the workspace** — a directory whose git remote URL matches `benitogf/detritus`
   (any remote). Use it as the base. If it is a clone of a fork with no upstream remote, add and fetch
   one: `git -C <base> remote add upstream https://github.com/benitogf/detritus && git -C <base> fetch upstream`.
2. **Cached base clone** at `${DETRITUS_CACHE_DIR:-<platform cache dir>}/detritus-codex/kb` — if
   present, refresh it: `git -C <base> fetch upstream` (or `origin` when the base is a direct upstream
   clone). This base is DURABLE (kept across lessons), not temporary.
3. **Provision it** (no clone anywhere yet): decide fork-vs-direct from write access —
   ```
   gh api repos/benitogf/detritus --jq .permissions.push
   ```
   - `true` (has write access): `git clone https://github.com/benitogf/detritus <base>` — `origin` is
     upstream; branches push to `origin`.
   - `false` (no write access — the common case): `gh repo fork benitogf/detritus --clone=false`
     (idempotent; safe to re-run), then `git clone https://github.com/<login>/detritus <base>`,
     `git -C <base> remote add upstream https://github.com/benitogf/detritus`, and
     `git -C <base> fetch upstream` (the worktree recipe branches off `upstream/main`, which does not
     exist until upstream is fetched). `origin` is the fork (pushable); `upstream` is benitogf/detritus.

Invariant: a PR needs no write access to upstream, but its branch must live in a pushable repo —
upstream (write access) or the user's fork (everyone else). There is no fork-less path for a
no-write-access contributor.

## Ship a lesson (per-writeback recipe)

Each lesson runs in its OWN worktree so the base and the user's checkout stay clean and concurrent
lessons never collide.

1. `git -C <base> worktree add <base>-wt-<slug> -b <branch> upstream/main` (use `origin/main` when the
   base is a direct upstream clone).
2. Edit `docs/` — the usual surface. `generated/` is gitignored and the search index regenerates at
   release — **no Go toolchain is needed** to ship a pure docs change.
   **Doctrine does not live in `docs/` alone** — detritus embeds it in Go string literals (the
   installer's agent/rule definitions and prompts). A lesson that deletes or renames a doc, or
   changes a model other docs or agents state, sweeps the whole reference surface per
   `core/review-rigor` R1 (deleted/renamed entity) and folds every embedded statement; if the sweep
   touches Go strings it is a **code+docs writeback** — run
   `go generate ./... && go build ./... && go test ./...` before pushing. The no-toolchain shortcut
   applies only when the sweep confirms no embedded references exist.
3. Commit (attribution / footer / scrub per `/gh`).
4. Push the branch to the pushable remote (`origin` — fork or upstream depending on the ladder).
5. Open the PR against upstream (`--repo benitogf/detritus --base main`), choosing the `--head` shape
   by write access. **`flows/github/gh-issue-work` Phase 9 owns the exact `--head` syntax** (same-repo
   `<branch>` vs cross-fork `<login>:<branch>`) and the body/conventions — follow it; do not restate it
   here. The fork case here maps to Phase 9's no-write-access shape. **Mint the learning-loop footer.**
   Because this is a KB-writeback PR, append the 📚 provenance footer to the PR body below the
   attribution footer (#1 in `flows/github/gh`):
   ```
   📚 Learned by detritus — <flow> `<ref>`
   ```
   (`<flow>` ∈ `grow` / `learn` / `absorb` / `optimize`; `<ref>` is the source signal — a candyland unit
   id, a PR URL, or a session tag). This is the sole minting site for the footer; `flows/github/gh`
   convention #12 only preserves it on later body rewrites.
6. `git -C <base> worktree remove <base>-wt-<slug>` when the PR is open. The base stays on a clean
   default branch.

## Guardrails

- Never require a pre-existing local clone — resolve or provision one.
- Never a temp clone per lesson — one durable cached base + a worktree per lesson.
- Never dirty the user's own checkout — always work in a worktree.
- Docs-only writeback needs no Go build/test — but only after the repo-wide sweep (recipe step 2)
  confirms the lesson touches no Go-embedded doctrine; a doc deletion/rename or model change that
  hits embedded strings is a code+docs writeback and runs the Go verification.
- Public repo: scrub private names from every issue / PR / branch / commit (per `/gh`).
