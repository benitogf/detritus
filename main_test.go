package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func testBinaryPath(t *testing.T) string {
	t.Helper()
	name := "detritus"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(t.TempDir(), name)
}

func TestMCPServer(t *testing.T) {
	// Build the binary
	binPath := testBinaryPath(t)
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Dir = "."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	ctx := t.Context()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1.0.0"}, nil)
	cmd := exec.Command(binPath)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal("connect:", err)
	}
	defer session.Close()

	// Test: list tools
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal("ListTools:", err)
	}
	if len(tools.Tools) != 7 {
		t.Fatalf("expected 7 tools, got %d", len(tools.Tools))
	}
	toolNames := map[string]bool{}
	for _, tool := range tools.Tools {
		toolNames[tool.Name] = true
	}
	for _, name := range []string{
		"kb_list", "kb_get", "kb_search", "kb_sections",
		"code_map", "code_outline", "code_graph",
	} {
		if !toolNames[name] {
			t.Errorf("missing tool: %s", name)
		}
	}

	// Test: kb_list
	listResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "kb_list",
	})
	if err != nil {
		t.Fatal("kb_list:", err)
	}
	if listResult.IsError {
		t.Fatal("kb_list returned error")
	}
	listText := listResult.Content[0].(*mcp.TextContent).Text
	if len(listText) == 0 {
		t.Fatal("kb_list returned empty")
	}
	t.Log("kb_list output:\n" + listText)

	// Verify subdirectory doc appears in kb_list
	if !contains(listText, "ooo/state-patterns") {
		t.Fatal("kb_list missing ooo/state-patterns")
	}

	// Verify deleted docs are gone from kb_list
	for _, deleted := range []string{"ooo-ko", "scaffold-simple-service"} {
		if contains(listText, deleted) {
			t.Fatalf("kb_list still contains deleted doc: %s", deleted)
		}
	}

	// Test: kb_get with valid doc
	getResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "kb_get",
		Arguments: map[string]any{"name": "ooo/package"},
	})
	if err != nil {
		t.Fatal("kb_get:", err)
	}
	if getResult.IsError {
		t.Fatal("kb_get returned error")
	}
	getText := getResult.Content[0].(*mcp.TextContent).Text
	if len(getText) < 100 {
		t.Fatal("kb_get ooo/package content too short")
	}
	t.Log("kb_get ooo/package length:", len(getText))

	// Test: kb_get with subdirectory doc
	stateResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "kb_get",
		Arguments: map[string]any{"name": "ooo/state-patterns"},
	})
	if err != nil {
		t.Fatal("kb_get ooo/state-patterns:", err)
	}
	if stateResult.IsError {
		t.Fatal("kb_get ooo/state-patterns returned error")
	}
	stateText := stateResult.Content[0].(*mcp.TextContent).Text
	if !contains(stateText, "Server-Side State") {
		t.Fatal("ooo/state-patterns missing expected content")
	}
	t.Log("kb_get ooo/state-patterns length:", len(stateText))

	// Test: kb_get with deleted docs returns error
	for _, deleted := range []string{"ooo-ko", "scaffold-simple-service"} {
		delResult, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      "kb_get",
			Arguments: map[string]any{"name": deleted},
		})
		if err != nil {
			t.Fatalf("kb_get %s: %v", deleted, err)
		}
		if !delResult.IsError {
			t.Fatalf("expected error for deleted doc %s", deleted)
		}
	}

	// Test: kb_get with alias resolution (underscore-prefixed old naming convention)
	for _, tc := range []struct {
		input string
		want  string
	}{
		{"_truthseeker", "Foundational"},                 // old _alias -> flows/principles/truthseeker
		{"truthseeker", "Foundational"},                  // bare alias -> flows/principles/truthseeker
		{"/truthseeker", "Foundational"},                 // slash-prefixed -> flows/principles/truthseeker
		{"plan", ""},                                     // alias -> flows/plan/plan (just check no error)
		{"ooo-package", ""},                              // hyphen alias -> ooo/package
		{"flows/principles/truthseeker", "Foundational"}, // canonical name still works
		{"testing-go-backend-mock", ""},                  // leaf alias -> flows/testing/testing-go-backend-mock
	} {
		aliasResult, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      "kb_get",
			Arguments: map[string]any{"name": tc.input},
		})
		if err != nil {
			t.Fatalf("kb_get alias %q: %v", tc.input, err)
		}
		if aliasResult.IsError {
			t.Fatalf("kb_get alias %q returned error", tc.input)
		}
		aliasText := aliasResult.Content[0].(*mcp.TextContent).Text
		if tc.want != "" && !contains(aliasText, tc.want) {
			t.Fatalf("kb_get alias %q: expected content containing %q", tc.input, tc.want)
		}
	}

	// Test: kb_get with invalid doc
	notFoundResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "kb_get",
		Arguments: map[string]any{"name": "nonexistent"},
	})
	if err != nil {
		t.Fatal("kb_get nonexistent:", err)
	}
	if !notFoundResult.IsError {
		t.Fatal("expected error for nonexistent doc")
	}

	// Test: kb_get with section parameter
	sectionResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "kb_get",
		Arguments: map[string]any{"name": "ooo/package", "section": "Server Setup"},
	})
	if err != nil {
		t.Fatal("kb_get section:", err)
	}
	if sectionResult.IsError {
		t.Fatal("kb_get section returned error")
	}
	sectionText := sectionResult.Content[0].(*mcp.TextContent).Text
	if len(sectionText) == 0 {
		t.Fatal("kb_get section returned empty")
	}
	if len(sectionText) >= len(getText) {
		t.Fatalf("section should be shorter than full doc (%d >= %d)", len(sectionText), len(getText))
	}
	t.Log("kb_get section length:", len(sectionText))

	// Test: kb_search
	searchResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "kb_search",
		Arguments: map[string]any{"query": "WaitGroup"},
	})
	if err != nil {
		t.Fatal("kb_search:", err)
	}
	if searchResult.IsError {
		t.Fatal("kb_search returned error")
	}
	searchText := searchResult.Content[0].(*mcp.TextContent).Text
	if len(searchText) == 0 {
		t.Fatal("kb_search returned empty for WaitGroup")
	}
	t.Log("kb_search WaitGroup result length:", len(searchText))

	// Test: kb_search with no results
	noResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "kb_search",
		Arguments: map[string]any{"query": "xyznonexistent123"},
	})
	if err != nil {
		t.Fatal("kb_search no results:", err)
	}
	noText := noResult.Content[0].(*mcp.TextContent).Text
	if noText != "No results found for: xyznonexistent123" {
		t.Fatalf("unexpected no-result text: %s", noText)
	}

	// Test: kb_search finds content in subdirectory docs
	stateSearch, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "kb_search",
		Arguments: map[string]any{"query": "MetricsTick pending reset"},
	})
	if err != nil {
		t.Fatal("kb_search state-patterns:", err)
	}
	stateSearchText := stateSearch.Content[0].(*mcp.TextContent).Text
	if !contains(stateSearchText, "patterns/state-management") && !contains(stateSearchText, "ooo/state-patterns") {
		t.Fatal("kb_search didn't find state-management docs for 'MetricsTick pending reset'")
	}
}

