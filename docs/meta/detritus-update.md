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
when: User asks to update or upgrade detritus, or to pull the latest detritus release.
---

# /detritus-update — Update to the latest release

Run `detritus --update` via the Bash tool. That's it.

The binary handles everything: checks the latest release on GitHub, downloads the matching asset for the current OS/arch, atomically replaces the running binary, and re-runs `--setup` to refresh MCP configs and skills.

## Steps

1. Run `detritus --update` via Bash. Stream its output to the user.
2. If the command reports "Already up to date", stop. Nothing else to do.
3. If it updated, the new binary has already re-run `--setup`. Surface the new version from the command output and let the user know to restart their MCP client (IDE / Claude Code) so it picks up the new binary.

## Don't

- Don't try to download release assets manually — `detritus --update` already does that.
- Don't run `detritus --setup` separately — `--update` chains it.
- Don't rebuild from source (`go build`) — this skill is for released binaries, not local development.
- Don't pass `--dry-run` unless the user asks for a preview.
