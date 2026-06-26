package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// docEntry holds a doc name, alias, and description.
type docEntry struct {
	name  string
	alias string
	desc  string
}

// listDocEntries walks the embedded docs FS and returns all entries.
func listDocEntries() []docEntry {
	var entries []docEntry
	_ = fs.WalkDir(docsFS, "docs", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		name := strings.TrimSuffix(strings.TrimPrefix(path, "docs/"), ".md")
		content, _ := fs.ReadFile(docsFS, path)
		desc := extractDescription(string(content))
		entries = append(entries, docEntry{
			name:  name,
			alias: aliasForDoc(name),
			desc:  desc,
		})
		return nil
	})
	return entries
}

// homeDir returns the user home directory.
func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "~"
	}
	return h
}

// RunSetup configures all detected IDEs.
// binaryPath is the path to the detritus binary to embed in configs.
// If dryRun is true, nothing is written to disk; actions are only printed.
func RunSetup(binaryPath string, dryRun bool) error {
	docs := listDocEntries()

	if dryRun {
		fmt.Println("[dry-run] No files will be written.")
	}

	home := homeDir()

	// Fetch the candyland sidecar binary beside detritus FIRST, so the per-host
	// MCP registrations below find it and wire up the candyland control-mcp.
	// Best-effort: skips cleanly when there's no release yet.
	fetchCandylandBinary(binaryPath, dryRun)

	// Windsurf
	setupWindsurf(home, binaryPath, docs, dryRun)

	// VS Code
	setupVSCode(home, binaryPath, docs, dryRun)

	// Cursor
	setupCursor(home, binaryPath, dryRun)

	// Claude Code
	setupClaudeCode(home, binaryPath, docs, dryRun)

	// Codex
	setupCodex(home, binaryPath, docs, dryRun)

	// Verdent
	if verdentDetected(home) {
		setupVerdent(home, binaryPath, docs, dryRun)
	} else {
		fmt.Println("Verdent not detected; skipping Verdent setup.")
	}

	// Bootstrap cache: scripts/detritus-mcp.js re-downloads the binary only when
	// the cached file is missing, so invalidate it here or the in-repo MCP server
	// keeps serving the previous version after an update.
	clearCodexCache(home, dryRun)

	// Post-install verification
	if !dryRun {
		printVerification(home)
	}

	return nil
}

// ---- Windsurf ---------------------------------------------------------------

func setupWindsurf(home, binaryPath string, _ []docEntry, dryRun bool) {
	cfgFile := filepath.Join(home, ".codeium", "windsurf", "mcp_config.json")
	if dryRun {
		fmt.Printf("[dry-run] Would upsert detritus into %s (mcpServers)\n", cfgFile)
		previewCandyland(cfgFile+" (mcpServers)", binaryPath)
		return
	}
	if err := os.MkdirAll(filepath.Dir(cfgFile), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "warning: windsurf config dir: %v\n", err)
		return
	}
	upsertMCP(cfgFile, "mcpServers", binaryPath)
	registerCandylandJSON(cfgFile, "mcpServers", binaryPath)
}

// ---- VS Code ----------------------------------------------------------------

func vscodeUserDirs(home string) []string {
	switch runtime.GOOS {
	case "windows":
		appdata := os.Getenv("APPDATA")
		if appdata == "" {
			appdata = filepath.Join(home, "AppData", "Roaming")
		}
		return []string{filepath.Join(appdata, "Code", "User")}
	case "darwin":
		return []string{filepath.Join(home, "Library", "Application Support", "Code", "User")}
	default: // linux
		return []string{
			filepath.Join(home, ".config", "Code", "User"),
			filepath.Join(home, ".vscode-server", "data", "User"),
		}
	}
}

func setupVSCode(home, binaryPath string, docs []docEntry, dryRun bool) {
	dirs := vscodeUserDirs(home)
	for _, dir := range dirs {
		if !dirExists(dir) {
			continue
		}
		if dryRun {
			fmt.Printf("[dry-run] Would upsert detritus into %s/mcp.json (servers)\n", dir)
			previewCandyland(filepath.Join(dir, "mcp.json")+" (servers)", binaryPath)
			fmt.Printf("[dry-run] Would upsert VS Code settings in %s/settings.json\n", dir)
		} else {
			upsertMCP(filepath.Join(dir, "mcp.json"), "servers", binaryPath)
			registerCandylandJSON(filepath.Join(dir, "mcp.json"), "servers", binaryPath)
			upsertVSCodeSettings(filepath.Join(dir, "settings.json"))
			cleanOldUserPrompts(filepath.Join(dir, "prompts"))
		}
	}

	generateSharedPrompts(home, docs, dryRun)
	generateInlineCommandInstructions(home, docs, dryRun)
	generateAgentFile(home, dryRun)
}

