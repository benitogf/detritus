# Manual Install

This file is for AI tools and maintainers that need explicit shell commands.
Human users should usually ask their AI coding tool to install Detritus from the
GitHub repository instead of running these steps themselves.

## Install From A Release

Download the binary for your platform from the
[latest release](https://github.com/benitogf/detritus/releases/latest), unpack
it anywhere, then run it once with `--setup`:

```bash
detritus --setup
```

`detritus --setup` is self-placing: it copies the binary into a stable,
per-user directory on `PATH` (`~/.local/bin` on Linux/macOS,
`%LOCALAPPDATA%\detritus` on Windows) and adds that directory to `PATH` if
needed, then configures every detected assistant app and editor. No install
script is required — the binary bootstraps itself.

## VS Code Copilot Prompts

Setup creates Copilot prompt files for the public Detritus flows, including
`/plan` and `/truthseeker`. After the first setup or an update, run
**Developer: Reload Window** in VS Code so Copilot Chat indexes the new prompt
files. Type `/truthseeker` at the beginning of a Copilot Chat prompt to verify
that it appears in the completion list. VS Code reserves `/plan` for its
built-in Plan agent, so a custom prompt cannot replace that command.

VS Code only completes prompt-file commands at the beginning of the chat input.
Detritus still recognizes a slash command anywhere in a submitted message via
its instruction file, but VS Code cannot offer inline completion for that form.

If it does not appear after a reload, confirm that the active VS Code user
settings enable `~/.copilot/prompts` in `chat.promptFilesLocations`, then run
`detritus --setup` again and reload the window.

## OpenCode Commands

Setup writes every public Detritus flow as an OpenCode slash command under
`~/.config/opencode/commands/` (or `$XDG_CONFIG_HOME/opencode/commands`),
including `/truthseeker`. This path is also used by the native Windows Desktop
app (`%USERPROFILE%\.config\opencode\commands`). OpenCode reads these files
only when its server starts. After setup or an update, restart the OpenCode
session itself; reloading the VS Code window alone does not refresh the commands
held by its running OpenCode server. If `OPENCODE_CONFIG_DIR` is set, setup
uses that directory instead.

## Add The Codex Plugin Marketplace

For Codex plugin workflows, add the Detritus plugin marketplace/source:

```bash
codex plugin marketplace add benitogf/detritus
```

## Install From A Downloaded Binary

Download a binary from the
[latest release](https://github.com/benitogf/detritus/releases/latest), put it on
`PATH`, then run:

```bash
detritus --setup
```

## Update

```bash
detritus --update
```

`detritus --update` downloads the latest release and runs setup with the new
binary.

## Code context

Code context is zero-setup — there is no pack to build or manage. Use `/code`
(or the `code_map` / `code_outline` / `code_graph` MCP tools directly); they
read the working tree live, so there are no install or maintenance commands.
