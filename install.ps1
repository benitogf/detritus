$ErrorActionPreference = "Stop"

$repo = "benitogf/detritus"
$binary = "detritus"

# Detect architecture
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }

# Get latest version
$release = Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest"
$version = $release.tag_name
if (-not $version) {
    Write-Error "Failed to get latest version"
    exit 1
}

Write-Host "Installing $binary $version (windows/$arch)..."

# Setup install directory
$installDir = "$env:LOCALAPPDATA\detritus"
if (-not (Test-Path $installDir)) {
    New-Item -ItemType Directory -Path $installDir -Force | Out-Null
}

# Download
$url = "https://github.com/$repo/releases/download/$version/${binary}_windows_${arch}.zip"
$tmpZip = Join-Path $env:TEMP "detritus_download.zip"
$tmpExtract = Join-Path $env:TEMP "detritus_extract"

Write-Host "Downloading $url..."
Invoke-WebRequest -Uri $url -OutFile $tmpZip

# Extract
if (Test-Path $tmpExtract) { Remove-Item $tmpExtract -Recurse -Force }
Expand-Archive -Path $tmpZip -DestinationPath $tmpExtract

# Stop running detritus process (Windows locks running executables)
$running = Get-Process -Name $binary -ErrorAction SilentlyContinue
if ($running) {
    Write-Host "Stopping running detritus process..."
    $running | Stop-Process -Force
    Start-Sleep -Milliseconds 500
}

# Install
Copy-Item "$tmpExtract\$binary.exe" "$installDir\$binary.exe" -Force

# Cleanup
Remove-Item $tmpZip -Force -ErrorAction SilentlyContinue
Remove-Item $tmpExtract -Recurse -Force -ErrorAction SilentlyContinue

$binaryPath = "$installDir\$binary.exe"

# Verify binary works (timeout protects against old binaries without --version)
Write-Host "Verifying installation..."
try {
    $proc = Start-Process -FilePath $binaryPath -ArgumentList "--version" -NoNewWindow -PassThru -RedirectStandardOutput "$env:TEMP\detritus_ver.txt" -RedirectStandardError "$env:TEMP\detritus_ver_err.txt"
    $exited = $proc.WaitForExit(5000)
    if ($exited -and $proc.ExitCode -eq 0) {
        $verifyOutput = (Get-Content "$env:TEMP\detritus_ver.txt" -Raw).Trim()
        Write-Host "  $verifyOutput"
    } else {
        if (-not $exited) { $proc.Kill() }
        Write-Host "  Warning: --version not supported (old binary?). Install completed but verify manually after restart."
    }
    Remove-Item "$env:TEMP\detritus_ver.txt" -Force -ErrorAction SilentlyContinue
    Remove-Item "$env:TEMP\detritus_ver_err.txt" -Force -ErrorAction SilentlyContinue
} catch {
    Write-Host "  Warning: Could not verify binary. Install completed."
}

Write-Host ""
Write-Host "Installed $binary $version to $binaryPath"

# Auto-configure mcp_config.json
# Avoid PowerShell 5.1 ConvertTo-Json bugs (empty arrays → null, UTF8 BOM)
$mcpConfigPath = Join-Path $env:USERPROFILE ".codeium\windsurf\mcp_config.json"
$mcpConfigDir = Split-Path $mcpConfigPath
$binaryPathForJson = $binaryPath -replace '\\', '/'

if (-not (Test-Path $mcpConfigDir)) {
    New-Item -ItemType Directory -Path $mcpConfigDir -Force | Out-Null
}

$detritusBlock = @"
    "detritus": {
      "command": "$binaryPathForJson",
      "args": [],
      "disabled": false
    }
"@

