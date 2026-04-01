---
name: coding-style
description: Coding style conventions — naming, error handling, formatting, commit messages, PR structure.
user-invocable: false
---

# Coding Style

For full reference, call `kb_get(name="patterns/coding-style")` if the detritus MCP server is available.

## Key Conventions

- **Naming**: clear, descriptive names; avoid abbreviations except well-known ones (ctx, err, req, resp)
- **Error handling**: return errors up the stack; wrap with context using `fmt.Errorf("doing X: %w", err)`
- **Formatting**: `gofmt`/`goimports` — no debate
- **Commits**: imperative mood, short summary line, body explains *why*
- **PRs**: one concern per PR; tests accompany functional changes
