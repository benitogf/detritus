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
# Prompt files are written to the user-level prompts dir — available in all workspaces

# Alias name mapping for VS Code prompt files (mirrors setup-detritus.md logic)
vscode_alias_for_doc() {
  local name="$1"
  local leaf="${name##*/}"
  case "$name" in
    plan/analyze)        echo "plan" ;;
    plan/export)         echo "plan-export" ;;
    plan/diagrams)       echo "diagrams" ;;
    testing/index)       echo "testing" ;;
    testing/go-backend-*) echo "testing-${leaf}" ;;
    ooo/*)               echo "ooo-${leaf}" ;;
    *)                   echo "$leaf" ;;
  esac
}

configure_vscode_dir() {
  local VSCODE_DIR="$1"
  if [ ! -d "$VSCODE_DIR" ]; then
    return
  fi

  local VSCODE_MCP="${VSCODE_DIR}/mcp.json"
  mkdir -p "$VSCODE_DIR"

  # Write mcp.json
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

  # Write user-level prompt files (slash commands available in all workspaces)
  local PROMPTS_DIR="${VSCODE_DIR}/prompts"
  mkdir -p "$PROMPTS_DIR"

  tab=$(printf '\t')
  while IFS="$tab" read -r name desc; do
    [ -z "$name" ] && continue
    local alias
    alias=$(vscode_alias_for_doc "$name")
    local file="${PROMPTS_DIR}/${alias}.prompt.md"
    cat > "$file" <<EOF
---
description: ${desc}
agent: agent
tools: ["detritus/*"]
---

Call kb_get(name="${name}") and follow the instructions in the returned document.
EOF
  done << DOCLIST
$("$BINARY_PATH" --list 2>/dev/null)
DOCLIST

  echo "VS Code config: ${VSCODE_MCP}"
  echo "VS Code prompts: ${PROMPTS_DIR}/"
}

# Linux/macOS VS Code locations
if [ "$OS" = "linux" ]; then
  configure_vscode_dir "$HOME/.config/Code/User"
  configure_vscode_dir "$HOME/.vscode-server/data/User"
elif [ "$OS" = "darwin" ]; then
  configure_vscode_dir "$HOME/Library/Application Support/Code/User"
elif [ "$OS" = "windows" ]; then
  WIN_APPDATA_CODE=$(cygpath -u "$APPDATA" 2>/dev/null || echo "$HOME/AppData/Roaming")
  configure_vscode_dir "${WIN_APPDATA_CODE}/Code/User"
fi

echo ""
echo "Reload VS Code window (Developer: Reload Window) to activate."