func generateSharedPrompts(home string, docs []docEntry, dryRun bool) {
	promptsDir := filepath.Join(home, ".copilot", "prompts")
	if dryRun {
		fmt.Printf("[dry-run] Would write %d prompt files to %s\n", len(docs), promptsDir)
		return
	}
	if err := os.MkdirAll(promptsDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "warning: prompts dir: %v\n", err)
		return
	}
	generated := map[string]bool{}
	for _, doc := range docs {
		if !isFlowDoc(doc.name) {
			continue
		}
		filename := doc.alias + ".prompt.md"
		generated[filename] = true
		content := fmt.Sprintf("---\ndescription: %s\nagent: agent\n---\n\nCall kb_get(name=\"%s\") and follow the instructions in the returned document.\n", doc.desc, doc.name)
		_ = os.WriteFile(filepath.Join(promptsDir, filename), []byte(content), 0o644)
	}
	// Remove stale
	entries, _ := os.ReadDir(promptsDir)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".prompt.md") || generated[e.Name()] {
			continue
		}
		data, _ := os.ReadFile(filepath.Join(promptsDir, e.Name()))
		if strings.Contains(string(data), "kb_get") {
			os.Remove(filepath.Join(promptsDir, e.Name()))
		}
	}
	fmt.Printf("Shared VS Code prompts: %s\n", promptsDir)
}

func generateInlineCommandInstructions(home string, docs []docEntry, dryRun bool) {
	instrDir := filepath.Join(home, ".copilot", "instructions")
	instrFile := filepath.Join(instrDir, "detritus.instructions.md")
	if dryRun {
		fmt.Printf("[dry-run] Would write %s\n", instrFile)
		return
	}
	if err := os.MkdirAll(instrDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "warning: instructions dir: %v\n", err)
		return
	}

	var sb strings.Builder
	sb.WriteString("---\ndescription: detritus knowledge base guardrails and command router\napplyTo: \"**\"\n---\n\n")
	sb.WriteString("## Guardrails\n\n")
	sb.WriteString("Push back when evidence demands it — including against the user. Research (KB via kb_search/kb_get, source code, docs) before asking researchable questions. Prove before acting. Early returns, flat code, no deep nesting. Comments terse: default to none, one line max, only for a non-obvious WHY — never restate the code or paraphrase the name (exported APIs keep a contract doc comment).\n\n")
	sb.WriteString("## Command Tokens\n\n")
	sb.WriteString("When a user message contains one or more detritus command tokens anywhere in the text (for example: /truthseeker, /plan, /testing), treat each token as an explicit request to load the matching knowledge doc.\n\n")
	sb.WriteString("Rules:\n")
	sb.WriteString("1. Detect command tokens anywhere in the message, not only at the beginning.\n")
	sb.WriteString("2. Support multiple tokens in one message; process all of them (deduplicated) in order of appearance.\n")
	sb.WriteString("3. For each detected token, call kb_get(name=\"...\") with the mapped doc name before producing the final answer.\n")
	sb.WriteString("4. If no token is present, do not force a kb_get call from this instruction alone.\n\n")
	sb.WriteString("5. If a slash command token appears but is not in the mapping, call kb_search(query=\"<token-without-slash>\") and then kb_get the best match before answering.\n\n")
	sb.WriteString("Token to doc mapping:\n")
	commandAliases := listCommandAliases()
	for _, doc := range docs {
		if !commandAliases[doc.alias] {
			continue
		}
		fmt.Fprintf(&sb, "- /%s -> %s\n", doc.alias, doc.name)
	}

	_ = os.WriteFile(instrFile, []byte(sb.String()), 0o644)
	fmt.Printf("VS Code shared instructions: %s\n", instrFile)
}

