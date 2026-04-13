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

	// Windsurf
	setupWindsurf(home, binaryPath, docs, dryRun)

	// VS Code
	setupVSCode(home, binaryPath, docs, dryRun)

	// Cursor
	setupCursor(home, binaryPath, dryRun)

	// Claude Code
	setupClaudeCode(home, binaryPath, dryRun)

	// Verdent
	if verdentDetected(home) {
		setupVerdent(home, binaryPath, docs, dryRun)
	} else {
		fmt.Println("Verdent not detected; skipping Verdent setup.")
	}

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
		return
	}
	if err := os.MkdirAll(filepath.Dir(cfgFile), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "warning: windsurf config dir: %v\n", err)
		return
	}
	upsertMCP(cfgFile, "mcpServers", binaryPath)
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
			fmt.Printf("[dry-run] Would upsert VS Code settings in %s/settings.json\n", dir)
		} else {
			upsertMCP(filepath.Join(dir, "mcp.json"), "servers", binaryPath)
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
	sb.WriteString("Push back when evidence demands it — including against the user. Research (KB via kb_search/kb_get, source code, docs) before asking researchable questions. Prove before acting. Early returns, flat code, no deep nesting.\n\n")
	sb.WriteString("## Command Tokens\n\n")
	sb.WriteString("When a user message contains one or more detritus command tokens anywhere in the text (for example: /truthseeker, /plan, /testing), treat each token as an explicit request to load the matching knowledge doc.\n\n")
	sb.WriteString("Rules:\n")
	sb.WriteString("1. Detect command tokens anywhere in the message, not only at the beginning.\n")
	sb.WriteString("2. Support multiple tokens in one message; process all of them (deduplicated) in order of appearance.\n")
	sb.WriteString("3. For each detected token, call kb_get(name=\"...\") with the mapped doc name before producing the final answer.\n")
	sb.WriteString("4. If no token is present, do not force a kb_get call from this instruction alone.\n\n")
	sb.WriteString("Token to doc mapping:\n")
	for _, doc := range docs {
		fmt.Fprintf(&sb, "- /%s -> %s\n", doc.alias, doc.name)
	}

	_ = os.WriteFile(instrFile, []byte(sb.String()), 0o644)
	fmt.Printf("VS Code shared instructions: %s\n", instrFile)
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
		} else {
			upsertMCP(cfgFile, "mcpServers", binaryPath)
			fmt.Printf("Cursor MCP config: %s\n", cfgFile)
		}
	}
}

// ---- Claude Code -------------------------------------------------------------

func setupClaudeCode(home, binaryPath string, dryRun bool) {
	cfgFile := filepath.Join(home, ".claude", "mcp.json")
	if dryRun {
		fmt.Printf("[dry-run] Would upsert detritus into %s (mcpServers)\n", cfgFile)
		return
	}
	if err := os.MkdirAll(filepath.Dir(cfgFile), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "warning: claude config dir: %v\n", err)
		return
	}
	upsertMCP(cfgFile, "mcpServers", binaryPath)
	fmt.Printf("Claude Code MCP config: %s\n", cfgFile)
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
	claudeFile := filepath.Join(home, ".claude", "mcp.json")
	if fileContains(claudeFile, `"detritus"`) {
		fmt.Println("  [PASS] Claude Code MCP entry")
	} else {
		fmt.Println("  [WARN] Claude Code MCP entry not found")
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
