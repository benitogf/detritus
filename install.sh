#!/bin/sh
set -e

REPO="benitogf/detritus"
BINARY="detritus"

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
  linux)  OS="linux" ;;
  darwin) OS="darwin" ;;
  mingw*|msys*|cygwin*) OS="windows" ;;
  *) echo "Unsupported OS: $OS" >&2; exit 1 ;;
esac

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

# Get latest version
VERSION=$(curl -sL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
if [ -z "$VERSION" ]; then
  echo "Failed to get latest version" >&2
  exit 1
fi

echo "Installing ${BINARY} ${VERSION} (${OS}/${ARCH})..."

# Setup install directory
if [ "$OS" = "windows" ]; then
  # Convert Windows LOCALAPPDATA path for Git Bash
  WIN_APPDATA=$(cygpath -u "$LOCALAPPDATA" 2>/dev/null || echo "$HOME/AppData/Local")
  INSTALL_DIR="${WIN_APPDATA}/detritus"
  BINARY_NAME="${BINARY}.exe"
  mkdir -p "$INSTALL_DIR"
else
  INSTALL_DIR="/usr/local/bin"
  BINARY_NAME="${BINARY}"
fi

# Download
EXT="tar.gz"
if [ "$OS" = "windows" ]; then
  EXT="zip"
fi
URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY}_${OS}_${ARCH}.${EXT}"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

echo "Downloading ${URL}..."
curl -sL "$URL" -o "${TMP}/archive.${EXT}"

# Extract
if [ "$EXT" = "zip" ]; then
  unzip -q "${TMP}/archive.zip" -d "$TMP"
else
  tar -xzf "${TMP}/archive.tar.gz" -C "$TMP"
fi

# Install (Windows locks running executables, kill first)
if [ "$OS" = "windows" ]; then
  taskkill //F //IM "${BINARY_NAME}" 2>/dev/null || true
  sleep 1
  cp "${TMP}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
else
  if [ -w "$INSTALL_DIR" ]; then
    mv "${TMP}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
  else
    echo "Installing to ${INSTALL_DIR} (requires sudo)..."
    sudo mv "${TMP}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
  fi
fi
chmod +x "${INSTALL_DIR}/${BINARY_NAME}"

# Verify binary works (timeout protects against old binaries without --version)
echo "Verifying installation..."
BINARY_PATH="${INSTALL_DIR}/${BINARY_NAME}"
if command -v timeout >/dev/null 2>&1; then
  VERIFY=$(timeout 5 "$BINARY_PATH" --version 2>&1) && echo "  ${VERIFY}" || echo "  Warning: --version timed out or failed. Install completed."
elif command -v gtimeout >/dev/null 2>&1; then
  VERIFY=$(gtimeout 5 "$BINARY_PATH" --version 2>&1) && echo "  ${VERIFY}" || echo "  Warning: --version timed out or failed. Install completed."
else
  # No timeout command available, skip verification
  echo "  Skipping verification (no timeout command). Binary installed."
fi

echo ""
echo "Installed ${BINARY} ${VERSION} to ${INSTALL_DIR}/${BINARY}"

# Auto-configure mcp_config.json
MCP_CONFIG="$HOME/.codeium/windsurf/mcp_config.json"
MCP_DIR=$(dirname "$MCP_CONFIG")

# For mcp_config.json, use forward-slash absolute path
if [ "$OS" = "windows" ]; then
  # Convert to Windows path with forward slashes for JSON
  BINARY_PATH_JSON=$(cygpath -w "${INSTALL_DIR}/${BINARY_NAME}" 2>/dev/null | sed 's/\\/\//g' || echo "${INSTALL_DIR}/${BINARY_NAME}")
else
  BINARY_PATH_JSON="$BINARY_PATH"
fi

mkdir -p "$MCP_DIR"

if [ -f "$MCP_CONFIG" ]; then
  if command -v python3 >/dev/null 2>&1; then
    python3 -c "
import json, sys
with open('$MCP_CONFIG', 'r') as f:
    config = json.load(f)
