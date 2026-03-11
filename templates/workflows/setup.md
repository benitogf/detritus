---
description: Install or update detritus MCP knowledge base server
---

# Setup Detritus MCP Server

Detect the user's OS before proceeding. Run ONLY the commands matching their platform.

## Step 1: Install the binary

### Linux / macOS

// turbo
```bash
curl -sSL https://raw.githubusercontent.com/benitogf/detritus/main/install.sh | sh
```

### Windows (PowerShell, no WSL)

// turbo
```powershell
irm https://raw.githubusercontent.com/benitogf/detritus/main/install.ps1 | iex
```

## Step 2: Configure Windsurf MCP

The MCP config file is `~/.codeium/windsurf/mcp_config.json` on all platforms (`~` = `%USERPROFILE%` on Windows).

Read the config file. If it exists, add `"detritus"` to the `"mcpServers"` object. If it doesn't exist, create it.

### Linux / macOS

```json
"detritus": {
  "command": "/usr/local/bin/detritus",
  "args": [],
  "disabled": false
}
```

### Windows

The binary path is `%LOCALAPPDATA%\detritus\detritus.exe`. Resolve it to the absolute path with forward slashes for JSON.

```json
"detritus": {
  "command": "C:/Users/USERNAME/AppData/Local/detritus/detritus.exe",
  "args": [],
  "disabled": false
}
```

Replace `USERNAME` with the actual username from the resolved `%LOCALAPPDATA%` path.

## Step 3: Install project files

Download the template files from the detritus repo and place them in the project root. Only create files that don't already exist (do not overwrite existing ones).

### Linux / macOS

// turbo
```bash
mkdir -p .windsurf/workflows && [ ! -f .windsurfrules ] && curl -sSL https://raw.githubusercontent.com/benitogf/detritus/main/templates/.windsurfrules -o .windsurfrules; for f in setup.md _truthseeker.md plan.md scaffold-simple-service.md; do [ ! -f ".windsurf/workflows/$f" ] && curl -sSL "https://raw.githubusercontent.com/benitogf/detritus/main/templates/workflows/$f" -o ".windsurf/workflows/$f"; done
```

### Windows (PowerShell)

// turbo
```powershell
New-Item -ItemType Directory -Path .windsurf/workflows -Force | Out-Null; if (-not (Test-Path .windsurfrules)) { irm https://raw.githubusercontent.com/benitogf/detritus/main/templates/.windsurfrules | Set-Content .windsurfrules -Encoding UTF8 }; @('setup.md','_truthseeker.md','plan.md','scaffold-simple-service.md') | ForEach-Object { if (-not (Test-Path ".windsurf/workflows/$_")) { irm "https://raw.githubusercontent.com/benitogf/detritus/main/templates/workflows/$_" | Set-Content ".windsurf/workflows/$_" -Encoding UTF8 } }
```

## Step 4: Restart Windsurf

Tell the user to restart Windsurf to load the new MCP server. After restart, the `kb_list`, `kb_get`, and `kb_search` tools will be available.

## Update

To update to the latest version, re-run Step 1.
