---
description: Install or update detritus MCP knowledge base server
---

# Setup Detritus MCP Server

## Step 1: Install the binary

// turbo
```bash
curl -sSL https://raw.githubusercontent.com/benitogf/detritus/main/install.sh | sh
```

## Step 2: Verify installation

// turbo
```bash
detritus --help 2>/dev/null || echo "detritus installed at $(which detritus)"
```

## Step 3: Configure Windsurf MCP

Add the following to `~/.codeium/windsurf/mcp_config.json` inside the `"mcpServers"` object:

```json
"detritus": {
  "command": "/usr/local/bin/detritus",
  "args": [],
  "disabled": false
}
```

## Step 4: Restart Windsurf

Restart Windsurf to load the new MCP server. After restart, the `kb_list`, `kb_get`, and `kb_search` tools will be available automatically.

## Update

To update to the latest version, re-run Step 1.
