# Detritus Development Notes

This file is for maintainers. Keep `README.md` focused on people who only need
to install and use Detritus.

## Documentation Boundaries

`README.md` is the public user guide. It should cover:

- The human install prompt: ask an AI coding tool to install the MCP from the
  GitHub repository.
- User-facing capabilities and top-level routers.
- Simple command descriptions that do not require knowing the internal workflow
  docs.

Shell install commands, update commands, pack CLI commands, raw MCP tool names,
sub-skill families, schema details, and routing mechanics belong in
`INSTALL.md`, this file, or the relevant `docs/meta/*` knowledge documents. Do
not expand `README.md` with implementation entry points.

## Current Implementation Surface

The MCP knowledge tools are registered in `main.go`:

- `kb_list`
- `kb_get`
- `kb_search`
- `kb_sections`

The MCP workspace-pack tools are registered in `internal/code/tools.go`:

- `code_pack`
- `code_list`
- `code_tree`
- `code_search`
- `code_outline`
- `code_get`

These MCP tool names are implementation details. README should describe the
capability ("Detritus guidance", "workspace packs", "/code") rather than naming
these tools.

## User-Facing Routers

`detritus --setup` generates Codex skills from every Markdown file under
`docs/`. The public README may name broad user commands that fit the human guide:

- `/code`
- `/gh`
- `/todo`
- `/truthseeker`
- `/plan`
- `/testing`
- `/vibe`
- `/janitor`
- `/smith`

Specialist guidance commands are installed, but they should stay out of the
human README unless the README grows an advanced section:

- `/coding-style`
- `/go-modern`
- `/line-of-sight`

The plugin command shims currently bundled in `commands/` and
`plugins/detritus/commands/` are:

- `/truthseeker`
- `/plan`
- `/testing`
- `/coding-style`
- `/go-modern`
- `/line-of-sight`
- `/vibe`
- `/janitor`
- `/smith`
- `/grow`
- `/optimize`

`/grow` and `/optimize` are maintainer commands. They are installed so maintainers
can improve and tune the Detritus knowledge base, but they should not be promoted
in the README user command table.

## Internal Router Families

`/gh`, `/todo`, and `/code` are user-facing routers. Their routed sub-skills and
backing MCP tools are not user-facing documentation surface.

README may list:

- `/gh` as the GitHub issue/PR workflow entry point.
- `/todo` as the persistent todo workflow entry point.
- `/code` as the workspace-pack entry point.

README should not list `/gh-issue-work`, `/gh-feedback-work`, `/gh-self-review`,
`/gh-pr`, `/todo-add`, `/todo-work`, `code_search`, `code_get`, or similar
internal family members as public user commands.

If a future change promotes a sub-skill into a real user command, update both:

- `README.md`, only for the user-facing entry point.
- This file, with the implementation and routing details.

## Workspace Pack Entry Points

Workspace pack behavior is exposed in three places:

- The `/code` router in `docs/code/code.md`.
- MCP tools in `internal/code/tools.go`.
- CLI flags in `main.go`: `--pack`, `--packs`, `--refresh`, and `--unpack`.

README should document `/code` as the user-facing workspace-pack command. Pack
CLI flags live in `INSTALL.md`; `code_*` MCP tools stay in this maintainer
document.

## Generated Knowledge Data

`main.go` embeds `docs/` and `generated/data.gob`.

When any file under `docs/` changes, run:

```bash
go generate ./...
```

The generator in `cmd/generate/` parses every Markdown file under `docs/`, builds
the search index, and writes `generated/data.gob`.

Changes to `README.md` or this file do not require regenerating the knowledge
data because they are not under `docs/`.

## Local Verification

Before sending documentation changes:

```bash
go test ./...
```

Before sending knowledge-base changes under `docs/`:

```bash
go generate ./...
go test ./...
```

Before releasing a new binary:

```bash
go generate ./...
go test ./...
go build -o detritus .
```