func TestListFlag(t *testing.T) {
	binPath := testBinaryPath(t)
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Dir = "."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	out, err := exec.Command(binPath, "--list").Output()
	if err != nil {
		t.Fatal("--list failed:", err)
	}

	output := string(out)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		t.Fatal("--list returned no lines")
	}

	seen := map[string]string{}
	for _, line := range lines {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			t.Fatalf("bad --list line (expected name<TAB>description): %q", line)
		}
		name, desc := parts[0], parts[1]
		if name == "" || desc == "" {
			t.Fatalf("empty name or description in line: %q", line)
		}
		seen[name] = desc
	}

	for _, required := range []string{"ooo/package", "ooo/filters-internals", "ooo/state-patterns", "flows/maintainer/grow", "flows/principles/truthseeker", "flows/plan/plan", "patterns/async-events", "flows/principles/line-of-sight"} {
		if _, ok := seen[required]; !ok {
			t.Errorf("--list missing required doc: %s", required)
		}
	}

	for _, deleted := range []string{"ooo-ko", "scaffold-simple-service", "ooo-package", "grow", "truthseeker", "testing", "async-events"} {
		if _, ok := seen[deleted]; ok {
			t.Errorf("--list still contains deleted doc: %s", deleted)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && strings.Contains(s, substr)
}

func TestListCommandAliasesIncludesJanitor(t *testing.T) {
	aliases := listCommandAliases()
	if !aliases["janitor"] {
		t.Fatal("expected janitor alias in command shim list")
	}
	if !aliases["truthseeker"] {
		t.Fatal("expected truthseeker alias in command shim list")
	}
}

func TestGenerateInlineCommandInstructionsUsesCommandOnlyMap(t *testing.T) {
	home := t.TempDir()
	docs := listDocEntries()

	generateInlineCommandInstructions(home, docs, false)

	path := filepath.Join(home, ".copilot", "instructions", "detritus.instructions.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated instructions: %v", err)
	}
	text := string(data)

	if !strings.Contains(text, "- /janitor -> flows/build/janitor") {
		t.Fatal("expected /janitor mapping in generated instructions")
	}
	if strings.Contains(text, "- /loop ->") || strings.Contains(text, "core/loop") {
		t.Fatal("unexpected loop-core mapping; non-flows docs should not appear as commands")
	}
	if !strings.Contains(text, "If a slash command token appears but is not in the mapping") {
		t.Fatal("expected unmapped slash fallback rule")
	}
}

