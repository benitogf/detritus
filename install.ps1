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
tools: ["detritus/*"]
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
        Write-Host "Updated $settingsPath (chat.promptFilesLocations)"
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

$vsCodeUserDir = Join-Path $env:APPDATA "Code\User"
Configure-VSCodeMcp $vsCodeUserDir

Write-Host ""
Write-Host "VS Code slash commands: loaded from ~/.copilot/prompts/ (shared across workspaces)"
Write-Host "Optional: run 'detritus --init' in a repo if you specifically want repo-local prompt files."
Write-Host "Reload VS Code window (Ctrl+Shift+P > Developer: Reload Window) to activate."