// listCommandAliases returns the slash-command aliases: every doc under
// docs/flows/ (the user-facing surface). flows/ is the single source for what
// becomes a command — core/, roles/, and reference docs are kb_get-only.
func listCommandAliases() map[string]bool {
	out := map[string]bool{}
	_ = fs.WalkDir(docsFS, "docs/flows", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		name := strings.TrimSuffix(strings.TrimPrefix(path, "docs/"), ".md")
		out[aliasForDoc(name)] = true
		return nil
	})
	return out
}

// isFlowDoc reports whether a doc is user-facing — only docs under flows/ are
// surfaced as generated skills/prompts/commands. core/, roles/, and reference
// docs (ooo/, patterns/) are kb_get-only: the MCP engine still serves them, but
// they never appear in a user's command list.
func isFlowDoc(name string) bool {
	return strings.HasPrefix(name, "flows/")
}

func generateAgentFile(home string, dryRun bool) {
	agentsDir := filepath.Join(home, ".copilot", "agents")
	agentFile := filepath.Join(agentsDir, "detritus.agent.md")
	if dryRun {
		fmt.Printf("[dry-run] Would write %s\n", agentFile)
		return
	}
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "warning: agents dir: %v\n", err)
		return
	}
	content := `---
name: detritus
description: Knowledge-enhanced coding agent with truthseeker principles and project-specific guardrails.
tools:
  - detritus
---

# Detritus Agent

You have access to the **detritus MCP server** providing knowledge base tools: ` + "`kb_list`" + `, ` + "`kb_get`" + `, ` + "`kb_search`" + `. Use them to answer questions about testing patterns, Go idioms, and project architecture.

## Always-On Principles

1. **Push back when facts demand it** — including against the user. Do not soften challenges.
2. **Research before asking** — exhaust KB docs (` + "`kb_search`" + `, ` + "`kb_get`" + `), source code, and inline docs before asking the user anything researchable.
3. **Prove before acting** — base conclusions on evidence, not assumptions. Show your reasoning.
4. **Radical honesty** — if something is wrong, unproven, or assumed, say so directly.
5. **Line-of-sight code** — early returns, flat structure, no deep nesting.
6. **Terse comments** — default to no comment; one short line max; only when the WHY is non-obvious, never restating the code or paraphrasing the name; exported APIs keep a contract doc comment.

## Workflow

- For planning tasks, use the ` + "`/plan`" + ` prompt.
- For testing guidance, use the ` + "`/testing`" + ` prompt.
- When uncertain, search the KB first: ` + "`kb_search(query=\"your question\")`" + `.
`
	_ = os.WriteFile(agentFile, []byte(content), 0o644)
	fmt.Printf("Agent file: %s\n", agentFile)
}

func cleanOldUserPrompts(promptsDir string) {
	if !dirExists(promptsDir) {
		return
	}
	entries, _ := os.ReadDir(promptsDir)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".prompt.md") {
			continue
		}
		data, _ := os.ReadFile(filepath.Join(promptsDir, e.Name()))
		if strings.Contains(string(data), "kb_get") {
			os.Remove(filepath.Join(promptsDir, e.Name()))
		}
	}
	os.Remove(promptsDir) // only succeeds if empty
}

// ---- Cursor -----------------------------------------------------------------

func cursorUserDirs(home string) []string {
	switch runtime.GOOS {
	case "windows":
		appdata := os.Getenv("APPDATA")
		if appdata == "" {
			appdata = filepath.Join(home, "AppData", "Roaming")
		}
		return []string{filepath.Join(appdata, "Cursor", "User")}
	case "darwin":
		return []string{filepath.Join(home, "Library", "Application Support", "Cursor", "User")}
	default:
		return []string{filepath.Join(home, ".config", "Cursor", "User")}
	}
}

func setupCursor(home, binaryPath string, dryRun bool) {
	dirs := cursorUserDirs(home)
	for _, dir := range dirs {
		if !dirExists(dir) {
			continue
		}
		cfgFile := filepath.Join(dir, "mcp.json")
		if dryRun {
			fmt.Printf("[dry-run] Would upsert detritus into %s (mcpServers)\n", cfgFile)
			previewCandyland(cfgFile+" (mcpServers)", binaryPath)
		} else {
			upsertMCP(cfgFile, "mcpServers", binaryPath)
			registerCandylandJSON(cfgFile, "mcpServers", binaryPath)
			fmt.Printf("Cursor MCP config: %s\n", cfgFile)
		}
	}
}

