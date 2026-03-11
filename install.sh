#!/bin/sh
set -e

REPO="benitogf/detritus"
INSTALL_DIR="/usr/local/bin"
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

# Install
if [ -w "$INSTALL_DIR" ]; then
  mv "${TMP}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
else
  echo "Installing to ${INSTALL_DIR} (requires sudo)..."
  sudo mv "${TMP}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
fi
chmod +x "${INSTALL_DIR}/${BINARY}"

# Verify binary works
echo "Verifying installation..."
VERIFY=$("${INSTALL_DIR}/${BINARY}" --version 2>&1)
echo "  ${VERIFY}"

echo ""
echo "Installed ${BINARY} ${VERSION} to ${INSTALL_DIR}/${BINARY}"

# Auto-configure mcp_config.json
MCP_CONFIG="$HOME/.codeium/windsurf/mcp_config.json"
MCP_DIR=$(dirname "$MCP_CONFIG")
BINARY_PATH="${INSTALL_DIR}/${BINARY}"

mkdir -p "$MCP_DIR"

if [ -f "$MCP_CONFIG" ]; then
  if command -v python3 >/dev/null 2>&1; then
    python3 -c "
import json, sys
with open('$MCP_CONFIG', 'r') as f:
    config = json.load(f)
config.setdefault('mcpServers', {})
config['mcpServers']['detritus'] = {'command': '$BINARY_PATH', 'args': [], 'disabled': False}
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
      "command": "${BINARY_PATH}",
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
echo "Binary:     ${BINARY_PATH}"
echo ""
echo "Restart Windsurf to activate."
