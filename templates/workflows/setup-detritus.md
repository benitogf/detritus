---
description: Install or update detritus MCP knowledge base server
---

# Setup Detritus MCP Server

Detect the user's OS and **shell** before proceeding. On Windows, check if the terminal is PowerShell or a bash-like shell (Git Bash, WSL, MSYS2). Run ONLY the commands matching their platform and shell.

## Step 1: Install the binary

### Linux / macOS / Windows (Git Bash, WSL, MSYS2)

// turbo
```bash
curl -sSL https://raw.githubusercontent.com/benitogf/detritus/main/install.sh | sh
```

### Windows (PowerShell only)

// turbo
```powershell
irm https://raw.githubusercontent.com/benitogf/detritus/main/install.ps1 | iex
```

## Step 2: Verify MCP config

The install script automatically configures `~/.codeium/windsurf/mcp_config.json` (`~` = `%USERPROFILE%` on Windows).

Read the config file and verify the `"detritus"` entry exists with the correct binary path:
- **Linux/macOS**: `/usr/local/bin/detritus`
- **Windows**: `C:/Users/USERNAME/AppData/Local/detritus/detritus.exe` (forward slashes)

## Step 3: Select target project

Check the workspace roots (available from IDE metadata). **Install project files only to the root the user selects.**

- **Single root**: use it directly, no prompt needed.
- **Multiple roots**: list all workspace roots and ask the user which one should receive the MCP workflow files. Install to that root only.

Let `TARGET` be the selected root for the steps below.

## Step 4: Install project files to TARGET

### 4a. Download `.windsurfrules`

If `TARGET/.windsurfrules` does not exist, download it:

#### Linux / macOS / Windows (Git Bash, WSL, MSYS2)

// turbo
```bash
[ ! -f .windsurfrules ] && curl -sSL https://raw.githubusercontent.com/benitogf/detritus/main/templates/.windsurfrules -o .windsurfrules
```

#### Windows (PowerShell only)

// turbo
```powershell
if (-not (Test-Path .windsurfrules)) { irm https://raw.githubusercontent.com/benitogf/detritus/main/templates/.windsurfrules | Set-Content .windsurfrules -Encoding UTF8 }
```

### 4b. Create the setup workflow bootstrapper

Create `TARGET/.windsurf/workflows/detritus/setup-detritus.md` if it doesn't exist:

```markdown
---
description: Install or update detritus MCP knowledge base server
---

Fetch the latest setup instructions from:
https://raw.githubusercontent.com/benitogf/detritus/main/templates/workflows/setup-detritus.md

Read that URL and execute all the steps in the fetched document.
```

### 4c. Create workflow aliases from MCP

Call `kb_list()` to get all available document names. For each document, create a workflow alias file at `TARGET/.windsurf/workflows/detritus/{alias}.md` **only if it doesn't already exist**.

Deriving the alias filename from the document name:

- **Subdirectory path**: use only the last segment (e.g., `scaffold/create` → `create.md`)
- The `kb_get` call inside must always use the **full original name** (e.g., `scaffold/create`)

Each workflow alias file should follow this exact format:

```markdown
---
description: {description from kb_list}
---

Call kb_get(name="{full_name}") and follow the instructions in the returned document.
```

**If `kb_list` is not available** (first-time install — MCP not loaded yet), skip this step and tell the user to restart Windsurf then re-run `/setup-detritus` to generate the workflow aliases.

### 4d. Clean up old flat installations

Previous versions of detritus installed workflow aliases directly into `TARGET/.windsurf/workflows/`. Check if any detritus-created alias files exist there (outside the `detritus/` subfolder).

To identify detritus-created files: call `kb_list()` to get all document names. Any `.md` file in `TARGET/.windsurf/workflows/` whose name (without `.md`) matches a document name or alias name from `kb_list()` — or is `setup` or `setup-detritus` — is a detritus-created file. Also check for these known old names: `_truthseeker.md`, `scaffold-simple-service.md`, `create-app.md`, `create-service.md`, `setup.md`.

Also clean up old alias files inside `TARGET/.windsurf/workflows/detritus/` that no longer match any current document (e.g., `scaffold-simple-service.md`).

Delete only those files. Do **not** delete any other files or folders — those are user-created.

## Step 5: Restart Windsurf

Tell the user to **fully close Windsurf** (File > Exit, not just close the window) and reopen it. After restart, the `kb_list`, `kb_get`, and `kb_search` tools will be available.

If this was a first-time install, remind the user to run `/setup-detritus` again after restart to generate workflow aliases (Step 4c).

## Update

To update to the latest version, re-run all steps. Step 4c will add workflow aliases for any new documents added since the last run. Step 4d will clean up any leftover files from older flat installations.

## Troubleshooting

### Verify the binary

```bash
detritus --version
```

On Windows:
```powershell
& "$env:LOCALAPPDATA\detritus\detritus.exe" --version
```

This should print `detritus <version>`. If it outputs JSON-RPC or hangs, you have an old binary without `--version` support — re-run Step 1.

### MCP server not loading after restart

1. **Check the config path**: Must be `~/.codeium/windsurf/mcp_config.json` (on Windows: `%USERPROFILE%\.codeium\windsurf\mcp_config.json`)
2. **Check the binary path in config**: Must use **forward slashes** even on Windows (e.g., `C:/Users/Name/AppData/Local/detritus/detritus.exe`)
3. **Full restart required**: File > Exit (or Alt+F4), not just closing the window. On Windows, check Task Manager to ensure all Windsurf processes are stopped
4. **Check MCP panel**: Settings (gear icon) > Cascade > MCP Servers — detritus should appear there
5. **Verify config is valid JSON**: Open `mcp_config.json` in a text editor and check for syntax errors (trailing commas, missing quotes)

### Windows-specific issues

- **Path must use forward slashes** in `mcp_config.json`: `C:/Users/...` not `C:\Users\...`
- **Do not run the binary manually** — it communicates via stdio and will appear to hang. Use `--version` to test
- **Antivirus may block**: Some antivirus software blocks unsigned executables. Add an exception for `%LOCALAPPDATA%\detritus\detritus.exe`