// ---- Claude Code -------------------------------------------------------------

func setupClaudeCode(home, binaryPath string, docs []docEntry, dryRun bool) {
	cfgFile := filepath.Join(home, ".claude.json")
	if dryRun {
		fmt.Printf("[dry-run] Would upsert detritus into %s (mcpServers)\n", cfgFile)
		previewCandyland(cfgFile+" (mcpServers)", binaryPath)
		fmt.Printf("[dry-run] Would write %d skill files to %s\n", len(docs), filepath.Join(home, ".claude", "skills"))
		setupClaudeTodoGuard(home, binaryPath, hasTodoDoc(docs), true)
		return
	}
	upsertMCP(cfgFile, "mcpServers", binaryPath)
	registerCandylandJSON(cfgFile, "mcpServers", binaryPath)
	fmt.Printf("Claude Code MCP config: %s\n", cfgFile)

	generateClaudeSkills(home, docs)

	// Enforce the flows/project/todo convention #13 when the /todo family ships: install the
	// PreToolUse write-guard hook (idempotent). If a future build drops /todo,
	// hasTodoDoc is false and any prior guard entry is removed instead.
	setupClaudeTodoGuard(home, binaryPath, hasTodoDoc(docs), false)
}

func generateClaudeSkills(home string, docs []docEntry) {
	skillsDir := filepath.Join(home, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "warning: claude skills dir: %v\n", err)
		return
	}
	generated := map[string]bool{}
	for _, doc := range docs {
		if !isFlowDoc(doc.name) {
			continue
		}
		generated[doc.alias] = true
		skillDir := filepath.Join(skillsDir, doc.alias)
		_ = os.MkdirAll(skillDir, 0o755)
		desc := doc.desc
		if desc == "" {
			desc = "Detritus knowledge base document: " + doc.name
		}
		content := fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\nCall kb_get with name=\"%s\" and follow the instructions in the returned document.\n", doc.alias, desc, doc.name)
		_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644)
	}
	// Prune detritus-generated skills whose backing doc was removed in this
	// release, so a deleted doc doesn't leave a dangling skill that kb_gets a
	// missing document. Match only our own generated marker so hand-authored
	// skills are never touched. Mirrors the stale-removal in generateCodexSkills
	// and generateVerdentSkills.
	entries, _ := os.ReadDir(skillsDir)
	for _, e := range entries {
		if !e.IsDir() || generated[e.Name()] {
			continue
		}
		sf := filepath.Join(skillsDir, e.Name(), "SKILL.md")
		data, err := os.ReadFile(sf)
		if err == nil && strings.Contains(string(data), "Call kb_get with name=\"") {
			os.RemoveAll(filepath.Join(skillsDir, e.Name()))
		}
	}
	fmt.Printf("Claude Code skills: %s\n", skillsDir)
}

// ---- Codex ------------------------------------------------------------------

func setupCodex(home, binaryPath string, docs []docEntry, dryRun bool) {
	codexDir := filepath.Join(home, ".codex")
	if !dirExists(codexDir) {
		fmt.Println("Codex not detected; skipping Codex setup.")
		return
	}

	skillsDir := filepath.Join(codexDir, "skills")
	configFile := filepath.Join(codexDir, "config.toml")
	if dryRun {
		fmt.Printf("[dry-run] Would upsert detritus into %s (mcp_servers)\n", configFile)
		previewCandyland(configFile+" (mcp_servers.candyland)", binaryPath)
		fmt.Printf("[dry-run] Would write %d Codex skill files to %s\n", len(docs), skillsDir)
		return
	}

	upsertCodexMCPConfig(configFile, binaryPath)
	generateCodexSkills(skillsDir, docs)
	fmt.Printf("Codex MCP config: %s\n", configFile)
	fmt.Printf("Codex skills: %s\n", skillsDir)
}

