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

# Helper: upsert MCP config entry using the binary, or write fresh file as fallback.
upsert_mcp_or_create() {
  local file="$1" parent_key="$2" cmd_path="$3"
  if "$BINARY_PATH" --upsert-mcp "$file" "$parent_key" "$cmd_path" 2>/dev/null; then
    return
  fi
  # Fallback: write a fresh config (binary may be an older version without --upsert-mcp)
  mkdir -p "$(dirname "$file")"
  cat > "$file" <<EOF
{
  "${parent_key}": {
    "detritus": {
      "command": "${cmd_path}",
      "args": []
    }
  }
}
EOF
  echo "Created ${file}"
}

# Helper: upsert VS Code settings using the binary, or write fresh file as fallback.
upsert_vscode_settings_or_create() {
  local file="$1"
  if "$BINARY_PATH" --upsert-vscode-settings "$file" 2>/dev/null; then
    return
  fi
  # Fallback: write fresh settings
  mkdir -p "$(dirname "$file")"
  cat > "$file" <<EOF
{
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
}
EOF
  echo "Created ${file}"
}

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

upsert_mcp_or_create "$MCP_CONFIG" mcpServers "$BINARY_PATH_JSON"

echo ""
echo "MCP config: ${MCP_CONFIG}"
echo "Binary:     ${INSTALL_DIR}/${BINARY_NAME}"
echo ""
echo "Restart Windsurf to activate."

# Auto-configure VS Code
# VS Code uses "servers" (not "mcpServers") in mcp.json
# Prompt files (slash commands) are loaded from one shared folder to avoid duplicates in multi-root workspaces