if (Test-Path $mcpConfigPath) {
    $raw = Get-Content $mcpConfigPath -Raw
    if ($raw -match '"detritus"\s*:') {
        # Replace existing detritus block (match up to closing brace at same indent)
        $raw = [regex]::Replace($raw, '"detritus"\s*:\s*\{[^}]*\}', $detritusBlock.Trim())
        Write-Host "Updated existing detritus entry in $mcpConfigPath"
    } elseif ($raw -match '"mcpServers"\s*:\s*\{') {
        # Insert detritus into existing mcpServers
        $raw = [regex]::Replace($raw, '("mcpServers"\s*:\s*\{)', "`$1`n$detritusBlock,")
        Write-Host "Added detritus to $mcpConfigPath"
    } else {
        # No mcpServers key, wrap entire file
        $json = @"
{
  "mcpServers": {
$detritusBlock
  }
}
"@
        $raw = $json
        Write-Host "Created mcpServers with detritus in $mcpConfigPath"
    }
    # Write UTF8 without BOM (PS 5.1 compat)
    [System.IO.File]::WriteAllText($mcpConfigPath, $raw, [System.Text.UTF8Encoding]::new($false))
} else {
    $json = @"
{
  "mcpServers": {
$detritusBlock
  }
}
"@
    [System.IO.File]::WriteAllText($mcpConfigPath, $json, [System.Text.UTF8Encoding]::new($false))
    Write-Host "Created $mcpConfigPath"
}

# Show config for verification
Write-Host ""
Write-Host "MCP config: $mcpConfigPath"
Write-Host "Binary:     $binaryPath"
Write-Host ""
Write-Host "--- Config contents ---"
Get-Content $mcpConfigPath
Write-Host "--- End config ---"
Write-Host ""
Write-Host "Restart Windsurf (File > Exit, then reopen) to activate."
Write-Host ""
Write-Host "To verify after restart, ask Cascade: 'list available kb docs'"

# Auto-configure VS Code
# VS Code uses "servers" (not "mcpServers") in mcp.json
# Prompt files (slash commands) are loaded from one shared folder to avoid duplicates in multi-root workspaces

function Get-VSCodeAliasForDoc {
    param([string]$Name)
    $parts = $Name.Split('/')
    $leaf = $parts[$parts.Length - 1]
    if ($Name -eq "plan/analyze") { return "plan" }
    if ($Name -eq "plan/export") { return "plan-export" }
    if ($Name -eq "plan/diagrams") { return "diagrams" }
    if ($Name -eq "testing/index") { return "testing" }
    if ($Name.StartsWith("testing/go-backend-")) { return "testing-$leaf" }
    if ($Name.StartsWith("ooo/")) { return "ooo-$leaf" }
    return $leaf
}

function Generate-SharedPrompts {
    $sharedPrompts = Join-Path $env:USERPROFILE ".copilot\prompts"
    New-Item -ItemType Directory -Path $sharedPrompts -Force | Out-Null

    $generated = @{}
    $listOutput = & $binaryPath --list 2>$null
    foreach ($line in $listOutput) {
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        $parts = $line -split "`t", 2
        if ($parts.Count -lt 1 -or [string]::IsNullOrWhiteSpace($parts[0])) { continue }
        $name = $parts[0]
        $desc = if ($parts.Count -ge 2) { $parts[1] } else { "" }
        $alias = Get-VSCodeAliasForDoc $name
        $fileName = "$alias.prompt.md"
        $generated[$fileName] = $true
        $filePath = Join-Path $sharedPrompts $fileName

        $content = @"
---
description: $desc
agent: agent
---

Call kb_get(name="$name") and follow the instructions in the returned document.
"@
        [System.IO.File]::WriteAllText($filePath, $content, [System.Text.UTF8Encoding]::new($false))
    }

    # Remove stale detritus-generated prompts (keep unrelated user prompts)
    Get-ChildItem $sharedPrompts -Filter "*.prompt.md" -ErrorAction SilentlyContinue | ForEach-Object {
        if (-not $generated.ContainsKey($_.Name)) {
            $raw = Get-Content $_.FullName -Raw
            if ($raw -match 'kb_get\(name="') {
                Remove-Item $_.FullName -Force
            }
        }
    }

    Write-Host "Shared VS Code prompts: $sharedPrompts"
}