func upsertCodexMCPConfig(file, command string) {
	content := ""
	if raw, err := os.ReadFile(file); err == nil {
		content = string(raw)
	}

	content = upsertTOMLTable(content, "mcp_servers.detritus", []string{
		"command = " + tomlString(command),
		"args = []",
	})

	// Register the candyland control-mcp alongside detritus when its binary is
	// installed beside detritus (the detritus installer fetches it).
	if cbin, ok := candylandBinFor(command); ok {
		content = upsertTOMLTable(content, "mcp_servers.candyland", []string{
			"command = " + tomlString(cbin),
			`args = ["control-mcp"]`,
		})
	}

	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create Codex config directory: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write %s: %v\n", file, err)
		os.Exit(1)
	}
}

func upsertTOMLTable(content, table string, body []string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.TrimRight(content, "\n")

	header := "[" + table + "]"
	blockLines := append([]string{header}, body...)
	block := strings.Join(blockLines, "\n")

	if content == "" {
		return block + "\n"
	}

	lines := strings.Split(content, "\n")
	lines, inlineParentIndex := removeTOMLTableForms(lines, table)

	if inlineParentIndex >= 0 {
		parent, child, _ := strings.Cut(table, ".")
		lines[inlineParentIndex] = upsertInlineTOMLTableValue(lines[inlineParentIndex], parent, child, body)
		return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
	}

	content = strings.TrimRight(strings.Join(lines, "\n"), "\n")
	if content == "" {
		return block + "\n"
	}
	return content + "\n\n" + block + "\n"
}

func removeTOMLTableForms(lines []string, table string) ([]string, int) {
	parent, child, hasChild := strings.Cut(table, ".")
	header := "[" + table + "]"
	parentHeader := "[" + parent + "]"
	inlineParentIndex := -1
	section := ""
	filtered := make([]string, 0, len(lines))

	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == header {
			for i+1 < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i+1]), "[") {
				i++
			}
			continue
		}

		if strings.HasPrefix(trimmed, "[") {
			section = trimmed
			filtered = append(filtered, lines[i])
			continue
		}

		if hasChild && section == "" && isInlineTOMLTableAssignment(trimmed, parent) {
			inlineParentIndex = len(filtered)
			filtered = append(filtered, lines[i])
			continue
		}

		if hasChild && section == "" && isTOMLKeyFor(trimmed, table) {
			continue
		}

		if hasChild && section == parentHeader && isTOMLKeyFor(trimmed, child) {
			continue
		}

		filtered = append(filtered, lines[i])
	}

	return filtered, inlineParentIndex
}

func isTOMLKeyFor(trimmed, key string) bool {
	return strings.HasPrefix(trimmed, key+".") || strings.HasPrefix(trimmed, key+" ") || strings.HasPrefix(trimmed, key+"=")
}

func isInlineTOMLTableAssignment(trimmed, key string) bool {
	eq := strings.Index(trimmed, "=")
	if eq < 0 || strings.TrimSpace(trimmed[:eq]) != key {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(trimmed[eq+1:]), "{")
}

func upsertInlineTOMLTableValue(line, parent, child string, body []string) string {
	eq := strings.Index(line, "=")
	if eq < 0 || strings.TrimSpace(line[:eq]) != parent {
		return line
	}

	open := strings.Index(line[eq+1:], "{")
	if open < 0 {
		return line
	}
	open += eq + 1
	close := findMatchingBrace(line, open)
	if close < 0 {
		return line
	}

	entries := splitInlineTOMLEntries(line[open+1 : close])
	kept := entries[:0]
	for _, entry := range entries {
		if inlineTOMLEntryKey(entry) == child {
			continue
		}
		kept = append(kept, entry)
	}
	kept = append(kept, child+" = "+inlineTOMLTableBody(body))

	return line[:open+1] + " " + strings.Join(kept, ", ") + " " + line[close:]
}

func findMatchingBrace(value string, open int) int {
	depth := 0
	inString := false
	escaped := false
	for i := open; i < len(value); i++ {
		ch := value[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func splitInlineTOMLEntries(value string) []string {
	var entries []string
	start := 0
	depth := 0
	inString := false
	escaped := false
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		case ',':
			if depth == 0 {
				if entry := strings.TrimSpace(value[start:i]); entry != "" {
					entries = append(entries, entry)
				}
				start = i + 1
			}
		}
	}
	if entry := strings.TrimSpace(value[start:]); entry != "" {
		entries = append(entries, entry)
	}
	return entries
}

func inlineTOMLEntryKey(entry string) string {
	eq := strings.Index(entry, "=")
	if eq < 0 {
		return ""
	}
	return strings.TrimSpace(entry[:eq])
}

func inlineTOMLTableBody(body []string) string {
	return "{ " + strings.Join(body, ", ") + " }"
}

func tomlString(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, "\r", "\\r")
	return "\"" + value + "\""
}

