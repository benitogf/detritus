package main

import (
	"os/exec"
	"path/filepath"
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
	t.Log("kb_list result length:", len(listText))

	// Test: kb_get with valid doc
	getResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "kb_get",
		Arguments: map[string]any{"name": "ooo-package"},
	})
	if err != nil {
		t.Fatal("kb_get:", err)
	}
	if getResult.IsError {
		t.Fatal("kb_get returned error")
	}
	getText := getResult.Content[0].(*mcp.TextContent).Text
	if len(getText) < 100 {
		t.Fatal("kb_get ooo-package content too short")
	}
	t.Log("kb_get ooo-package length:", len(getText))

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
}