function Generate-InlineCommandInstructions {
    $instrDir = Join-Path $env:USERPROFILE ".copilot\instructions"
    $instrFile = Join-Path $instrDir "detritus.instructions.md"
    New-Item -ItemType Directory -Path $instrDir -Force | Out-Null

    $lines = New-Object System.Collections.Generic.List[string]
    $lines.Add('---')
    $lines.Add('description: detritus knowledge base guardrails and command router')
    $lines.Add('applyTo: "**"')
    $lines.Add('---')
    $lines.Add('')
    $lines.Add('## Guardrails')
    $lines.Add('')
    $lines.Add('Push back when evidence demands it — including against the user. Research (KB via kb_search/kb_get, source code, docs) before asking researchable questions. Prove before acting. Early returns, flat code, no deep nesting.')
    $lines.Add('')
    $lines.Add('## Command Tokens')
    $lines.Add('')
    $lines.Add('When a user message contains one or more detritus command tokens anywhere in the text (for example: /truthseeker, /plan, /testing), treat each token as an explicit request to load the matching knowledge doc.')
    $lines.Add('')
    $lines.Add('Rules:')
    $lines.Add('1. Detect command tokens anywhere in the message, not only at the beginning.')
    $lines.Add('2. Support multiple tokens in one message; process all of them (deduplicated) in order of appearance.')
    $lines.Add('3. For each detected token, call kb_get(name="...") with the mapped doc name before producing the final answer.')
    $lines.Add('4. If no token is present, do not force a kb_get call from this instruction alone.')
    $lines.Add('')
    $lines.Add('Token to doc mapping:')

    $listOutput = & $binaryPath --list 2>$null
    foreach ($line in $listOutput) {
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        $parts = $line -split "`t", 2
        if ($parts.Count -lt 1 -or [string]::IsNullOrWhiteSpace($parts[0])) { continue }
        $name = $parts[0]
        $alias = Get-VSCodeAliasForDoc $name
        $lines.Add("- /$alias -> $name")
    }

    [System.IO.File]::WriteAllLines($instrFile, $lines, [System.Text.UTF8Encoding]::new($false))
    Write-Host "VS Code shared instructions: $instrFile"
}

function Generate-AgentFile {
    $agentsDir = Join-Path $env:USERPROFILE ".copilot\agents"
    New-Item -ItemType Directory -Path $agentsDir -Force | Out-Null
    $agentFile = Join-Path $agentsDir "detritus.agent.md"

    $content = @"
---
name: detritus
description: Knowledge-enhanced coding agent with ooo ecosystem expertise, truthseeker principles, and project-specific guardrails.
tools:
  - detritus
---

# Detritus Agent

You have access to the **detritus MCP server** providing knowledge base tools: ``kb_list``, ``kb_get``, ``kb_search``. Use them to answer questions about the ooo ecosystem, testing patterns, Go idioms, and project architecture.

## Always-On Principles

1. **Push back when facts demand it** — including against the user. Do not soften challenges.
2. **Research before asking** — exhaust KB docs (``kb_search``, ``kb_get``), source code, and inline docs before asking the user anything researchable.
3. **Prove before acting** — base conclusions on evidence, not assumptions. Show your reasoning.
4. **Radical honesty** — if something is wrong, unproven, or assumed, say so directly.
5. **Line-of-sight code** — early returns, flat structure, no deep nesting.

## Workflow

- For planning tasks, use the ``/plan`` prompt followed by ``/plan-export`` for documents.
- For scaffolding, use the ``/create`` prompt.
- For testing guidance, use the ``/testing`` prompt.
- When uncertain about ooo internals, search the KB first: ``kb_search(query="your question")``.
"@

    [System.IO.File]::WriteAllText($agentFile, $content, [System.Text.UTF8Encoding]::new($false))
    Write-Host "Agent file: $agentFile"
}

