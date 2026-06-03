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
3. If it updated, the new binary has already re-run `--setup`. Surface the new version from the command output.
4. **Invalidate the cached bootstrap binary.** The detritus repo's project-level `.mcp.json` runs `scripts/detritus-mcp.js`, which downloads the binary into the cache and re-downloads **only when that file is missing — it never version-checks**. So `detritus --update` replaces the installed binary but leaves this cache stale, and any MCP client whose `cwd` is the detritus repo keeps serving the old version. Remove it so the bootstrap re-fetches on next launch (harmless if absent):
   ```bash
   rm -f "${DETRITUS_CACHE_DIR:-$HOME/.cache}/detritus-codex/detritus"
   ```
   Cache location per platform: Linux `$DETRITUS_CACHE_DIR` or `~/.cache/detritus-codex/detritus`; macOS `~/Library/Caches/detritus-codex/detritus`; Windows `%LOCALAPPDATA%\detritus-codex\detritus.exe`.
5. Tell the user to restart their MCP client (IDE / Claude Code) so it picks up the new binary — and, inside the detritus repo, re-fetches the bootstrap binary cleared in step 4. The running MCP server keeps the old binary loaded in memory until then.

## Symptom: MCP still serving the old version after an update

If a `kb_get`/`kb_list` call returns "not found" for a doc you know exists in this release (e.g. a newly added `meta/*` doc), or the MCP otherwise behaves like an older version right after `detritus --update`, the cause is almost always the stale cached bootstrap binary from step 4. Confirm with `"${DETRITUS_CACHE_DIR:-$HOME/.cache}/detritus-codex/detritus" --version`, then clear the cache and restart the client.

## Don't

- Don't try to download release assets manually — `detritus --update` already does that.
- Don't run `detritus --setup` separately — `--update` chains it.
- Don't rebuild from source (`go build`) — this skill is for released binaries, not local development.
- Don't pass `--dry-run` unless the user asks for a preview.
