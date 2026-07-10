package main

import (
	"encoding/json"
	"fmt"
	"io"
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

// binaryName is the on-disk name of the detritus executable for this platform.
func binaryName() string {
	if runtime.GOOS == "windows" {
		return "detritus.exe"
	}
	return "detritus"
}

// installBinDir is the stable, per-user directory detritus places itself into
// when it is not already reachable on PATH. It never requires elevation:
// ~/.local/bin on Unix, %LOCALAPPDATA%\detritus on Windows.
func installBinDir() string {
	if runtime.GOOS == "windows" {
		if la := os.Getenv("LOCALAPPDATA"); la != "" {
			return filepath.Join(la, "detritus")
		}
		return filepath.Join(homeDir(), "AppData", "Local", "detritus")
	}
	return filepath.Join(homeDir(), ".local", "bin")
}

// dirOnPath reports whether dir is already an entry in the PATH environment
// variable (order- and duplicate-insensitive).
func dirOnPath(dir string) bool {
	clean := filepath.Clean(dir)
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if p != "" && filepath.Clean(p) == clean {
			return true
		}
	}
	return false
}

// selfPlace makes the running binary reachable on PATH and returns the path the
// rest of setup should reference. If detritus is already on PATH (e.g. it was
// installed to a directory that is), it is left in place. Otherwise it is copied
// into installBinDir() and that dir is added to PATH. Best-effort: on any
// failure it warns and falls back to the current path so setup still proceeds.
func selfPlace(binaryPath string, dryRun bool) string {
	if dirOnPath(filepath.Dir(binaryPath)) {
		return binaryPath
	}

	dir := installBinDir()
	dest := filepath.Join(dir, binaryName())

	if dryRun {
		fmt.Printf("[dry-run] Would copy detritus to %s\n", dest)
		fmt.Printf("[dry-run] Would ensure %s is on PATH\n", dir)
		return dest
	}

	if binaryPath != dest {
		if err := copyBinary(binaryPath, dest); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not place detritus at %s: %v\n", dest, err)
			return binaryPath
		}
		fmt.Printf("Installed detritus binary: %s\n", dest)
	}
	ensureDirOnPath(dir)
	return dest
}

// copyBinary copies src to dst (creating dst's directory) with the executable
// bit set, writing to a temp file first so a partial copy never leaves a broken
// binary at dst.
func copyBinary(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	tmp := dst + ".new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

// ensureDirOnPath persists dir onto the user's PATH when it isn't already there.
// Unix appends an export line to the shell profile(s); Windows updates the
// persistent user PATH via setx. No-op if dir is already on PATH.
func ensureDirOnPath(dir string) {
	if dirOnPath(dir) {
		return
	}
	if runtime.GOOS == "windows" {
		if err := addDirToWindowsUserPath(dir); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not add %s to PATH: %v\n", dir, err)
			return
		}
		fmt.Printf("Added %s to user PATH (restart terminal for effect)\n", dir)
		return
	}
	ensureUnixProfilePath(dir)
}