function Test-ContinueInstalled {
    if (Get-Command cn -ErrorAction SilentlyContinue) { return $true }
    if (Test-Path (Join-Path $env:USERPROFILE ".continue")) { return $true }
    if (Test-Path "$env:USERPROFILE\.vscode\extensions") {
        if (Get-ChildItem "$env:USERPROFILE\.vscode\extensions" -Filter "*continue*" -ErrorAction SilentlyContinue) { return $true }
    }
    if (Test-Path "$env:USERPROFILE\.vscode-server\extensions") {
        if (Get-ChildItem "$env:USERPROFILE\.vscode-server\extensions" -Filter "*continue*" -ErrorAction SilentlyContinue) { return $true }
    }
    return $false
}

function Configure-Continue {
    $continueDir = Join-Path $env:USERPROFILE ".continue"
    $mcpDir = Join-Path $continueDir "mcpServers"
    $promptsDir = Join-Path $continueDir "prompts"
    New-Item -ItemType Directory -Path $mcpDir -Force | Out-Null
    New-Item -ItemType Directory -Path $promptsDir -Force | Out-Null

    $mcpContent = @"
name: detritus-local
version: 0.0.1
schema: v1
mcpServers:
  - name: detritus
    command: $binaryPathForJson
    args: []
"@
    [System.IO.File]::WriteAllText((Join-Path $mcpDir "detritus.yaml"), $mcpContent, [System.Text.UTF8Encoding]::new($false))

    $generated = @{}
    $listOutput = & $binaryPath --list 2>$null
    foreach ($line in $listOutput) {
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        $parts = $line -split "`t", 2
        if ($parts.Count -lt 1 -or [string]::IsNullOrWhiteSpace($parts[0])) { continue }
        $name = $parts[0]
        $alias = Get-VSCodeAliasForDoc $name
        $generated["$alias.prompt"] = $true
        $content = @"
name: $alias
description: Load detritus knowledge doc $name
invokable: true
---
Use the detritus MCP server and call kb_get with name="$name". Then follow the returned guidance strictly.
"@
        [System.IO.File]::WriteAllText((Join-Path $promptsDir "$alias.prompt"), $content, [System.Text.UTF8Encoding]::new($false))
    }

    # Remove stale detritus-generated prompts while preserving unrelated user prompts
    Get-ChildItem $promptsDir -Filter "*.prompt" -ErrorAction SilentlyContinue | ForEach-Object {
        if ($_.Name -eq "detritus-help.prompt") { return }
        if ($generated.ContainsKey($_.Name)) { return }
        $raw = Get-Content $_.FullName -Raw
        if ($raw -match 'Use the detritus MCP server and call kb_get with name=') {
            Remove-Item $_.FullName -Force
        }
    }

    $commands = Get-ChildItem $promptsDir -Filter "*.prompt" -ErrorAction SilentlyContinue |
        ForEach-Object { $_.BaseName } |
        Sort-Object -Unique |
        ForEach-Object { "- /$_" }

    $helpContent = @(
        "name: detritus-help",
        "description: List all detritus slash commands",
        "invokable: true",
        "---",
        "Available detritus commands:"
    ) + $commands
    [System.IO.File]::WriteAllLines((Join-Path $promptsDir "detritus-help.prompt"), $helpContent, [System.Text.UTF8Encoding]::new($false))

    Write-Host "Continue MCP config: $(Join-Path $mcpDir 'detritus.yaml')"
    Write-Host "Continue prompts: $promptsDir"
}

function Test-VerdentInstalled {
    if (Test-Path (Join-Path $env:USERPROFILE ".verdent")) { return $true }
    if (Test-Path "$env:USERPROFILE\.vscode\extensions") {
        if (Get-ChildItem "$env:USERPROFILE\.vscode\extensions" -Filter "*verdent*" -ErrorAction SilentlyContinue) { return $true }
    }
    if (Test-Path "$env:USERPROFILE\.vscode-server\extensions") {
        if (Get-ChildItem "$env:USERPROFILE\.vscode-server\extensions" -Filter "*verdent*" -ErrorAction SilentlyContinue) { return $true }
    }
    return $false
}

