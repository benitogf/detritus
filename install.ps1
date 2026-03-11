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

# Install
Copy-Item "$tmpExtract\$binary.exe" "$installDir\$binary.exe" -Force

# Cleanup
Remove-Item $tmpZip -Force -ErrorAction SilentlyContinue
Remove-Item $tmpExtract -Recurse -Force -ErrorAction SilentlyContinue

$binaryPath = "$installDir\$binary.exe"

# Verify binary works
Write-Host "Verifying installation..."
$verifyOutput = & $binaryPath --version 2>&1
Write-Host "  $verifyOutput"

Write-Host ""
Write-Host "Installed $binary $version to $binaryPath"

# Auto-configure mcp_config.json
$mcpConfigPath = Join-Path $env:USERPROFILE ".codeium\windsurf\mcp_config.json"
$mcpConfigDir = Split-Path $mcpConfigPath
$binaryPathForJson = $binaryPath -replace '\\', '/'

if (-not (Test-Path $mcpConfigDir)) {
    New-Item -ItemType Directory -Path $mcpConfigDir -Force | Out-Null
}

if (Test-Path $mcpConfigPath) {
    $config = Get-Content $mcpConfigPath -Raw | ConvertFrom-Json
    if (-not $config.mcpServers) {
        $config | Add-Member -NotePropertyName "mcpServers" -NotePropertyValue ([PSCustomObject]@{})
    }
    $detritusCfg = [PSCustomObject]@{
        command  = $binaryPathForJson
        args     = @()
        disabled = $false
    }
    if ($config.mcpServers.detritus) {
        $config.mcpServers.detritus = $detritusCfg
        Write-Host "Updated existing detritus entry in $mcpConfigPath"
    } else {
        $config.mcpServers | Add-Member -NotePropertyName "detritus" -NotePropertyValue $detritusCfg
        Write-Host "Added detritus to $mcpConfigPath"
    }
    $config | ConvertTo-Json -Depth 10 | Set-Content $mcpConfigPath -Encoding UTF8
} else {
    $config = [PSCustomObject]@{
        mcpServers = [PSCustomObject]@{
            detritus = [PSCustomObject]@{
                command  = $binaryPathForJson
                args     = @()
                disabled = $false
            }
        }
    }
    $config | ConvertTo-Json -Depth 10 | Set-Content $mcpConfigPath -Encoding UTF8
    Write-Host "Created $mcpConfigPath"
}

Write-Host ""
Write-Host "MCP config: $mcpConfigPath"
Write-Host "Binary:     $binaryPath"
Write-Host ""
Write-Host "Restart Windsurf (File > Exit, then reopen) to activate."
Write-Host ""
Write-Host "To verify after restart, ask Cascade: 'list available kb docs'"
