package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPServer(t *testing.T) {
	// Build the binary
	binPath := filepath.Join(t.TempDir(), "ooo-kb")
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
	if len(tools.Tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(tools.Tools))
	}
	toolNames := map[string]bool{}
	for _, tool := range tools.Tools {
		toolNames[tool.Name] = true
	}
	for _, name := range []string{"kb_list", "kb_get", "kb_search"} {
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
	if !contains(listText, "scaffold/create") {
		t.Fatal("kb_list missing scaffold/create")
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
	scaffoldResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "kb_get",
		Arguments: map[string]any{"name": "scaffold/create"},
	})
	if err != nil {
		t.Fatal("kb_get scaffold/create:", err)
	}
	if scaffoldResult.IsError {
		t.Fatal("kb_get scaffold/create returned error")
	}
	scaffoldText := scaffoldResult.Content[0].(*mcp.TextContent).Text
	if !contains(scaffoldText, "Create a New Project") {
		t.Fatal("scaffold/create missing expected content")
	}
	t.Log("kb_get scaffold/create length:", len(scaffoldText))

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
	scaffoldSearch, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "kb_search",
		Arguments: map[string]any{"query": "INTERACTIVE WORKFLOW"},
	})
	if err != nil {
		t.Fatal("kb_search scaffold:", err)
	}
	scaffoldSearchText := scaffoldSearch.Content[0].(*mcp.TextContent).Text
	if !contains(scaffoldSearchText, "scaffold/create") {
		t.Fatal("kb_search didn't find scaffold/create for 'INTERACTIVE WORKFLOW'")
	}
}

func TestListFlag(t *testing.T) {
	binPath := filepath.Join(t.TempDir(), "ooo-kb")
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

	for _, required := range []string{"ooo/package", "ooo/filters-internals", "scaffold/create", "meta/grow", "meta/truthseeker", "meta/research-first", "plan/analyze", "plan/diagrams", "plan/export", "patterns/async-events", "patterns/line-of-sight"} {
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