function Configure-Verdent {
    $verdentDir = Join-Path $env:USERPROFILE ".verdent"
    $verdentMcp = Join-Path $verdentDir "mcp.json"
    $verdentRules = Join-Path $verdentDir "VERDENT.md"
    New-Item -ItemType Directory -Path $verdentDir -Force | Out-Null

    if (Test-Path $verdentMcp) {
        $raw = Get-Content $verdentMcp -Raw
        if ([string]::IsNullOrWhiteSpace($raw)) {
            $raw = "{}"
        }
        try {
            $data = $raw | ConvertFrom-Json -Depth 20
        } catch {
            $data = [pscustomobject]@{}
        }
        if (-not ($data.PSObject.Properties.Name -contains "mcpServers")) {
            $data | Add-Member -NotePropertyName "mcpServers" -NotePropertyValue ([pscustomobject]@{})
        }
        $data.mcpServers | Add-Member -NotePropertyName "detritus" -NotePropertyValue ([pscustomobject]@{ command = $binaryPathForJson; args = @() }) -Force
        $json = $data | ConvertTo-Json -Depth 20
        [System.IO.File]::WriteAllText($verdentMcp, $json, [System.Text.UTF8Encoding]::new($false))
        Write-Host "Updated detritus in $verdentMcp"
    } else {
        $json = @"
{
  "mcpServers": {
    "detritus": {
      "command": "$binaryPathForJson",
      "args": []
    }
  }
}
"@
        [System.IO.File]::WriteAllText($verdentMcp, $json, [System.Text.UTF8Encoding]::new($false))
        Write-Host "Created $verdentMcp"
    }

    $commands = (& $binaryPath --list 2>$null) |
        ForEach-Object {
            if ([string]::IsNullOrWhiteSpace($_)) { return }
            $parts = $_ -split "`t", 2
            if ($parts.Count -lt 1 -or [string]::IsNullOrWhiteSpace($parts[0])) { return }
            $name = $parts[0]
            $alias = Get-VSCodeAliasForDoc $name
            "- /$alias -> $name"
        }

    $ruleBlock = @(
        "<!-- DETRITUS-RULES:START -->",
        "# Detritus Knowledge Base Rules",
        "",
        "- Use the detritus MCP server as the default knowledge source for software-engineering guidance.",
        "- For architecture, planning, testing, patterns, and ooo ecosystem questions, call detritus kb_get before answering.",
        "- When uncertain which document to use, call kb_search first and then kb_get for the best match.",
        "- Keep manual invocation available. If user explicitly asks, support command-style prompts like /plan, /grow, /create, /testing.",
        "",
        "Manual command to doc mapping:"
    ) + $commands + @(
        "<!-- DETRITUS-RULES:END -->"
    )

    if (Test-Path $verdentRules) {
        $existing = Get-Content $verdentRules
        $start = ($existing | Select-String '<!-- DETRITUS-RULES:START -->' -SimpleMatch).LineNumber
        $end = ($existing | Select-String '<!-- DETRITUS-RULES:END -->' -SimpleMatch).LineNumber
        if ($start -and $end -and $end -ge $start) {
            $before = if ($start -gt 1) { $existing[0..($start-2)] } else { @() }
            $after = if ($end -lt $existing.Length) { $existing[$end..($existing.Length-1)] } else { @() }
            $merged = @($before + $after + @("") + $ruleBlock)
            [System.IO.File]::WriteAllLines($verdentRules, $merged, [System.Text.UTF8Encoding]::new($false))
        } else {
            $merged = @($existing + @("", "") + $ruleBlock)
            [System.IO.File]::WriteAllLines($verdentRules, $merged, [System.Text.UTF8Encoding]::new($false))
        }
    } else {
        [System.IO.File]::WriteAllLines($verdentRules, $ruleBlock, [System.Text.UTF8Encoding]::new($false))
    }

    # Generate Verdent skills for slash-command support
    $skillsDir = Join-Path $verdentDir "skills"
    New-Item -ItemType Directory -Path $skillsDir -Force | Out-Null

    $generatedSkills = @{}
    $listOutput2 = & $binaryPath --list 2>$null
    foreach ($line in $listOutput2) {
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        $parts = $line -split "`t", 2
        if ($parts.Count -lt 1 -or [string]::IsNullOrWhiteSpace($parts[0])) { continue }
        $name = $parts[0]
        $desc = if ($parts.Count -ge 2) { $parts[1] } else { "Detritus knowledge base document: $name" }
        $alias = Get-VSCodeAliasForDoc $name
        $generatedSkills[$alias] = $true

        $skillDir = Join-Path $skillsDir $alias
        New-Item -ItemType Directory -Path $skillDir -Force | Out-Null
        $skillFile = Join-Path $skillDir "SKILL.md"
        $skillContent = @"
---
name: $alias
description: $desc
---

Call the detritus MCP tool ``kb_get`` with name="$name" and follow the instructions in the returned document.
"@
        [System.IO.File]::WriteAllText($skillFile, $skillContent, [System.Text.UTF8Encoding]::new($false))
    }

    # Remove stale detritus-generated skills
    if (Test-Path $skillsDir) {
        Get-ChildItem $skillsDir -Directory -ErrorAction SilentlyContinue | ForEach-Object {
            if (-not $generatedSkills.ContainsKey($_.Name)) {
                $sf = Join-Path $_.FullName "SKILL.md"
                if ((Test-Path $sf) -and ((Get-Content $sf -Raw) -match 'kb_get')) {
                    Remove-Item $_.FullName -Recurse -Force
                }
            }
        }
    }

    Write-Host "Verdent MCP config: $verdentMcp"
    Write-Host "Verdent rules: $verdentRules"
    Write-Host "Verdent skills: $skillsDir"
}

