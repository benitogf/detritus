---
description: Cut a release of detritus and/or candyland (bump → annotated tag → CI build), then update the local installation.
triggers:
  - detritus-release
  - release detritus
  - release candyland
  - release both
  - bump tag release
  - cut a release
  - tag and release
  - ship a release
  - release and update my install
when: User asks to bump/tag/release detritus and/or candyland and (usually) update their local install afterward. Canonical phrasing — "please bump, tag, release both candyland and detritus and when the release is done update my local installation".
related:
  - flows/maintainer/detritus-update
  - flows/github/gh
---

# /detritus-release — Bump, tag, release, then update the local install

Both repos release the same way: **push an annotated `v*` tag** and GitHub Actions
builds and publishes the assets. **The version comes from the tag** — there is no
version file to edit (detritus injects it via goreleaser `-X main.version={{.Version}}`;
candyland reads `CANDYLAND_VERSION=${{ github.ref_name }}` in its Bazel build). So a
release is: pick the next tag, tag, push, watch CI, then refresh the local install.

Release **each repo that has merged, unreleased work**. They are independent tags —
release one or both as applicable.

## ⛔ Tag only after ALL intended PRs are merged to `main`

The tag captures whatever commit `main` points at **now**. A tag cut before a later
merge silently omits that work (a real incident: a candyland tag cut right after one
PR merged excluded two MCP PRs that merged minutes later). Before tagging:

1. `git checkout main && git pull` in the repo.
2. Confirm `git log --oneline -n 15 origin/main` contains every PR you intend to ship.
3. Confirm there are **no open PRs** you meant to include (`gh pr list`), and the tree is clean.

## Steps (per repo)

1. **Pick the next version.** Read the latest tag: `git tag --sort=-v:refname | head -1`.
   Bump **minor** for features (the common case), **patch** for fix-only releases,
   **major** only on a breaking change. detritus is `vX.Y.Z`; candyland is pre-1.0
   (`v0.Y.Z`). Do not edit any version file — the tag is the source of truth.
2. **Tag + push.** Annotated tag with a short changelog derived from the merged PRs:
   ```bash
   git tag -a vX.Y.Z -m "vX.Y.Z

   - <one line per shipped PR>"
   git push origin vX.Y.Z
   ```
   The push triggers the repo's release workflow (`on: push tags ['v*']`).
3. **Watch CI to completion.** `gh run watch <run-id> --repo benitogf/<repo> --exit-status`
   (find the run: `gh run list --repo benitogf/<repo> --workflow=<release|release.yml> --limit 1`).
   - **detritus**: goreleaser → multi-arch `detritus_<os>_<arch>.tar.gz`/`.zip` + `checksums.txt`. Fast (~1–2 min).
   - **candyland**: Bazel + Zig hermetic toolchain → raw per-platform `candyland-<os>-<arch>[.exe]`. **Slower** (several minutes) — don't assume it failed because it's not instant.
4. **Confirm assets published.** `gh release view vX.Y.Z --repo benitogf/<repo> --json assets --jq '.assets[].name'`.

## Update the local installation (after the releases are live)

1. `detritus --update` — self-updates detritus to the latest release and, **when it
   actually updated**, chains `--setup`, which re-fetches the **latest** candyland
   sidecar binary beside detritus. Stream output; surface the new version.
2. **If `--update` prints "Already up to date"** (detritus itself didn't change) **but
   candyland was released**, `--setup` did NOT run — refresh the sidecar explicitly:
   ```bash
   detritus --setup
   ```
   `--setup` unconditionally downloads the latest candyland release and overwrites the
   local `candyland` binary, so this is how a candyland-only release reaches the install.
3. **MCP client refresh.** The running MCP server keeps the old binary in memory —
   tell the user to restart their IDE / Claude Code. If a `kb_get` for a doc you know
   is in this release still 404s after restart, the cached bootstrap binary is stale;
   clear it and restart (see `flows/maintainer/detritus-update` → *Symptom*):
   ```bash
   rm -f "${DETRITUS_CACHE_DIR:-$HOME/.cache}/detritus-codex/detritus"
   ```

## Report

Per repo: the new version, the release URL, and CI result. Then the local-install
outcome (detritus version now installed, candyland sidecar version, and "restart your
MCP client" if detritus changed).

## Don't

- Don't edit `package.json`/`main.go` version strings — the tag is the version.
- Don't tag before every intended PR is merged (see the ⛔ rule above).
- Don't hand-build or hand-upload assets — the tagged CI workflow does it.
- Don't skip watching candyland's build just because it's slow — confirm it went green and published assets before updating the local install.
- Don't run `detritus --setup` as a *substitute* for `--update` when detritus itself has a new release; `--update` chains it. Run `--setup` alone only to pull a candyland-only release onto an already-current detritus.