// TestReadmeCommandsMatchFlows guards README ≡ docs/flows/: the generated
// command table in README.md must equal what readmeCommandsSection() produces
// from the flows/ tree, so the documented surface can never drift from what
// ships. Regenerate with `detritus --readme`.
func TestReadmeCommandsMatchFlows(t *testing.T) {
	raw, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	text := string(raw)
	si := strings.Index(text, readmeCommandsStart)
	ei := strings.Index(text, readmeCommandsEnd)
	if si < 0 || ei < 0 || ei < si {
		t.Fatalf("README.md missing %s/%s markers", readmeCommandsStart, readmeCommandsEnd)
	}
	got := text[si : ei+len(readmeCommandsEnd)]
	want := readmeCommandsSection()
	if got != want {
		t.Fatalf("README command table is stale — run `detritus --readme`.\n--- README ---\n%s\n--- expected ---\n%s", got, want)
	}
}

// TestPluginCommandsMatchFlows guards the Claude/Codex plugin command shims ≡
// docs/flows/: the committed shims must equal what writePluginCommands would
// generate, so they can never drift to a deleted doc name. Regenerate with
// `detritus --plugin-commands`.
func TestPluginCommandsMatchFlows(t *testing.T) {
	want := pluginCommandFiles()
	entries, err := os.ReadDir(pluginCommandsDir)
	if err != nil {
		t.Fatalf("read %s: %v", pluginCommandsDir, err)
	}
	got := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		got[e.Name()] = true
		data, err := os.ReadFile(filepath.Join(pluginCommandsDir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		w, ok := want[e.Name()]
		if !ok {
			t.Errorf("plugin command %s has no backing flows/ doc — run `detritus --plugin-commands`", e.Name())
			continue
		}
		if string(data) != w {
			t.Errorf("plugin command %s is stale — run `detritus --plugin-commands`", e.Name())
		}
	}
	for fname := range want {
		if !got[fname] {
			t.Errorf("missing plugin command %s for a flows/ doc — run `detritus --plugin-commands`", fname)
		}
	}
}

// TestPluginDocReferencesResolve guards every doc path referenced under
// plugins/ (command shims AND the plugin SKILL.md) against the real docs tree,
// so a doc rename can't leave a plugin file pointing at a deleted name — the
// failure that shipped a broken SKILL.md before it was caught. Catches both
// `kb_get name="<path>"` and backticked `<folder>/<doc>` references.
func TestPluginDocReferencesResolve(t *testing.T) {
	nameRe := regexp.MustCompile(`name="([^"]+)"`)
	pathRe := regexp.MustCompile("`((?:flows|core|roles|ooo|patterns)/[A-Za-z0-9/_-]+)`")
	err := filepath.Walk("plugins", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		refs := map[string]bool{}
		for _, m := range nameRe.FindAllStringSubmatch(text, -1) {
			refs[m[1]] = true
		}
		for _, m := range pathRe.FindAllStringSubmatch(text, -1) {
			refs[m[1]] = true
		}
		for ref := range refs {
			if strings.Contains(ref, "/") { // doc path, not a bare alias
				if _, statErr := os.Stat(filepath.Join("docs", ref+".md")); statErr != nil {
					t.Errorf("%s references non-existent doc %q (run `detritus --plugin-commands`, fix SKILL.md)", path, ref)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk plugins/: %v", err)
	}
}

// TestAliasForDocNoCollisions guards the leaf-based aliasForDoc: since folders
// no longer disambiguate (two docs with the same leaf filename in different
// folders would collide to one alias and silently shadow each other in the
// kb_get alias map), assert every doc's alias is unique across the tree.
func TestAliasForDocNoCollisions(t *testing.T) {
	seen := map[string]string{}
	err := filepath.Walk("docs", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		name := strings.TrimSuffix(strings.TrimPrefix(path, "docs/"), ".md")
		alias := aliasForDoc(name)
		if prev, ok := seen[alias]; ok {
			t.Errorf("alias %q collides: %q and %q", alias, prev, name)
		}
		seen[alias] = name
		return nil
	})
	if err != nil {
		t.Fatalf("walk docs/: %v", err)
	}
}

// TestGeneratedArtifactsAreTracked closes the root cause behind the clean-checkout
// failure: the content drift tests (TestPluginCommandsMatchFlows,
// TestReadmeCommandsMatchFlows) read the working TREE, so a generated file that
// exists on disk but is gitignored passes them yet never reaches the commit.
// Assert the git INDEX actually contains every generated, committed artifact, so
// a future .gitignore slip can't silently ship a broken plugin again.
func TestGeneratedArtifactsAreTracked(t *testing.T) {
	tracked := func(dir string) map[string]bool {
		out, err := exec.Command("git", "ls-files", "-z", dir).Output()
		if err != nil {
			t.Fatalf("git ls-files %s: %v", dir, err)
		}
		set := map[string]bool{}
		for _, p := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
			if p != "" {
				set[p] = true
			}
		}
		return set
	}

	// Every generated plugin command shim must be committed, not just on disk.
	shims := tracked(pluginCommandsDir)
	for fname := range pluginCommandFiles() {
		p := pluginCommandsDir + "/" + fname
		if !shims[p] {
			t.Errorf("generated plugin shim %s is not tracked in git (gitignored or unstaged) — a .gitignore slip would ship a broken plugin", p)
		}
	}
	// The embedded KB blob is the opposite case: it is a deterministic build
	// artifact (regenerated from docs/ by `go generate`, which the release
	// workflow runs before building) and is deliberately NOT committed, so
	// doc-touching branches don't binary-conflict on it. Assert it stays out of
	// the index — a future accidental commit would revive that recurring conflict.
	if tracked("generated/data.gob")["generated/data.gob"] {
		t.Error("generated/data.gob must NOT be tracked in git — it is a gitignored //go:embed artifact built by `go generate`; committing it reintroduces per-branch binary merge conflicts")
	}
}

func cacheBinName() string {
	if runtime.GOOS == "windows" {
		return "detritus.exe"
	}
	return "detritus"
}

func TestCodexCacheBinaryHonorsEnvOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DETRITUS_CACHE_DIR", dir)

	got := codexCacheBinary(t.TempDir())
	want := filepath.Join(dir, cacheBinName())
	if got != want {
		t.Fatalf("codexCacheBinary = %q, want %q", got, want)
	}
}

func TestCodexCacheBinaryDefaultPath(t *testing.T) {
	t.Setenv("DETRITUS_CACHE_DIR", "")

	got := codexCacheBinary(t.TempDir())
	suffix := filepath.Join("detritus-codex", cacheBinName())
	if !strings.HasSuffix(got, suffix) {
		t.Fatalf("codexCacheBinary = %q, want suffix %q", got, suffix)
	}
}

func TestClearCodexCacheRemovesStaleBinary(t *testing.T) {
	// DETRITUS_CACHE_DIR overrides the home-derived path, so the home arg is irrelevant.
	cacheDir := t.TempDir()
	t.Setenv("DETRITUS_CACHE_DIR", cacheDir)
	binPath := filepath.Join(cacheDir, cacheBinName())
	if err := os.WriteFile(binPath, []byte("stale"), 0o755); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	clearCodexCache(homeDir(), false)

	if fileExists(binPath) {
		t.Fatal("expected stale cached MCP binary to be removed so the bootstrap re-fetches")
	}
}

func TestClearCodexCacheDryRunKeepsBinary(t *testing.T) {
	// DETRITUS_CACHE_DIR overrides the home-derived path, so the home arg is irrelevant.
	cacheDir := t.TempDir()
	t.Setenv("DETRITUS_CACHE_DIR", cacheDir)
	binPath := filepath.Join(cacheDir, cacheBinName())
	if err := os.WriteFile(binPath, []byte("stale"), 0o755); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	clearCodexCache(homeDir(), true)

	if !fileExists(binPath) {
		t.Fatal("dry-run must not remove the cached MCP binary")
	}
}

func readUpsertedEntry(t *testing.T, file, parentKey string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	parent, ok := data[parentKey].(map[string]any)
	if !ok {
		t.Fatalf("missing parent key %q in %s", parentKey, file)
	}
	entry, ok := parent["detritus"].(map[string]any)
	if !ok {
		t.Fatalf("missing detritus entry under %q in %s", parentKey, file)
	}
	return entry
}

// VS Code's native MCP host skips any server entry that lacks an explicit
// transport "type", so upsertMCP must stamp "stdio" when writing under the
// "servers" key. Other hosts key under "mcpServers" and infer stdio, so the
// type must NOT be added there.
func TestUpsertMCPStampsStdioForVSCodeServersKey(t *testing.T) {
	file := filepath.Join(t.TempDir(), "mcp.json")
	upsertMCP(file, "servers", "/usr/local/bin/detritus")

	entry := readUpsertedEntry(t, file, "servers")
	if entry["type"] != "stdio" {
		t.Fatalf("expected type=stdio for servers key, got %v", entry["type"])
	}
	if entry["command"] != "/usr/local/bin/detritus" {
		t.Fatalf("expected command to be preserved, got %v", entry["command"])
	}
}

func TestUpsertMCPOmitsTypeForMcpServersKey(t *testing.T) {
	file := filepath.Join(t.TempDir(), "mcp.json")
	upsertMCP(file, "mcpServers", "/usr/local/bin/detritus")

	entry := readUpsertedEntry(t, file, "mcpServers")
	if _, ok := entry["type"]; ok {
		t.Fatalf("expected no type for mcpServers key, got %v", entry["type"])
	}
}

// The in-the-wild repair path: a config written by an older detritus has a
// "servers.detritus" entry with no transport "type". Re-running setup must
// upgrade it in place by adding "type":"stdio" so VS Code stops skipping it.
func TestUpsertMCPUpgradesExistingServersEntryWithoutType(t *testing.T) {
	file := filepath.Join(t.TempDir(), "mcp.json")
	seed := []byte(`{"servers":{"detritus":{"command":"/old/path","args":[]}}}`)
	if err := os.WriteFile(file, seed, 0o644); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	upsertMCP(file, "servers", "/usr/local/bin/detritus")

	entry := readUpsertedEntry(t, file, "servers")
	if entry["type"] != "stdio" {
		t.Fatalf("expected re-run to add type=stdio, got %v", entry["type"])
	}
	if entry["command"] != "/usr/local/bin/detritus" {
		t.Fatalf("expected command to be refreshed, got %v", entry["command"])
	}
}