function Configure-VSCodeMcp {
    param([string]$VsCodeDir)
    if (-not (Test-Path $VsCodeDir)) { return }

    $vscodeMcp = Join-Path $VsCodeDir "mcp.json"
    $binaryPathJson = $binaryPath -replace '\\', '/'

    # Write mcp.json
    $detritusBlock = "    `"detritus`": {`n      `"command`": `"$binaryPathJson`",`n      `"args`": []`n    }"
    if (Test-Path $vscodeMcp) {
        $raw = Get-Content $vscodeMcp -Raw
        if ($raw -match '"detritus"\s*:') {
            $raw = [regex]::Replace($raw, '"detritus"\s*:\s*\{[^}]*\}', $detritusBlock.Trim())
            Write-Host "Updated detritus in $vscodeMcp"
        } elseif ($raw -match '"servers"\s*:\s*\{') {
            $raw = [regex]::Replace($raw, '("servers"\s*:\s*\{)', "`$1`n$detritusBlock,")
            Write-Host "Added detritus to $vscodeMcp"
        } else {
            $json = "{`n  `"servers`": {`n$detritusBlock`n  }`n}"
            $raw = $json
            Write-Host "Created servers with detritus in $vscodeMcp"
        }
        [System.IO.File]::WriteAllText($vscodeMcp, $raw, [System.Text.UTF8Encoding]::new($false))
    } else {
        $json = "{`n  `"servers`": {`n$detritusBlock`n  }`n}"
        [System.IO.File]::WriteAllText($vscodeMcp, $json, [System.Text.UTF8Encoding]::new($false))
        Write-Host "Created $vscodeMcp"
    }

    # Configure a single prompt source to avoid duplicate slash commands in multi-root workspaces.
    $settingsPath = Join-Path $VsCodeDir "settings.json"
    $promptLocationsBlock = @"
  "chat.promptFilesLocations": {
    ".github/prompts": false,
    "~/.copilot/prompts": true
  },
  "chat.instructionsFilesLocations": {
    "~/.copilot/instructions": true
  },
  "chat.agentFilesLocations": {
    "~/.copilot/agents": true
  }
