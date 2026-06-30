package main

import (
	"context"
	"testing"
	"time"

	"github.com/benitogf/detritus/internal/search"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// newTestEngine builds a search engine from the embedded docs/data for tests.
func newTestEngine(t *testing.T) *search.Engine {
	t.Helper()
	engine, err := search.New(dataFS, "generated/data.gob", docsFS, "docs")
	if err != nil {
		t.Fatalf("search engine init: %v", err)
	}
	t.Cleanup(func() { engine.Close() })
	return engine
}

// TestBuildMCPServer asserts buildMCPServer constructs a server exposing the
// expected kb tools — the surface the stdio path serves (and the same one each
// candyland-spawned agent reaches via its own passive detritus stdio child).
func TestBuildMCPServer(t *testing.T) {
	server := buildMCPServer(newTestEngine(t))
	if server == nil {
		t.Fatal("buildMCPServer returned nil")
	}

	// Connect an in-memory client to enumerate the registered tools.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clientT, serverT := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer cs.Close()

	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if !hasTool(tools.Tools, "kb_get") {
		t.Fatalf("expected kb_get tool, got %v", toolNames(tools.Tools))
	}
}

func hasTool(tools []*mcp.Tool, name string) bool {
	for _, tl := range tools {
		if tl.Name == name {
			return true
		}
	}
	return false
}

func toolNames(tools []*mcp.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tl := range tools {
		names = append(names, tl.Name)
	}
	return names
}
