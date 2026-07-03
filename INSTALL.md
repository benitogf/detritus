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
