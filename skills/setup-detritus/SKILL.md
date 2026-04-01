---
name: setup-detritus
description: Install or update the detritus MCP knowledge base server — handles binary install and editor configuration for Windsurf, VS Code, Cursor, and Verdant.
---

# /setup-detritus — Install Detritus MCP Server

Detect the user's OS and shell before proceeding.

## Step 1: Install the binary

### Linux / macOS / Windows (Git Bash, WSL, MSYS2)

```bash
curl -sSL https://raw.githubusercontent.com/benitogf/detritus/main/install.sh | sh
```

### Windows (PowerShell only)

```powershell
irm https://raw.githubusercontent.com/benitogf/detritus/main/install.ps1 | iex
```

## Step 2: Verify installation

```bash
detritus --version
```

## Step 3: Verify MCP config

The install script auto-configures:
- **Windsurf**: `~/.codeium/windsurf/mcp_config.json`
- **VS Code**: `~/.config/Code/User/mcp.json` (Linux), `~/Library/Application Support/Code/User/mcp.json` (macOS), `%APPDATA%\Code\User\mcp.json` (Windows)
- **Cursor**: `~/.cursor/mcp.json`

Read the relevant config and verify the `"detritus"` entry exists.

## Step 4: Install plugin (VS Code)

For VS Code users, the Agent Plugin can be installed from source:
1. Open Command Palette → `Chat: Install Plugin From Source`
2. Enter: `https://github.com/benitogf/detritus`

This enables agent skills (`/plan`, `/create`, `/testing`, etc.) and the detritus custom agent.

## Step 5: Restart

Restart your editor to activate the MCP server and skills.