// ensureUnixProfilePath appends an idempotent PATH export for dir to the user's
// shell profiles. It always ensures ~/.profile, and additionally updates
// ~/.bashrc and ~/.zshrc when they already exist.
func ensureUnixProfilePath(dir string) {
	home := homeDir()
	const marker = "# detritus PATH"
	line := fmt.Sprintf("export PATH=\"%s:$PATH\"", dir)
	block := fmt.Sprintf("\n%s\n%s\n", marker, line)

	updated := false
	for _, name := range []string{".profile", ".bashrc", ".zshrc"} {
		p := filepath.Join(home, name)
		data, err := os.ReadFile(p)
		if err != nil && name != ".profile" {
			continue // only create ~/.profile; touch the others only if present
		}
		if strings.Contains(string(data), marker) || strings.Contains(string(data), dir) {
			updated = true
			continue
		}
		f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			continue
		}
		_, _ = f.WriteString(block)
		f.Close()
		updated = true
	}
	if updated {
		fmt.Printf("Added %s to PATH in your shell profile (restart your shell)\n", dir)
	}
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

	// Self-place: copy this binary into a stable, PATH-reachable install
	// directory and ensure that directory is on PATH, so running `detritus
	// --setup` from a freshly downloaded binary is enough to bootstrap the CLI.
	// Everything downstream (host configs, companion fetches) points at the
	// placed binary.
	binaryPath = selfPlace(binaryPath, dryRun)

	// Fetch the candyland sidecar binary beside detritus so the launcher can
	// start it on demand (the build flows drive it over REST; it is not
	// registered as an MCP). Best-effort: skips cleanly when there's no release yet.
	fetchCandylandBinary(binaryPath, dryRun)

	// Companion binaries for the /consult flow (Typst render, D2 diagrams),
	// installed beside detritus. Best-effort like the candyland fetch.
	fetchTypstBinary(binaryPath, dryRun)
	fetchD2Binary(binaryPath, dryRun)

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
		printVerification(home, binaryPath)
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
	removeMCPServer(cfgFile, "mcpServers", "candyland")
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
			fmt.Printf("[dry-run] Would remove stale candyland MCP entry from %s/mcp.json\n", dir)
			fmt.Printf("[dry-run] Would upsert VS Code settings in %s/settings.json\n", dir)
		} else {
			upsertMCP(filepath.Join(dir, "mcp.json"), "servers", binaryPath)
			removeMCPServer(filepath.Join(dir, "mcp.json"), "servers", "candyland")
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
	// No `tools:` key: Copilot's agent frontmatter treats it as a strict
	// allowlist (omitted = all tools) — listing only the detritus MCP server
	// would strip read/search/edit from a coding agent, the same defect the
	// Claude Code detritus-coder definition had.
	content := `---
name: detritus
description: Knowledge-enhanced coding agent with truthseeker principles and project-specific guardrails.
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
			fmt.Printf("[dry-run] Would remove stale candyland MCP entry from %s\n", cfgFile)
		} else {
			upsertMCP(cfgFile, "mcpServers", binaryPath)
			removeMCPServer(cfgFile, "mcpServers", "candyland")
			fmt.Printf("Cursor MCP config: %s\n", cfgFile)
		}
	}
}

// ---- Claude Code -------------------------------------------------------------

func setupClaudeCode(home, binaryPath string, docs []docEntry, dryRun bool) {
	cfgFile := filepath.Join(home, ".claude.json")
	if dryRun {
		fmt.Printf("[dry-run] Would upsert detritus into %s (mcpServers)\n", cfgFile)
		fmt.Printf("[dry-run] Would remove stale candyland MCP entry from %s\n", cfgFile)
		fmt.Printf("[dry-run] Would write %d skill files to %s\n", len(docs), filepath.Join(home, ".claude", "skills"))
		fmt.Printf("[dry-run] Would write %s\n", filepath.Join(home, ".claude", "agents", "detritus-coder.md"))
		fmt.Printf("[dry-run] Would write %s\n", filepath.Join(home, ".claude", "agents", "detritus-reviewer.md"))
		setupClaudeTodoGuard(home, binaryPath, hasTodoDoc(docs), true)
		return
	}
	upsertMCP(cfgFile, "mcpServers", binaryPath)
	removeMCPServer(cfgFile, "mcpServers", "candyland")
	fmt.Printf("Claude Code MCP config: %s\n", cfgFile)

	generateClaudeSkills(home, docs)
	generateClaudeCoderAgent(home)
	generateClaudeReviewerAgent(home)

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

// generateClaudeCoderAgent installs the subagent definition that /forge spawns
// per fork-safe task (roles/coder-*). The tech-lead runs at the session effort;
// its coders are the wide fan-out, so they run at low effort to keep the loop
// cheap — set via the `effort` frontmatter key, which Claude Code honors as a
// per-subagent override (low|medium|high|xhigh|max). Verified against the
// subagents frontmatter contract: `effort` is a first-class key, so no Workflow
// model/opts fallback is needed — the definition file carries it directly.
func generateClaudeCoderAgent(home string) {
	agentsDir := filepath.Join(home, ".claude", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "warning: claude agents dir: %v\n", err)
		return
	}
	agentFile := filepath.Join(agentsDir, "detritus-coder.md")
	// No `tools:` key: a restricted list strips the built-in file/shell tools
	// (Read/Edit/Write/Bash/…) and leaves the coder unable to change anything —
	// its first file-read comes back as plain text. Omitting the key inherits
	// the session's full toolset, detritus MCP included.
	content := `---
name: detritus-coder
description: Implementation-loop coder spawned by /forge's tech-lead — takes one fork-safe task, loads its role via kb_get, and drives it to green. Do not invoke directly.
model: inherit
effort: low
---

# Detritus Coder

You are a coder in the ` + "`/forge`" + ` parallel implementation loop, spawned by the tech-lead for a single fork-safe task. You run at **low effort** deliberately — the task is already partitioned and names its defining test; your job is to write that test failing-first and land the smallest delta that turns it green.

1. Load your role doc with ` + "`kb_get`" + ` — ` + "`roles/coder-backend`" + `, ` + "`roles/coder-frontend`" + `, ` + "`roles/coder-fullstack`" + ` per the role in your brief — and follow it. It composes ` + "`core/coder`" + `.
2. Implement only the assigned task inside your worktree; honor the interface contract the task's defining test asserts.
3. Drive the defining test to green and keep the canonical verification passing.
4. If you hit a decision you cannot make within your boundary, emit the fenced ` + "`BLOCKED {json}`" + ` line and stop — the tech-lead decides and re-spawns you.
`
	_ = os.WriteFile(agentFile, []byte(content), 0o644)
	fmt.Printf("Claude Code coder agent: %s\n", agentFile)
}

// generateClaudeReviewerAgent installs the subagent definition the review flows
// spawn (/gh-self-review Phase 3, and /forge delivery through it). Model and
// effort are pinned per ROLE here — never per command: review runs on
// claude-fable-5 at high effort regardless of the session model, the reviewing
// counterpart of the coder's effort:low pin. An unrecognized model value makes
// Claude Code fall back to inherit, so old CLIs degrade to the session model
// rather than erroring.
func generateClaudeReviewerAgent(home string) {
	agentsDir := filepath.Join(home, ".claude", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "warning: claude agents dir: %v\n", err)
		return
	}
	agentFile := filepath.Join(agentsDir, "detritus-reviewer.md")
	// No `tools:` key — same rationale as detritus-coder: a restricted list
	// strips the built-in tools; omitting it inherits the session's full
	// toolset, detritus MCP included (the reviewer needs kb_get + Read/Bash/
	// Grep to verify, though it never edits).
	content := `---
name: detritus-reviewer
description: Delivery-loop reviewer spawned by /gh-self-review and /forge delivery — hard-reviews a diff against the driving intent under the shared review doctrine. Review-only. Do not invoke directly.
model: claude-fable-5
effort: high
---

# Detritus Reviewer

You are the reviewer in a delivery loop, spawned to hard-review a diff before it ships. You run on a pinned model at **high effort** deliberately — independent review is the last gate before a PR.

1. Load your role doc with ` + "`kb_get name=\"roles/reviewer\"`" + ` and follow it. It composes ` + "`core/review-rigor`" + ` and ` + "`flows/principles/truthseeker`" + ` — load those too and apply the rubric end-to-end; never paraphrase it.
2. Your brief carries pointers to the change (repo path, base, head SHA, in-scope files) and the **driving intent** (what the user asked for). Pull the diff live from the repo per the rigor doc — never from a dump or paste. Verify the diff satisfies the intent: a missing, partial, or contradicted intent commitment is a blocker. If no intent was provided, say so in your output and review mechanics only.
3. Never edit files, stage, commit, push, or post. Your output is the verdict/triage the wrapping flow asked for — nothing else.
`
	_ = os.WriteFile(agentFile, []byte(content), 0o644)
	fmt.Printf("Claude Code reviewer agent: %s\n", agentFile)
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
		fmt.Printf("[dry-run] Would remove stale candyland MCP entry from %s\n", configFile)
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

	// Self-heal: drop any stale candyland MCP table. candyland no longer ships a
	// control-mcp subcommand, so a registered entry boots its full app and fails
	// the MCP handshake. Only strip if the file already exists (don't create it).
	if fileExists(file) {
		content = removeTOMLTable(content, "mcp_servers.candyland")
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

// removeTOMLTable strips a full [table] block (its header plus all body lines up
// to the next "[" header or EOF) from content, preserving every other table. It is
// a no-op when the table isn't present.
func removeTOMLTable(content, table string) string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	header := "[" + table + "]"
	lines := strings.Split(normalized, "\n")
	filtered := make([]string, 0, len(lines))
	removed := false

	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == header {
			removed = true
			for i+1 < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i+1]), "[") {
				i++
			}
			continue
		}
		filtered = append(filtered, lines[i])
	}

	if !removed {
		return content
	}
	return strings.TrimRight(strings.Join(filtered, "\n"), "\n") + "\n"
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
		fmt.Printf("[dry-run] Would remove stale candyland MCP entry from %s\n", mcpFile)
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
	removeMCPServer(mcpFile, "mcpServers", "candyland")

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

func printVerification(home, binaryPath string) {
	fmt.Println("\nPost-install verification:")

	// Consult companion binaries (beside detritus)
	if _, ok := toolBinFor(binaryPath, "typst"); ok {
		fmt.Println("  [PASS] Typst binary")
	} else {
		fmt.Println("  [WARN] Typst binary not found")
	}
	if _, ok := toolBinFor(binaryPath, "d2"); ok {
		fmt.Println("  [PASS] D2 binary")
	} else {
		fmt.Println("  [WARN] D2 binary not found")
	}

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