func generateCodexSkills(skillsDir string, docs []docEntry) {
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "warning: codex skills dir: %v\n", err)
		return
	}

	generated := map[string]bool{}
	for _, doc := range docs {
		if !isFlowDoc(doc.name) {
			continue
		}
		generated[doc.alias] = true
		skillDir := filepath.Join(skillsDir, doc.alias)
		_ = os.MkdirAll(skillDir, 0o755)
		desc := doc.desc
		if desc == "" {
			desc = "Detritus knowledge base document: " + doc.name
		}
		content := fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\nCall the detritus MCP tool `kb_get` with name=\"%s\" and follow the instructions in the returned document.\n", doc.alias, desc, doc.name)
		_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644)
	}

	entries, _ := os.ReadDir(skillsDir)
	for _, e := range entries {
		if !e.IsDir() || generated[e.Name()] {
			continue
		}
		sf := filepath.Join(skillsDir, e.Name(), "SKILL.md")
		data, err := os.ReadFile(sf)
		if err == nil && strings.Contains(string(data), "detritus MCP tool `kb_get`") {
			os.RemoveAll(filepath.Join(skillsDir, e.Name()))
		}
	}
}

// ---- Verdent ----------------------------------------------------------------

func verdentDetected(home string) bool {
	if dirExists(filepath.Join(home, ".verdent")) {
		return true
	}
	// Check vscode extensions for verdent
	for _, extDir := range []string{
		filepath.Join(home, ".vscode", "extensions"),
		filepath.Join(home, ".vscode-server", "extensions"),
	} {
		if !dirExists(extDir) {
			continue
		}
		entries, _ := os.ReadDir(extDir)
		for _, e := range entries {
			if strings.Contains(strings.ToLower(e.Name()), "verdent") {
				return true
			}
		}
	}
	return false
}

func setupVerdent(home, binaryPath string, docs []docEntry, dryRun bool) {
	verdentDir := filepath.Join(home, ".verdent")
	mcpFile := filepath.Join(verdentDir, "mcp.json")
	rulesFile := filepath.Join(verdentDir, "VERDENT.md")
	skillsDir := filepath.Join(verdentDir, "skills")

	if dryRun {
		fmt.Printf("[dry-run] Would upsert detritus into %s (mcpServers)\n", mcpFile)
		previewCandyland(mcpFile+" (mcpServers)", binaryPath)
		fmt.Printf("[dry-run] Would upsert DETRITUS-RULES block in %s\n", rulesFile)
		fmt.Printf("[dry-run] Would write %d skill files to %s\n", len(docs), skillsDir)
		return
	}

	if err := os.MkdirAll(verdentDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "warning: verdent dir: %v\n", err)
		return
	}

	// MCP config
	upsertMCPJSON(mcpFile, "mcpServers", binaryPath)
	registerCandylandJSON(mcpFile, "mcpServers", binaryPath)

	// VERDENT.md rules block
	upsertVerdentRules(rulesFile, docs)

	// Skills
	generateVerdentSkills(skillsDir, docs)

	fmt.Printf("Verdent MCP config: %s\n", mcpFile)
	fmt.Printf("Verdent rules: %s\n", rulesFile)
	fmt.Printf("Verdent skills: %s\n", skillsDir)
}

// upsertMCPJSON upserts the detritus entry into a JSON file using the Go JSON library
// (unlike upsertMCP in main.go which uses raw string manipulation).
func upsertMCPJSON(file, parentKey, command string) {
	data := map[string]any{}
	if raw, err := os.ReadFile(file); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &data)
	}
	parent, ok := data[parentKey].(map[string]any)
	if !ok {
		parent = map[string]any{}
	}
	parent["detritus"] = map[string]any{"command": command, "args": []any{}}
	data[parentKey] = parent
	out, _ := json.MarshalIndent(data, "", "  ")
	_ = os.WriteFile(file, append(out, '\n'), 0o644)
}