vscode_alias_for_doc() {
  local name="$1"
  local leaf="${name##*/}"
  case "$name" in
    plan/analyze)         echo "plan" ;;
    plan/export)          echo "plan-export" ;;
    plan/diagrams)        echo "diagrams" ;;
    testing/index)        echo "testing" ;;
    testing/go-backend-*) echo "testing-${leaf}" ;;
    ooo/*)                echo "ooo-${leaf}" ;;
    *)                    echo "$leaf" ;;
  esac
}

generate_shared_prompts() {
  local SHARED_PROMPTS_DIR="$HOME/.copilot/prompts"
  local GENERATED_LIST="${TMP}/generated_prompts.txt"
  mkdir -p "$SHARED_PROMPTS_DIR"
  : > "$GENERATED_LIST"

  tab=$(printf '\t')
  while IFS="$tab" read -r name desc; do
    [ -z "$name" ] && continue
    local alias
    alias=$(vscode_alias_for_doc "$name")
    local filename="${alias}.prompt.md"
    local file="${SHARED_PROMPTS_DIR}/${filename}"

    cat > "$file" <<EOF
---
description: ${desc}
agent: agent
---

Call kb_get(name="${name}") and follow the instructions in the returned document.
EOF
    echo "$filename" >> "$GENERATED_LIST"
  done << DOCLIST
$($BINARY_PATH --list 2>/dev/null)
DOCLIST

  # Remove stale detritus-generated prompts (preserve user prompts that are unrelated)
  for f in "$SHARED_PROMPTS_DIR"/*.prompt.md; do
    [ -f "$f" ] || continue
    base=$(basename "$f")
    if grep -qx "$base" "$GENERATED_LIST"; then
      continue
    fi
    if grep -q 'kb_get(name="' "$f" 2>/dev/null; then
      rm -f "$f"
    fi
  done

  echo "Shared VS Code prompts: ${SHARED_PROMPTS_DIR}/"
}

generate_inline_command_instructions() {
  local INSTR_DIR="$HOME/.copilot/instructions"
  local INSTR_FILE="${INSTR_DIR}/detritus.instructions.md"
  mkdir -p "$INSTR_DIR"

  {
    echo "---"
    echo "description: detritus knowledge base guardrails and command router"
    echo "applyTo: \"**\""
    echo "---"
    echo ""
    echo "## Guardrails"
    echo ""
    echo "Push back when evidence demands it — including against the user. Research (KB via kb_search/kb_get, source code, docs) before asking researchable questions. Prove before acting. Early returns, flat code, no deep nesting."
    echo ""
    echo "## Command Tokens"
    echo ""
    echo "When a user message contains one or more detritus command tokens anywhere in the text (for example: /truthseeker, /plan, /testing), treat each token as an explicit request to load the matching knowledge doc."
    echo ""
    echo "Rules:"
    echo "1. Detect command tokens anywhere in the message, not only at the beginning."
    echo "2. Support multiple tokens in one message; process all of them (deduplicated) in order of appearance."
    echo "3. For each detected token, call kb_get(name=\"...\") with the mapped doc name before producing the final answer."
    echo "4. If no token is present, do not force a kb_get call from this instruction alone."
    echo ""
    echo "Token to doc mapping:"
    "$BINARY_PATH" --list 2>/dev/null | while IFS=$(printf '\t') read -r name _desc; do
      [ -z "$name" ] && continue
      alias=$(vscode_alias_for_doc "$name")
      echo "- /${alias} -> ${name}"
    done
  } > "$INSTR_FILE"

  echo "VS Code shared instructions: ${INSTR_FILE}"
}

generate_agent_file() {
  local AGENTS_DIR="$HOME/.copilot/agents"
  mkdir -p "$AGENTS_DIR"

  cat > "$AGENTS_DIR/detritus.agent.md" <<'AGENT_EOF'
---
name: detritus
description: Knowledge-enhanced coding agent with ooo ecosystem expertise, truthseeker principles, and project-specific guardrails.
tools:
  - detritus
---

# Detritus Agent

You have access to the **detritus MCP server** providing knowledge base tools: `kb_list`, `kb_get`, `kb_search`. Use them to answer questions about the ooo ecosystem, testing patterns, Go idioms, and project architecture.

## Always-On Principles

1. **Push back when facts demand it** — including against the user. Do not soften challenges.
2. **Research before asking** — exhaust KB docs (`kb_search`, `kb_get`), source code, and inline docs before asking the user anything researchable.
3. **Prove before acting** — base conclusions on evidence, not assumptions. Show your reasoning.
4. **Radical honesty** — if something is wrong, unproven, or assumed, say so directly.
5. **Line-of-sight code** — early returns, flat structure, no deep nesting.

## Workflow

- For planning tasks, use the `/plan` prompt followed by `/plan-export` for documents.
- For scaffolding, use the `/create` prompt.
- For testing guidance, use the `/testing` prompt.
- When uncertain about ooo internals, search the KB first: `kb_search(query="your question")`.
AGENT_EOF

  echo "Agent file: ${AGENTS_DIR}/detritus.agent.md"
}

continue_is_installed() {
  if command -v cn >/dev/null 2>&1; then
    return 0
  fi
  if [ -d "$HOME/.continue" ]; then
    return 0
  fi
  if [ -d "$HOME/.vscode-server/extensions" ] && ls "$HOME/.vscode-server/extensions"/*continue* >/dev/null 2>&1; then
    return 0
  fi
  if [ -d "$HOME/.vscode/extensions" ] && ls "$HOME/.vscode/extensions"/*continue* >/dev/null 2>&1; then
    return 0
  fi
  return 1
}

configure_continue() {
  local CONTINUE_DIR="$HOME/.continue"
  local MCP_DIR="${CONTINUE_DIR}/mcpServers"
  local PROMPTS_DIR="${CONTINUE_DIR}/prompts"
  local GENERATED_LIST="${TMP}/continue_generated_prompts.txt"

  mkdir -p "$MCP_DIR" "$PROMPTS_DIR"
  : > "$GENERATED_LIST"

  cat > "${MCP_DIR}/detritus.yaml" <<EOF
name: detritus-local
version: 0.0.1
schema: v1
mcpServers:
  - name: detritus
    command: ${BINARY_PATH_JSON}
    args: []
EOF

  tab=$(printf '\t')
  while IFS="$tab" read -r name _desc; do
    [ -z "$name" ] && continue
    alias=$(vscode_alias_for_doc "$name")
    file="${PROMPTS_DIR}/${alias}.prompt"
    cat > "$file" <<EOF
name: ${alias}
description: Load detritus knowledge doc ${name}
invokable: true
---
Use the detritus MCP server and call kb_get with name="${name}". Then follow the returned guidance strictly.
EOF
    echo "${alias}.prompt" >> "$GENERATED_LIST"
  done << DOCLIST
$($BINARY_PATH --list 2>/dev/null)
DOCLIST

  {
    echo "name: detritus-help"
    echo "description: List all detritus slash commands"
    echo "invokable: true"
    echo "---"
    echo "Available detritus commands:"
    for f in "$PROMPTS_DIR"/*.prompt; do
      [ -f "$f" ] || continue
      base=$(basename "$f" .prompt)
      echo "$base"
    done | sort -u | sed 's#^#- /#'
  } > "${PROMPTS_DIR}/detritus-help.prompt"

  # Remove stale detritus-generated prompt files while preserving unrelated user prompts.
  for f in "$PROMPTS_DIR"/*.prompt; do
    [ -f "$f" ] || continue
    base=$(basename "$f")
    if [ "$base" = "detritus-help.prompt" ]; then
      continue
    fi
    if grep -qx "$base" "$GENERATED_LIST"; then
      continue
    fi
    if grep -q 'Use the detritus MCP server and call kb_get with name=' "$f" 2>/dev/null; then
      rm -f "$f"
    fi
  done

  echo "Continue MCP config: ${MCP_DIR}/detritus.yaml"
  echo "Continue prompts: ${PROMPTS_DIR}/"
}

verdent_is_installed() {
  if [ -d "$HOME/.verdent" ]; then
    return 0
  fi
  if [ -d "$HOME/.vscode-server/extensions" ] && ls "$HOME/.vscode-server/extensions"/*verdent* >/dev/null 2>&1; then
    return 0
  fi
  if [ -d "$HOME/.vscode/extensions" ] && ls "$HOME/.vscode/extensions"/*verdent* >/dev/null 2>&1; then
    return 0
  fi
  return 1
}

configure_verdent() {
  local VERDENT_DIR="$HOME/.verdent"
  local VERDENT_MCP="${VERDENT_DIR}/mcp.json"
  local VERDENT_RULES="${VERDENT_DIR}/VERDENT.md"
  mkdir -p "$VERDENT_DIR"

  upsert_mcp_or_create "$VERDENT_MCP" mcpServers "$BINARY_PATH_JSON"

  local RULE_BLOCK_FILE="${TMP}/verdent_detritus_rules.md"
  {
    echo "<!-- DETRITUS-RULES:START -->"
    echo "# Detritus Knowledge Base Rules"
    echo ""
    echo "- Use the detritus MCP server as the default knowledge source for software-engineering guidance."
    echo "- For architecture, planning, testing, patterns, and ooo ecosystem questions, call detritus kb_get before answering."
    echo "- When uncertain which document to use, call kb_search first and then kb_get for the best match."
    echo "- Keep manual invocation available. If user explicitly asks, support command-style prompts like /plan, /grow, /create, /testing."
    echo ""
    echo "Manual command to doc mapping:"
    "$BINARY_PATH" --list 2>/dev/null | while IFS=$(printf '\t') read -r name _desc; do
      [ -z "$name" ] && continue
      alias=$(vscode_alias_for_doc "$name")
      echo "- /${alias} -> ${name}"
    done
    echo "<!-- DETRITUS-RULES:END -->"
  } > "$RULE_BLOCK_FILE"

  if [ -f "$VERDENT_RULES" ]; then
    if grep -q '<!-- DETRITUS-RULES:START -->' "$VERDENT_RULES"; then
      awk '
        BEGIN {inblock=0}
        /<!-- DETRITUS-RULES:START -->/ {inblock=1; next}
        /<!-- DETRITUS-RULES:END -->/ {inblock=0; next}
        !inblock {print}
      ' "$VERDENT_RULES" > "${TMP}/verdent_rules_base.md"
      cat "${TMP}/verdent_rules_base.md" "$RULE_BLOCK_FILE" > "$VERDENT_RULES"
    else
      {
        cat "$VERDENT_RULES"
        echo ""
        cat "$RULE_BLOCK_FILE"
      } > "${TMP}/verdent_rules_new.md"
      mv "${TMP}/verdent_rules_new.md" "$VERDENT_RULES"
    fi
  else
    cp "$RULE_BLOCK_FILE" "$VERDENT_RULES"
  fi

  # Generate Verdent skills for slash-command support
  local SKILLS_DIR="${VERDENT_DIR}/skills"
  mkdir -p "$SKILLS_DIR"

  local GENERATED_SKILLS="${TMP}/generated_verdent_skills.txt"
  : > "$GENERATED_SKILLS"

  tab=$(printf '\t')
  "$BINARY_PATH" --list 2>/dev/null | while IFS="$tab" read -r name desc; do
    [ -z "$name" ] && continue
    local alias
    alias=$(vscode_alias_for_doc "$name")
    echo "$alias" >> "$GENERATED_SKILLS"
    [ -z "$desc" ] && desc="Detritus knowledge base document: ${name}"

    local skill_dir="${SKILLS_DIR}/${alias}"
    mkdir -p "$skill_dir"
    cat > "${skill_dir}/SKILL.md" <<SKILLEOF
---
name: ${alias}
description: ${desc}
---

Call the detritus MCP tool \`\`kb_get\`\` with name="${name}" and follow the instructions in the returned document.
SKILLEOF
  done

  # Remove stale detritus-generated skills
  for d in "$SKILLS_DIR"/*/; do
    [ -d "$d" ] || continue
    local skill_name
    skill_name=$(basename "$d")
    if ! grep -qx "$skill_name" "$GENERATED_SKILLS" 2>/dev/null; then
      if [ -f "${d}SKILL.md" ] && grep -q 'kb_get' "${d}SKILL.md" 2>/dev/null; then
        rm -rf "$d"
      fi
    fi
  done

  echo "Verdent MCP config: ${VERDENT_MCP}"
  echo "Verdent rules: ${VERDENT_RULES}"
  echo "Verdent skills: ${SKILLS_DIR}"
}

