---
description: Update detritus to the latest released version by running `detritus --update`.
category: setup
triggers:
  - detritus-update
  - detritus update
  - detritus upgrade
  - update detritus
  - upgrade detritus
  - latest detritus
  - latest detritus release
  - mcp serving old version
  - kb_get not found after update
  - stale cached binary
  - detritus-codex cache
when: User asks to update or upgrade detritus, to pull the latest detritus release, or reports the MCP server still serving an old version (e.g. a known kb doc returns "not found") after an update.
---

# /detritus-update — Update to the latest release

Run `detritus --update` via the Bash tool. That's it.

The binary handles everything: checks the latest release on GitHub, downloads the matching asset for the current OS/arch, atomically replaces the running binary, and re-runs `--setup` to refresh MCP configs and skills.

## Steps

1. Run `detritus --update` via Bash. Stream its output to the user.
2. If the command reports "Already up to date", stop. Nothing else to do.
3. If it updated, the new binary has already re-run `--setup`, which also clears the cached bootstrap binary the detritus repo's project-level `.mcp.json` uses. (That bootstrap, `scripts/detritus-mcp.js`, re-downloads **only when the cached file is missing — it never version-checks**, so clearing it during `--setup` is what lets an update actually reach an MCP client whose `cwd` is the detritus repo.) You don't need to clear anything by hand. Surface the new version from the command output.
4. Tell the user to restart their MCP client (IDE / Claude Code) so it picks up the new binary — and, inside the detritus repo, re-fetches the freshly-cleared bootstrap binary. The running MCP server keeps the old binary loaded in memory until then.

## Symptom: MCP still serving the old version after an update

If, after updating and restarting, a `kb_get`/`kb_list` call still returns "not found" for a doc you know exists in this release (e.g. a newly added `meta/*` doc), the cached bootstrap binary is stale and wasn't cleared — e.g. the running binary predates automatic invalidation, or `DETRITUS_CACHE_DIR` points somewhere `--setup` didn't expect. Confirm with `"${DETRITUS_CACHE_DIR:-$HOME/.cache}/detritus-codex/detritus" --version`, then clear it manually and restart the client:

```bash
rm -f "${DETRITUS_CACHE_DIR:-$HOME/.cache}/detritus-codex/detritus"
```

Cache location per platform: Linux `$DETRITUS_CACHE_DIR` or `~/.cache/detritus-codex/detritus`; macOS `~/Library/Caches/detritus-codex/detritus`; Windows `%LOCALAPPDATA%\detritus-codex\detritus.exe`.

## Don't

- Don't try to download release assets manually — `detritus --update` already does that.
- Don't run `detritus --setup` separately — `--update` chains it.
- Don't rebuild from source (`go build`) — this skill is for released binaries, not local development.
- Don't pass `--dry-run` unless the user asks for a preview.