func upsertVerdentRules(rulesFile string, docs []docEntry) {
	var ruleBlock strings.Builder
	ruleBlock.WriteString("<!-- DETRITUS-RULES:START -->\n")
	ruleBlock.WriteString("# Detritus Knowledge Base Rules\n\n")
	ruleBlock.WriteString("- Use the detritus MCP server as the default knowledge source for software-engineering guidance.\n")
	ruleBlock.WriteString("- For architecture, planning, testing, patterns, and ooo ecosystem questions, call detritus kb_get before answering.\n")
	ruleBlock.WriteString("- When uncertain which document to use, call kb_search first and then kb_get for the best match.\n")
	ruleBlock.WriteString("- Keep manual invocation available. If user explicitly asks, support command-style prompts like /plan, /grow, /testing.\n\n")
	ruleBlock.WriteString("Manual command to doc mapping:\n")
	for _, doc := range docs {
		if !isFlowDoc(doc.name) {
			continue
		}
		fmt.Fprintf(&ruleBlock, "- /%s -> %s\n", doc.alias, doc.name)
	}
	ruleBlock.WriteString("<!-- DETRITUS-RULES:END -->")
	block := ruleBlock.String()

	existing := ""
	if data, err := os.ReadFile(rulesFile); err == nil {
		existing = string(data)
	}

	var merged string
	const startTag = "<!-- DETRITUS-RULES:START -->"
	const endTag = "<!-- DETRITUS-RULES:END -->"

	if si := strings.Index(existing, startTag); si >= 0 {
		if ei := strings.Index(existing, endTag); ei >= 0 && ei >= si {
			before := existing[:si]
			after := existing[ei+len(endTag):]
			merged = strings.TrimRight(before, "\n") + "\n" + block + "\n" + strings.TrimLeft(after, "\n")
		}
	} else if existing != "" {
		merged = strings.TrimRight(existing, "\n") + "\n\n" + block + "\n"
	} else {
		merged = block + "\n"
	}

	_ = os.WriteFile(rulesFile, []byte(merged), 0o644)
}

func generateVerdentSkills(skillsDir string, docs []docEntry) {
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "warning: skills dir: %v\n", err)
		return
	}
	generated := map[string]bool{}
	for _, doc := range docs {
		if !isFlowDoc(doc.name) {
			continue
		}
		generated[doc.alias] = true
		skillDir := filepath.Join(skillsDir, doc.alias)
		_ = os.MkdirAll(skillDir, 0o755)
		desc := doc.desc
		if desc == "" {
			desc = "Detritus knowledge base document: " + doc.name
		}
		content := fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\nCall the detritus MCP tool `kb_get` with name=\"%s\" and follow the instructions in the returned document.\n", doc.alias, desc, doc.name)
		_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644)
	}
	// Remove stale
	entries, _ := os.ReadDir(skillsDir)
	for _, e := range entries {
		if !e.IsDir() || generated[e.Name()] {
			continue
		}
		sf := filepath.Join(skillsDir, e.Name(), "SKILL.md")
		data, err := os.ReadFile(sf)
		if err == nil && strings.Contains(string(data), "kb_get") {
			os.RemoveAll(filepath.Join(skillsDir, e.Name()))
		}
	}
}

// ---- Post-install verification ----------------------------------------------

