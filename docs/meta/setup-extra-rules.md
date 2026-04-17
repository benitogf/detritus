---
description: Generate personalized Claude Code rule files and hook scripts based on the user's actual environment. Companion to setup-superpowers (which handles settings.json only).
category: setup
triggers:
  - setup extra rules
  - generate rules
  - generate hooks
  - personalize rules
  - personalize hooks
---

# Extra Rules & Hooks Generator

Generate personalized rule files (`~/.claude/rules/detritus-*.md`) and hook scripts (`~/.claude/hooks/detritus-*`) based on the user's actual environment, projects, and tooling.

This skill is **opt-in** — the user must invoke it explicitly. It does NOT touch `settings.json` deny lists, status line, effort, or autoMode environment (run `setup-superpowers` for those).

It is also **re-runnable** — running it again should detect existing `detritus-*` files/entries and update them in place rather than duplicating. Existing user files (without the `detritus-` prefix) are never touched.

## Phase 0: Platform Detection

Detect the OS first: `uname -s 2>/dev/null || echo Windows`.

- **Linux/macOS/WSL**: hook scripts use bash (`.sh`)
- **Native Windows** (no WSL/Git Bash): hook scripts use PowerShell (`.ps1`)
- WSL counts as Linux (check `/proc/version` for "microsoft")

If `~/.claude/settings.json` exists but is invalid JSON, fix it before proceeding (see setup-superpowers for repair logic).

## Phase 1: Discovery

Run silently. Don't dump raw output to the user.

1. **Projects**: List home directories containing `go.mod`, `package.json`, `Cargo.toml`, `pyproject.toml`, `project.godot`, `requirements.txt`, `Makefile`, `CMakeLists.txt`, `*.sln`.
2. **Languages & versions**: Run `go version`, `node --version`, `python3 --version`, `rustc --version`, `dotnet --version` — only for languages found in step 1.
3. **Formatters & linters**: Check `gofmt`, `prettier`, `black`, `rustfmt`, `clang-format`, `eslint`, `golangci-lint`, `go vet`.
4. **Infrastructure tools**: `docker`, `ssh`, `kubectl`, `terraform`, `ansible`.
5. **Git patterns**: Sample 3-5 active projects, check `git log --oneline -10` for commit style.
6. **Existing config**: List `~/.claude/rules/*` and `~/.claude/hooks/*` to see what's already configured.
7. **Build patterns**: Look for Makefiles, docker-compose files, CI configs, deploy scripts.

## Phase 2: Rules (`~/.claude/rules/detritus-*.md`)

Generate rules ONLY for languages/frameworks actually found. Overwrite existing `detritus-*.md` files; never touch other rule files.

- **Language style rules**: One file per primary language detected. Naming conventions, error handling, imports, testing patterns. Use path-scoping (`paths:` frontmatter) so they only load when editing relevant files.
- **Workflow rules**: Build/test/deploy commands specific to actual project structure.
- **Communication rules**: Match directness/detail to apparent user expertise (infer from code complexity, git history, toolchain sophistication).
- **Show-reasoning rule**: Makes Claude cite `[rule: file#section]` when rules shape decisions.
- **Update-context rule**: Keeps CLAUDE.md current after changes.

## Phase 3: Hooks (`~/.claude/hooks/detritus-*` + register in `settings.json`)

Generate hooks ONLY for tools confirmed installed.

### Hook types

- **Auto-format** (PostToolUse, matcher: `Edit|Write`): gofmt for Go, prettier for JS/TS, black for Python, rustfmt for Rust. Script must check file extension before formatting.
- **Session context** (SessionStart): Inject working dir, branch, language versions, running containers — only for installed tools.
- **Task completion** (Stop): Prompt-based hook verifying tests ran and build was checked for code changes.
- **Context preservation** (PreCompact): If user has infrastructure (servers, deploy targets), preserve those details. Ask before including IPs/hostnames/credentials.

### Platform-specific scripts

**Linux/macOS/WSL** — `~/.claude/hooks/detritus-*.sh` (bash):
- Use `sed` not `grep -P` (macOS has no PCRE grep)
- Use POSIX test `[ ]` syntax
- Use `2>/dev/null` on optional tools
- Make executable (`chmod +x`)
- Reference in settings.json: `"command": "bash ~/.claude/hooks/detritus-gofmt.sh"`

**Native Windows** — `~/.claude/hooks/detritus-*.ps1` (PowerShell):
- Use `$input | ConvertFrom-Json` to parse hook event JSON
- Use `Where-Object`, `Select-String` for filtering
- Use `2>$null` or `-ErrorAction SilentlyContinue` on optional tools
- Reference in settings.json: `"command": "powershell -ExecutionPolicy Bypass -File ~/.claude/hooks/detritus-gofmt.ps1"`

Example gofmt PowerShell:
```powershell
$event = $input | ConvertFrom-Json
$filePath = $event.tool_input.file_path
if ($filePath -and $filePath.EndsWith('.go') -and (Test-Path $filePath)) {
    gofmt -w $filePath 2>$null
}
```

Example session context PowerShell:
```powershell
$branch = git rev-parse --abbrev-ref HEAD 2>$null
if (-not $branch) { $branch = "no-git" }
$goVer = (go version 2>$null) -replace 'go version ',''
if (-not $goVer) { $goVer = "not found" }
Write-Output "{`"hookSpecificOutput`":{`"additionalContext`":`"Session environment:\\n- Working dir: $PWD\\n- Branch: $branch\\n- Go: $goVer`"}}"
```

### Registering hooks in settings.json

This is the ONLY settings.json change this skill makes. Apply with care:

- Read the file, parse as JSON. If invalid/missing, create `{"permissions":{"allow":[],"deny":[]}}` first.
- **Orphan sweep first**: scan all existing hook entries that reference `~/.claude/hooks/detritus-*` scripts. If the referenced script no longer exists on disk, remove that entry. This heals settings.json after the user manually deletes a hook script.
- For each generated hook, check whether an entry referencing the same `detritus-*` script already exists.
  - If absent: append it.
  - If present: leave it (the script content was overwritten on disk; the entry doesn't need rewriting).
- Never touch hook entries not referencing `detritus-*` scripts.
- Write back with proper JSON formatting.

For a full removal of all detritus-generated rules and hooks (including settings entries), use the `cleanup-extra-rules` skill.

## Phase 4: Report

Concise summary:

1. **What was created** — list each rule file and hook script with one-line purpose
2. **What was skipped** — languages/tools not detected, so no rule/hook generated
3. **What to customize** — areas the user might want to tweak

## Important Constraints

- NEVER overwrite files that don't start with `detritus-` (settings.json hook-array merge is the only exception, and only for `detritus-*` entries)
- NEVER add credentials, IPs, or secrets without explicit user approval
- NEVER generate rules or hooks for languages/tools not actually found on the system
- NEVER touch `permissions`, `statusLine`, `effortLevel`, `autoMode`, or any non-hook settings entries — those belong to `setup-superpowers`
- Skill must be **re-runnable**: detect existing `detritus-*` files/entries and update in place; never duplicate