config.setdefault('mcpServers', {})
config['mcpServers']['detritus'] = {'command': '$BINARY_PATH_JSON', 'args': [], 'disabled': False}
with open('$MCP_CONFIG', 'w') as f:
    json.dump(config, f, indent=2)
print('Updated detritus in $MCP_CONFIG')
"
  else
    echo "python3 not found, please add manually to ${MCP_CONFIG}:"
    echo '  "detritus": { "command": "'${BINARY_PATH}'", "args": [], "disabled": false }'
  fi
else
  cat > "$MCP_CONFIG" <<EOF
{
  "mcpServers": {
    "detritus": {
      "command": "${BINARY_PATH_JSON}",
      "args": [],
      "disabled": false
    }
  }
}
EOF
  echo "Created ${MCP_CONFIG}"
fi

echo ""
echo "MCP config: ${MCP_CONFIG}"
echo "Binary:     ${INSTALL_DIR}/${BINARY_NAME}"
echo ""
echo "Restart Windsurf to activate."

# Auto-configure VS Code
# VS Code uses "servers" (not "mcpServers") in mcp.json
# Prompt files (slash commands) are workspace-level — use 'detritus --init' per repo

configure_vscode_mcp() {
  local VSCODE_DIR="$1"
  if [ ! -d "$VSCODE_DIR" ]; then
    return
  fi

  local VSCODE_MCP="${VSCODE_DIR}/mcp.json"

  if [ -f "$VSCODE_MCP" ]; then
    if command -v python3 >/dev/null 2>&1; then
      python3 -c "
import json, sys
with open('$VSCODE_MCP', 'r') as f:
    config = json.load(f)
config.setdefault('servers', {})
config['servers']['detritus'] = {'command': '$BINARY_PATH_JSON', 'args': []}
with open('$VSCODE_MCP', 'w') as f:
    json.dump(config, f, indent=2)
print('Updated detritus in $VSCODE_MCP')
"
    else
      echo "python3 not found, please add manually to ${VSCODE_MCP}:"
      echo '  "detritus": { "command": "'${BINARY_PATH_JSON}'", "args": [] }'
    fi
  else
    cat > "$VSCODE_MCP" <<EOF
{
  "servers": {
    "detritus": {
      "command": "${BINARY_PATH_JSON}",
      "args": []
    }
  }
}
EOF
    echo "Created ${VSCODE_MCP}"
  fi

  # Clean up old user-level prompt files (no longer used — prompts are workspace-level now)
  local OLD_PROMPTS="${VSCODE_DIR}/prompts"
  if [ -d "$OLD_PROMPTS" ]; then
    # Only remove files that look like detritus-generated prompts (contain kb_get)
    for f in "$OLD_PROMPTS"/*.prompt.md; do
      [ -f "$f" ] && grep -q 'kb_get' "$f" 2>/dev/null && rm -f "$f"
    done
    # Remove dir if empty
    rmdir "$OLD_PROMPTS" 2>/dev/null || true
    echo "Cleaned up old user-level prompt files from ${OLD_PROMPTS}/"
  fi

  echo "VS Code MCP config: ${VSCODE_MCP}"
}

# Linux/macOS VS Code locations
if [ "$OS" = "linux" ]; then
  configure_vscode_mcp "$HOME/.config/Code/User"
  configure_vscode_mcp "$HOME/.vscode-server/data/User"
elif [ "$OS" = "darwin" ]; then
  configure_vscode_mcp "$HOME/Library/Application Support/Code/User"
elif [ "$OS" = "windows" ]; then
  WIN_APPDATA_CODE=$(cygpath -u "$APPDATA" 2>/dev/null || echo "$HOME/AppData/Roaming")
  configure_vscode_mcp "${WIN_APPDATA_CODE}/Code/User"
fi

echo ""
echo "VS Code slash commands: run 'detritus --init' in each project to generate .github/prompts/"
echo "Reload VS Code window (Developer: Reload Window) to activate."