configure_vscode_mcp() {
  local VSCODE_DIR="$1"
  if [ ! -d "$VSCODE_DIR" ]; then
    return
  fi

  local VSCODE_MCP="${VSCODE_DIR}/mcp.json"

  upsert_mcp_or_create "$VSCODE_MCP" servers "$BINARY_PATH_JSON"

  # Configure a single prompt source to avoid duplicate slash commands in multi-root workspaces.
  local VSCODE_SETTINGS="${VSCODE_DIR}/settings.json"
  upsert_vscode_settings_or_create "$VSCODE_SETTINGS"

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

generate_shared_prompts
generate_inline_command_instructions
generate_agent_file

if continue_is_installed; then
  configure_continue
else
  echo "Continue not detected; skipping Continue prompt/MCP setup."
fi

if verdent_is_installed; then
  configure_verdent
else
  echo "Verdent not detected; skipping Verdent MCP/rules setup."
fi

echo ""
echo "Post-install verification:"

if [ -f "$HOME/.codeium/windsurf/mcp_config.json" ] && grep -q '"detritus"' "$HOME/.codeium/windsurf/mcp_config.json" 2>/dev/null; then
  echo "  [PASS] Windsurf MCP entry"
else
  echo "  [WARN] Windsurf MCP entry"
fi

VSCODE_OK=0
for f in "$HOME/.config/Code/User/mcp.json" "$HOME/.vscode-server/data/User/mcp.json"; do
  if [ -f "$f" ] && grep -q '"detritus"' "$f" 2>/dev/null; then
    VSCODE_OK=1
    break
  fi
done
if [ "$VSCODE_OK" -eq 1 ]; then
  echo "  [PASS] VS Code MCP entry"
else
  echo "  [WARN] VS Code MCP entry"
fi

if [ -f "$HOME/.copilot/prompts/plan.prompt.md" ] && [ -f "$HOME/.copilot/instructions/detritus.instructions.md" ]; then
  echo "  [PASS] Copilot shared prompts/instructions"
else
  echo "  [WARN] Copilot shared prompts/instructions"
fi

if continue_is_installed; then
  if [ -f "$HOME/.continue/mcpServers/detritus.yaml" ]; then
    echo "  [PASS] Continue MCP config"
  else
    echo "  [WARN] Continue MCP config"
  fi
fi

if verdent_is_installed; then
  if [ -f "$HOME/.verdent/mcp.json" ] && [ -f "$HOME/.verdent/VERDENT.md" ]; then
    echo "  [PASS] Verdent MCP/rules"
  else
    echo "  [WARN] Verdent MCP/rules"
  fi
  if [ -d "$HOME/.verdent/skills" ] && [ "$(ls -A "$HOME/.verdent/skills" 2>/dev/null)" ]; then
    echo "  [PASS] Verdent skills"
  else
    echo "  [WARN] Verdent skills"
  fi
fi

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

# Auto-configure Cursor MCP
configure_cursor_mcp() {
  local CURSOR_DIR="$1"
  if [ ! -d "$CURSOR_DIR" ]; then
    return
  fi

  local CURSOR_MCP="${CURSOR_DIR}/mcp.json"

  upsert_mcp_or_create "$CURSOR_MCP" mcpServers "$BINARY_PATH_JSON"

  echo "Cursor MCP config: ${CURSOR_MCP}"
}

# Cursor config locations
if [ "$OS" = "linux" ]; then
  configure_cursor_mcp "$HOME/.config/Cursor/User"
elif [ "$OS" = "darwin" ]; then
  configure_cursor_mcp "$HOME/Library/Application Support/Cursor/User"
elif [ "$OS" = "windows" ]; then
  WIN_APPDATA_CURSOR=$(cygpath -u "$APPDATA" 2>/dev/null || echo "$HOME/AppData/Roaming")
  configure_cursor_mcp "${WIN_APPDATA_CURSOR}/Cursor/User"
fi

echo ""
echo "VS Code slash commands: loaded from ~/.copilot/prompts/ (shared across workspaces)"
echo "Inline detritus tokens: use multiple commands anywhere in one message (example: '/truthseeker ... /plan')."
echo "Continue integration: if Continue is installed, installer writes ~/.continue/mcpServers + ~/.continue/prompts."
echo "Cursor integration: MCP config written to Cursor User directory."
echo "Verdent integration: if Verdent is installed, installer writes ~/.verdent/mcp.json + ~/.verdent/VERDENT.md + ~/.verdent/skills/."
echo "Optional: run 'detritus --init' in a repo if you specifically want repo-local prompt files."
echo "Reload VS Code window (Developer: Reload Window) to activate."