"@
    if (Test-Path $settingsPath) {
        $raw = Get-Content $settingsPath -Raw
        if ([string]::IsNullOrWhiteSpace($raw) -or $raw.Trim() -eq "{}") {
            $raw = "{`n$promptLocationsBlock`n}`n"
        } elseif ($raw -match '"chat\.promptFilesLocations"\s*:\s*\{[^}]*\}') {
            $raw = [regex]::Replace($raw, '"chat\.promptFilesLocations"\s*:\s*\{[^}]*\}', $promptLocationsBlock.Trim())
        } elseif ($raw -match '^\s*\{') {
            $raw = [regex]::Replace($raw, '^\s*\{', "{`n$promptLocationsBlock,", 1)
        } else {
            $raw = "{`n$promptLocationsBlock`n}`n"
        }
        [System.IO.File]::WriteAllText($settingsPath, $raw, [System.Text.UTF8Encoding]::new($false))
        Write-Host "Updated $settingsPath (chat.promptFilesLocations, chat.instructionsFilesLocations)"
    } else {
        $json = "{`n$promptLocationsBlock`n}`n"
        [System.IO.File]::WriteAllText($settingsPath, $json, [System.Text.UTF8Encoding]::new($false))
        Write-Host "Created $settingsPath"
    }

    # Clean up old user-level prompt files (no longer used — prompts are workspace-level now)
    $oldPrompts = Join-Path $VsCodeDir "prompts"
    if (Test-Path $oldPrompts) {
        Get-ChildItem "$oldPrompts\*.prompt.md" | ForEach-Object {
            if ((Get-Content $_.FullName -Raw) -match 'kb_get') {
                Remove-Item $_.FullName -Force
            }
        }
        $remaining = Get-ChildItem $oldPrompts -ErrorAction SilentlyContinue
        if (-not $remaining) { Remove-Item $oldPrompts -Force -ErrorAction SilentlyContinue }
        Write-Host "Cleaned up old user-level prompt files from $oldPrompts\"
    }

    Write-Host "VS Code MCP config: $vscodeMcp"
}

Generate-SharedPrompts
Generate-InlineCommandInstructions
Generate-AgentFile

if (Test-ContinueInstalled) {
    Configure-Continue
} else {
    Write-Host "Continue not detected; skipping Continue prompt/MCP setup."
}

if (Test-VerdentInstalled) {
    Configure-Verdent
} else {
    Write-Host "Verdent not detected; skipping Verdent MCP/rules setup."
}

Write-Host ""
Write-Host "Post-install verification:"

if ((Test-Path $mcpConfigPath) -and ((Get-Content $mcpConfigPath -Raw) -match '"detritus"\s*:')) {
    Write-Host "  [PASS] Windsurf MCP entry"
} else {
    Write-Host "  [WARN] Windsurf MCP entry"
}

$vsCodeMcpPath = Join-Path $env:APPDATA "Code\User\mcp.json"
if ((Test-Path $vsCodeMcpPath) -and ((Get-Content $vsCodeMcpPath -Raw) -match '"detritus"\s*:')) {
    Write-Host "  [PASS] VS Code MCP entry"
} else {
    Write-Host "  [WARN] VS Code MCP entry"
}

$copilotPrompt = Join-Path $env:USERPROFILE ".copilot\prompts\plan.prompt.md"
$copilotInstr = Join-Path $env:USERPROFILE ".copilot\instructions\detritus.instructions.md"
if ((Test-Path $copilotPrompt) -and (Test-Path $copilotInstr)) {
    Write-Host "  [PASS] Copilot shared prompts/instructions"
} else {
    Write-Host "  [WARN] Copilot shared prompts/instructions"
}

