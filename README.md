# detritus

Detritus is an MCP knowledge base for AI coding assistants. It gives the
assistant reusable guidance for planning, testing, GitHub work, project todos,
and codebase exploration.

## Install

Ask your AI coding tool to install Detritus from this repository:

```text
please install detritus mcp https://github.com/benitogf/detritus
```

Or, if your tool supports Markdown links:

```text
please install detritus mcp [benitogf/detritus](https://github.com/benitogf/detritus)
```

That is the human install path. You should not need to run Detritus as a CLI
tool yourself. Manual install details for an assistant or maintainer live in
[INSTALL.md](./INSTALL.md).

## Commands

Use Detritus by command name:

| Command | Use it for |
| --- | --- |
| `/plan` | Turn a request into an implementation plan. |
| `/testing` | Choose a testing approach for a feature, bug, or risk. |
| `/truthseeker` | Check claims and assumptions against evidence. |
| `/code` | Use a workspace code index for codebase questions or scoped changes. |
| `/gh` | Work with GitHub issues and pull requests. |
| `/todo` | Manage a persistent project todo list across sessions. |
| `/vibe` | Start from a plain-language product request and drive delivery. |
| `/janitor` | Run recurring maintenance on a project. |
| `/smith` | Run the build loop for a scoped feature. |

The command names are the user surface. Internal tools, routed subcommands, and
maintainer details are intentionally left out of this README.
