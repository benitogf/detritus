---
description: Apply baseline Claude Code settings (deny list, status line, effort/thinking, autoMode environment). Idempotent and re-runnable. For rules and hooks, use setup-extra-rules.
category: setup
triggers:
  - setup superpowers
  - configure claude
  - personalize settings
  - global settings
  - baseline settings
---

# Claude Code Superpowers — Settings

Merge baseline settings into `~/.claude/settings.json`. This skill ONLY touches settings — it does not generate rule files, hook scripts, or modify other skills.

For personalized rules and hooks, run the separate `setup-extra-rules` skill (opt-in).

## Phase 0: Detect OS

Detect the OS first: `uname -s 2>/dev/null || echo Windows`.

- **Linux/macOS/WSL** (POSIX-style): use the bash status-line variant in Phase 4.
- **Native Windows** (no WSL/Git Bash): use the PowerShell status-line variant in Phase 4.
- WSL counts as Linux (check `/proc/version` for "microsoft").

Hold the OS choice in mind — it affects only the `statusLine` variant chosen in Phase 4.

## Phase 1: Validate settings.json

Check that `~/.claude/settings.json` exists and is valid JSON.

- **Missing or empty**: create a minimal stub `{"permissions":{"allow":[],"deny":[]}}` and continue — Phases 3-4 will fill in deny list, status line (correct OS variant from Phase 0), effort/thinking, autoMode.
- **Invalid JSON**: do NOT overwrite. Try to repair only the specific structural error (e.g. trailing comma, missing closing brace). If it's truly unrecoverable, back the file up to `~/.claude/settings.json.broken-<timestamp>` and create a fresh stub as above. Tell the user what was backed up and why.
- **Valid JSON**: proceed straight to Phase 2.

## Phase 2: Merge baseline settings

Apply each section below using the merge semantics in Phase 3. Sections:

- Deny list (`permissions.deny`)
- Status line (`statusLine`) — pick the variant matching the OS detected in Phase 0
- Effort and thinking (`effortLevel`, `alwaysThinkingEnabled`, `showThinkingSummaries`)
- Auto mode environment (`autoMode.environment`)

## Phase 3: Merge semantics — add what's missing, never duplicate

- **Array entries** (`permissions.deny`, `autoMode.environment`): check string equality, append only missing values, never re-append existing ones. Never reorder or deduplicate user-defined entries.
- **Scalar entries** (`effortLevel`, `alwaysThinkingEnabled`, `showThinkingSummaries`, `statusLine`): set only if absent. If the user already has a value, leave it alone.
- **Nested objects**: recurse with the same rules.

## Phase 4: Settings content

### Deny list (`permissions.deny`)

Append any missing entries from this list:

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

### Status line (`statusLine`)

Set only if absent. Pick the platform variant.

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

### Effort and thinking

Set each scalar only if absent:

```json
"effortLevel": "high",
"alwaysThinkingEnabled": true,
"showThinkingSummaries": true
```

### Auto mode environment (`autoMode.environment`)

Append entries describing the user's setup. Detect languages/tools quickly (run `go version`, `node --version`, `docker --version` etc. and only include lines for tools that respond). Example entries to consider:

- "Go build, test, vet commands are always safe"
- "npm install, build, test commands are always safe"
- "Docker compose up/down/ps/logs commands are always safe"

Do NOT include credentials, IPs, or hostnames in `autoMode.environment` without explicit user approval.

## Phase 5: Report

Concise summary:

1. **What was added** — list each settings key/value newly inserted
2. **What was preserved** — note that pre-existing user values were left alone
3. **Next step** — mention `setup-extra-rules` for opt-in rules + hooks generation

## Important Constraints

- Skill is **re-runnable** and must be safely callable on every startup
- NEVER overwrite or reorder user-defined settings entries — only fill in missing ones
- NEVER touch `~/.claude/rules/`, `~/.claude/hooks/`, or other skills — those belong to `setup-extra-rules`
- NEVER include credentials, IPs, or hostnames in `autoMode.environment` without explicit user approval