if (Test-ContinueInstalled) {
    $continueMcp = Join-Path $env:USERPROFILE ".continue\mcpServers\detritus.yaml"
    if (Test-Path $continueMcp) {
        Write-Host "  [PASS] Continue MCP config"
    } else {
        Write-Host "  [WARN] Continue MCP config"
    }
}

if (Test-VerdentInstalled) {
    $verdentMcp = Join-Path $env:USERPROFILE ".verdent\mcp.json"
    $verdentRules = Join-Path $env:USERPROFILE ".verdent\VERDENT.md"
    $verdentSkills = Join-Path $env:USERPROFILE ".verdent\skills"
    if ((Test-Path $verdentMcp) -and (Test-Path $verdentRules)) {
        Write-Host "  [PASS] Verdent MCP/rules"
    } else {
        Write-Host "  [WARN] Verdent MCP/rules"
    }
    if ((Test-Path $verdentSkills) -and (Get-ChildItem $verdentSkills -Directory -ErrorAction SilentlyContinue)) {
        Write-Host "  [PASS] Verdent skills"
    } else {
        Write-Host "  [WARN] Verdent skills"
    }
}

$vsCodeUserDir = Join-Path $env:APPDATA "Code\User"
Configure-VSCodeMcp $vsCodeUserDir

# Auto-configure Cursor MCP
function Configure-CursorMcp {
    param([string]$CursorDir)
    if (-not (Test-Path $CursorDir)) { return }

    $cursorMcp = Join-Path $CursorDir "mcp.json"
    $binaryPathJson = $binaryPath -replace '\\', '/'

    $detritusBlock = "    `"detritus`": {`n      `"command`": `"$binaryPathJson`",`n      `"args`": []`n    }"
    if (Test-Path $cursorMcp) {
        $raw = Get-Content $cursorMcp -Raw
        if ($raw -match '"detritus"\s*:') {
            $raw = [regex]::Replace($raw, '"detritus"\s*:\s*\{[^}]*\}', $detritusBlock.Trim())
            Write-Host "Updated detritus in $cursorMcp"
        } elseif ($raw -match '"mcpServers"\s*:\s*\{') {
            $raw = [regex]::Replace($raw, '("mcpServers"\s*:\s*\{)', "`$1`n$detritusBlock,")
            Write-Host "Added detritus to $cursorMcp"
        } else {
            $json = "{`n  `"mcpServers`": {`n$detritusBlock`n  }`n}"
            $raw = $json
        }
        [System.IO.File]::WriteAllText($cursorMcp, $raw, [System.Text.UTF8Encoding]::new($false))
    } else {
        $json = "{`n  `"mcpServers`": {`n$detritusBlock`n  }`n}"
        [System.IO.File]::WriteAllText($cursorMcp, $json, [System.Text.UTF8Encoding]::new($false))
        Write-Host "Created $cursorMcp"
    }

    Write-Host "Cursor MCP config: $cursorMcp"
}

$cursorUserDir = Join-Path $env:APPDATA "Cursor\User"
Configure-CursorMcp $cursorUserDir

Write-Host ""
Write-Host "VS Code slash commands: loaded from ~/.copilot/prompts/ (shared across workspaces)"
Write-Host "Inline detritus tokens: use multiple commands anywhere in one message (example: '/truthseeker ... /plan')."
Write-Host "Continue integration: if Continue is installed, installer writes ~/.continue/mcpServers + ~/.continue/prompts."
Write-Host "Cursor integration: MCP config written to Cursor User directory."
Write-Host "Verdent integration: if Verdent is installed, installer writes ~/.verdent/mcp.json + ~/.verdent/VERDENT.md + ~/.verdent/skills/."
Write-Host "Optional: run 'detritus --init' in a repo if you specifically want repo-local prompt files."
Write-Host "Reload VS Code window (Ctrl+Shift+P > Developer: Reload Window) to activate."
