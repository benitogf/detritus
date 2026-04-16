---
description: Analyze your workflow and generate personalized Claude Code rules, hooks, and settings
category: setup
triggers:
  - setup superpowers
  - configure claude
  - personalize settings
  - setup hooks
  - setup rules
  - global settings
---

# Claude Code Superpowers Setup

Generate a personalized Claude Code configuration by analyzing the user's actual environment, projects, and working patterns — not hardcoded templates.

## Phase 0: Platform Detection

Before anything else, detect the OS. Run `uname -s 2>/dev/null || echo Windows` to determine the platform.

- **Linux/macOS/WSL**: Hook scripts use bash (`.sh`)
- **Native Windows** (no WSL/Git Bash): Hook scripts use PowerShell (`.ps1`)

Detect WSL by checking for `/proc/version` containing "microsoft" — WSL counts as Linux for hook purposes.

Also check if `~/.claude/settings.json` exists and is valid JSON. If it exists but fails to parse, **fix it first** before proceeding:
- If empty or corrupt, replace with `{"permissions":{"allow":[],"deny":[]}}`
- If it has a structure error (e.g. missing arrays), repair the specific field

## Phase 1: Discovery

Gather facts before generating anything. Run these checks silently (don't dump raw output to user):

1. **Projects**: List directories in the home folder containing source code. Look for `go.mod`, `package.json`, `Cargo.toml`, `pyproject.toml`, `project.godot`, `requirements.txt`, `Makefile`, `CMakeLists.txt`, `*.sln`.
2. **Languages & versions**: Run `go version`, `node --version`, `python3 --version`, `rustc --version`, `dotnet --version` etc. — only for languages found in step 1.
3. **Formatters & linters**: Check which are installed: `gofmt`, `prettier`, `black`, `rustfmt`, `clang-format`, `eslint`, `golangci-lint`, `go vet`.
4. **Infrastructure tools**: `docker`, `ssh`, `kubectl`, `terraform`, `ansible` — note which are available.
5. **Git patterns**: Sample 3-5 active projects, check `git log --oneline -10` for commit style, branch naming, activity level.
6. **Existing config**: Read `~/.claude/settings.json`, list `~/.claude/rules/*`, `~/.claude/hooks/*`, `~/.claude/skills/*` to see what's already configured. Do NOT overwrite existing user files.
7. **Build patterns**: Look for Makefiles, docker-compose files, CI configs, deploy scripts across projects.

## Phase 2: Generate Personalized Configuration

Based on discovery, create or update the following. Prefix ALL generated filenames with `detritus-` so they're identifiable and won't conflict with user files.

### Rules (`~/.claude/rules/detritus-*.md`)

Generate rules ONLY for languages/frameworks actually found:

- **Language style rules**: One file per primary language detected. Include naming conventions, error handling patterns, import conventions, and testing patterns specific to that language. Use path-scoping (`paths:` frontmatter) so they only load when editing relevant files.
- **Workflow rules**: Build/test/deploy commands specific to their actual project structure.
- **Communication rules**: Match directness and detail level to the user's apparent expertise (infer from code complexity, git history, toolchain sophistication).
- **Show-reasoning rule**: Always include — makes Claude cite `[rule: file#section]` when rules shape decisions.
- **Update-context rule**: Always include — keeps CLAUDE.md current after changes.

### Hooks (merge into `settings.json`)

Only generate hooks for tools confirmed installed.

#### Hook types to generate:

- **Auto-format** (PostToolUse, matcher: `Edit|Write`): Run the detected formatter after file edits. gofmt for Go, prettier for JS/TS, black for Python, rustfmt for Rust, etc. Script must check file extension before formatting.
- **Session context** (SessionStart): Inject working directory, branch, language versions, running containers — but only for tools that are installed.
- **Task completion** (Stop): Prompt-based hook that verifies tests ran and build was checked for code changes. Adapt the check to the user's actual test/build commands.
- **Context preservation** (PreCompact): If the user has infrastructure (servers, deploy targets), preserve those details during compaction. Ask the user before including any IPs, hostnames, or credentials.

#### Platform-specific hook scripts:

**Linux/macOS/WSL** — generate `~/.claude/hooks/detritus-*.sh` (bash):
- Use `sed` not `grep -P` (macOS has no PCRE grep)
- Use POSIX test `[ ]` syntax where possible
- Use `2>/dev/null` on optional tools
- Make scripts executable (`chmod +x`)
- Reference in settings.json as: `"command": "bash ~/.claude/hooks/detritus-gofmt.sh"`

**Native Windows** — generate `~/.claude/hooks/detritus-*.ps1` (PowerShell):
- Use `$input | ConvertFrom-Json` to parse hook event JSON from stdin
- Use `Where-Object`, `Select-String` for filtering
- Use `2>$null` or `-ErrorAction SilentlyContinue` on optional tools
- Reference in settings.json as: `"command": "powershell -ExecutionPolicy Bypass -File ~/.claude/hooks/detritus-gofmt.ps1"`

Example gofmt hook for PowerShell:
```powershell
$event = $input | ConvertFrom-Json
$filePath = $event.tool_input.file_path
if ($filePath -and $filePath.EndsWith('.go') -and (Test-Path $filePath)) {
    gofmt -w $filePath 2>$null
}
```

Example session context hook for PowerShell:
```powershell
$branch = git rev-parse --abbrev-ref HEAD 2>$null
if (-not $branch) { $branch = "no-git" }
$goVer = (go version 2>$null) -replace 'go version ',''
if (-not $goVer) { $goVer = "not found" }
Write-Output "{`"hookSpecificOutput`":{`"additionalContext`":`"Session environment:\\n- Working dir: $PWD\\n- Branch: $branch\\n- Go: $goVer`"}}"
```

#### When merging into `settings.json`:
- Read existing file, parse as JSON
- If the file is empty, missing, or invalid, create a valid base: `{"permissions":{"allow":[],"deny":[]}}`
- Only replace entries containing `detritus-` in commands or `DETRITUS` in prompts
- Preserve ALL user-defined hooks
- Write back with proper JSON formatting

### Skills

- **parallel-review**: Adapt to detected languages — run the right test/lint commands for each language found.
- **plan**: Include dynamic git context injection (`` !`command` `` syntax) to show current branch, uncommitted changes, recent commits.

### Settings (`~/.claude/settings.json`)

Merge the following into settings.json, preserving all existing user entries:

#### Deny list (`permissions.deny`)

Always add these destructive command blocks. Merge with any existing deny entries — never remove user-defined ones:

```json
"deny": [
  "Bash(rm -rf /)", "Bash(rm -rf /*)", "Bash(rm -rf ~)", "Bash(rm -rf ~/*)",
  "Bash(rm -rf .)", "Bash(rm -rf ./*)", "Bash(rm -rf .git)",
  "Bash(sudo rm -rf /)", "Bash(sudo rm -rf /*)", "Bash(sudo rm -rf ~)",
  "Bash(git push --force origin main)", "Bash(git push --force origin master)",
  "Bash(git push -f origin main)", "Bash(git push -f origin master)",
  "Bash(git push --force-with-lease origin main)", "Bash(git push --force-with-lease origin master)",
  "Bash(git reset --hard)", "Bash(git checkout -- .)",
  "Bash(git clean -fd)", "Bash(git clean -fdx)", "Bash(git clean -ffdx)",
  "Bash(git branch -D main)", "Bash(git branch -D master)",
  "Bash(git stash drop)", "Bash(git stash clear)",
  "Bash(git reflog expire --expire=now --all)", "Bash(git gc --prune=now)",
  "Bash(> /dev/sda*)", "Bash(dd if=/dev/zero*)", "Bash(dd if=/dev/random*)", "Bash(dd if=/dev/urandom*)",
  "Bash(mkfs.*)", "Bash(fdisk*)", "Bash(parted*)",
  "Bash(chmod -R 777 /)", "Bash(chmod -R 777 /*)", "Bash(chmod 000 /)",
  "Bash(chown -R root:root /home)", "Bash(chown -R:*)",
  "Bash(docker system prune -a)", "Bash(docker volume prune)",
  "Bash(docker rm -f $(docker ps -aq))", "Bash(docker stop $(docker ps -aq))",
  "Bash(docker rmi -f $(docker images -aq))",
  "Bash(systemctl stop sshd)", "Bash(systemctl disable sshd)",
  "Bash(systemctl stop ssh)", "Bash(systemctl disable ssh)",
  "Bash(systemctl stop networking)", "Bash(systemctl stop NetworkManager)",
  "Bash(iptables -F)", "Bash(iptables -P INPUT DROP)", "Bash(ufw deny incoming)",
  "Bash(passwd *)", "Bash(usermod *)", "Bash(userdel *)", "Bash(deluser *)", "Bash(visudo*)",
  "Bash(shutdown*)", "Bash(reboot*)", "Bash(init 0*)", "Bash(halt*)", "Bash(poweroff*)",
  "Bash(crontab -r)", "Bash(> /var/log/*)", "Bash(truncate -s 0 /var/log/*)", "Bash(shred *)",
  "Bash(curl * | bash)", "Bash(curl * | sh)", "Bash(wget * | bash)", "Bash(wget * | sh)",
  "Bash(cat /etc/shadow)", "Bash(cat ~/.ssh/id_*)",
  "Bash(Remove-Item -Recurse -Force C:\\*)", "Bash(Remove-Item -Recurse -Force ~\\*)",
  "Bash(Format-Volume*)", "Bash(Stop-Service sshd)", "Bash(Stop-Computer*)",
  "Bash(Restart-Computer*)"
]
```

#### Status line

Always add a status line showing current directory and git branch. Do NOT overwrite if the user already has a custom `statusLine` configured.

**Linux/macOS/WSL:**
```json
"statusLine": {
  "type": "command",
  "command": "branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo 'no-git'); dir=$(basename \"$PWD\"); echo \"$dir [$branch]\"",
  "refreshInterval": 30
}
```

**Native Windows:**
```json
"statusLine": {
  "type": "command",
  "command": "powershell -Command \"$b = git rev-parse --abbrev-ref HEAD 2>$null; if(-not $b){$b='no-git'}; $d = Split-Path -Leaf (Get-Location); Write-Output \\\"$d [$b]\\\"\"",
  "refreshInterval": 30
}
```

#### Effort and thinking

```json
"effortLevel": "high",
"alwaysThinkingEnabled": true,
"showThinkingSummaries": true
```

#### Auto mode environment

Add `autoMode.environment` entries describing the user's actual setup based on discovery. Populate based on what was actually found — languages, build tools, infrastructure. Do NOT include credentials or IPs without explicit user approval.

## Phase 3: Report

After generating, show the user a concise summary:

1. **What was created** — list each file and one-line purpose
2. **Key features** — how to use `/parallel-review`, `/loop` for monitoring, `/plan` with live context
3. **What to customize** — suggest areas they might want to tweak

## Important Constraints

- NEVER overwrite files that don't start with `detritus-` (except settings.json merge)
- NEVER add credentials, IPs, or secrets without explicit user approval
- NEVER generate rules for languages/tools not actually found on the system
- Ask the user to confirm before writing to `settings.json`
- If the user already has extensive config, focus on gaps — don't regenerate what exists