func printVerification(home string) {
	fmt.Println("\nPost-install verification:")

	// Windsurf
	wsFile := filepath.Join(home, ".codeium", "windsurf", "mcp_config.json")
	if fileContains(wsFile, `"detritus"`) {
		fmt.Println("  [PASS] Windsurf MCP entry")
	} else {
		fmt.Println("  [WARN] Windsurf MCP entry not found")
	}

	// VS Code
	vsOK := false
	for _, dir := range vscodeUserDirs(home) {
		if fileContains(filepath.Join(dir, "mcp.json"), `"detritus"`) {
			vsOK = true
			break
		}
	}
	if vsOK {
		fmt.Println("  [PASS] VS Code MCP entry")
	} else {
		fmt.Println("  [WARN] VS Code MCP entry not found")
	}

	// Copilot prompts/instructions
	promptOK := fileExists(filepath.Join(home, ".copilot", "prompts", "plan.prompt.md"))
	instrOK := fileExists(filepath.Join(home, ".copilot", "instructions", "detritus.instructions.md"))
	if promptOK && instrOK {
		fmt.Println("  [PASS] Copilot shared prompts/instructions")
	} else {
		fmt.Println("  [WARN] Copilot shared prompts/instructions")
	}

	// Verdent
	if verdentDetected(home) {
		if fileExists(filepath.Join(home, ".verdent", "mcp.json")) && fileExists(filepath.Join(home, ".verdent", "VERDENT.md")) {
			fmt.Println("  [PASS] Verdent MCP/rules")
		} else {
			fmt.Println("  [WARN] Verdent MCP/rules")
		}
		skillsDir := filepath.Join(home, ".verdent", "skills")
		entries, _ := os.ReadDir(skillsDir)
		if len(entries) > 0 {
			fmt.Println("  [PASS] Verdent skills")
		} else {
			fmt.Println("  [WARN] Verdent skills")
		}
	}

	// Claude Code
	claudeFile := filepath.Join(home, ".claude.json")
	if fileContains(claudeFile, `"detritus"`) {
		fmt.Println("  [PASS] Claude Code MCP entry")
	} else {
		fmt.Println("  [WARN] Claude Code MCP entry not found")
	}
	skillsDir := filepath.Join(home, ".claude", "skills")
	if entries, err := os.ReadDir(skillsDir); err == nil && len(entries) > 0 {
		fmt.Println("  [PASS] Claude Code skills")
	} else {
		fmt.Println("  [WARN] Claude Code skills not found")
	}
	if fileExists(filepath.Join(skillsDir, "todo", "SKILL.md")) {
		if fileContains(filepath.Join(home, ".claude", "settings.json"), todoGuardMarker) {
			fmt.Println("  [PASS] Claude Code /todo write-guard hook")
		} else {
			fmt.Println("  [WARN] Claude Code /todo write-guard hook not found")
		}
	}

	// Codex
	codexSkillsDir := filepath.Join(home, ".codex", "skills")
	if fileExists(filepath.Join(codexSkillsDir, "plan", "SKILL.md")) {
		fmt.Println("  [PASS] Codex skills")
	} else if dirExists(filepath.Join(home, ".codex")) {
		fmt.Println("  [WARN] Codex skills not found")
	}
}

// ---- Helpers ----------------------------------------------------------------

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func fileContains(path, substr string) bool {
	data, err := os.ReadFile(path)
	return err == nil && strings.Contains(string(data), substr)
}

// codexCacheBinary returns the path to the cached MCP bootstrap binary that
// scripts/detritus-mcp.js downloads. Mirrors the path resolution in that
// script: DETRITUS_CACHE_DIR overrides everything, otherwise it is the
// platform cache dir plus "detritus-codex".
func codexCacheBinary(home string) string {
	binName := "detritus"
	if runtime.GOOS == "windows" {
		binName = "detritus.exe"
	}
	if dir := os.Getenv("DETRITUS_CACHE_DIR"); dir != "" {
		return filepath.Join(dir, binName)
	}
	return filepath.Join(platformCacheDir(home), "detritus-codex", binName)
}

// platformCacheDir mirrors defaultCacheDir() in scripts/detritus-mcp.js.
func platformCacheDir(home string) string {
	switch runtime.GOOS {
	case "windows":
		if appdata := os.Getenv("LOCALAPPDATA"); appdata != "" {
			return appdata
		}
		return filepath.Join(home, "AppData", "Local")
	case "darwin":
		return filepath.Join(home, "Library", "Caches")
	default:
		if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
			return xdg
		}
		return filepath.Join(home, ".cache")
	}
}

// clearCodexCache deletes the cached MCP bootstrap binary so the next launch of
// scripts/detritus-mcp.js re-fetches the freshly installed version. The
// bootstrap only re-downloads when the file is missing, so this is what makes
// an update actually reach an MCP client whose cwd is the detritus repo.
func clearCodexCache(home string, dryRun bool) {
	binPath := codexCacheBinary(home)
	if !fileExists(binPath) {
		return
	}
	if dryRun {
		fmt.Printf("[dry-run] Would clear stale MCP bootstrap cache %s\n", binPath)
		return
	}
	if err := os.Remove(binPath); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not clear cached MCP binary %s: %v\n", binPath, err)
		return
	}
	fmt.Printf("Cleared stale MCP bootstrap cache: %s\n", binPath)
}
