package memory

import (
	"context"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// connectToolsClient registers the memory tools on a server and connects an
// in-memory client, so a test can exercise them exactly as a real MCP consumer.
func connectToolsClient(t *testing.T) (context.Context, *mcp.ClientSession) {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	RegisterTools(server)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)

	clientT, serverT := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { ss.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { cs.Close() })
	return ctx, cs
}

// TestSkillToolAnnotations asserts the memory tools advertise the right hints:
// skill_search/skill_get are read-only + closed-world, skill_put is a write with
// an idempotent hint.
func TestSkillToolAnnotations(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	ctx, cs := connectToolsClient(t)
	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	byName := map[string]*mcp.Tool{}
	for _, tl := range tools.Tools {
		byName[tl.Name] = tl
	}

	for _, name := range []string{"skill_search", "skill_get"} {
		tl := byName[name]
		if tl == nil || tl.Annotations == nil {
			t.Fatalf("%s: missing tool/annotations", name)
		}
		if !tl.Annotations.ReadOnlyHint {
			t.Errorf("%s: ReadOnlyHint not set", name)
		}
		if tl.Annotations.OpenWorldHint == nil || *tl.Annotations.OpenWorldHint {
			t.Errorf("%s: OpenWorldHint want *false", name)
		}
	}
	put := byName["skill_put"]
	if put == nil || put.Annotations == nil {
		t.Fatal("skill_put: missing tool/annotations")
	}
	if put.Annotations.ReadOnlyHint {
		t.Error("skill_put: ReadOnlyHint should be false")
	}
	if !put.Annotations.IdempotentHint {
		t.Error("skill_put: IdempotentHint should be true")
	}
}

// TestSkillSearchStructuredOutput seeds a verified lesson, then asserts
// skill_search returns typed structuredContent {hits,next} plus an inferred
// output schema and the back-compat text block.
func TestSkillSearchStructuredOutput(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	if _, err := Put("goroutine-leak-waitgroup", "procedure",
		[]string{"always Wait on the WaitGroup before returning to avoid a goroutine leak"},
		greenSource()); err != nil {
		t.Fatalf("seed lesson: %v", err)
	}

	ctx, cs := connectToolsClient(t)

	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	for _, tl := range tools.Tools {
		if tl.Name == "skill_search" && tl.OutputSchema == nil {
			t.Fatal("skill_search: OutputSchema not inferred from typed result")
		}
	}

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "skill_search",
		Arguments: map[string]any{"query": "goroutine leak WaitGroup"},
	})
	if err != nil {
		t.Fatalf("skill_search: %v", err)
	}
	if res.IsError {
		t.Fatal("skill_search returned error")
	}
	if _, ok := res.Content[0].(*mcp.TextContent); !ok {
		t.Fatalf("skill_search: first content not text, got %T", res.Content[0])
	}

	sc, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent not an object: %T", res.StructuredContent)
	}
	hits, ok := sc["hits"].([]any)
	if !ok || len(hits) == 0 {
		t.Fatalf("structuredContent.hits empty/wrong type: %v", sc["hits"])
	}
	h0 := hits[0].(map[string]any)
	if h0["path"] != "goroutine-leak-waitgroup" {
		t.Errorf("hit path = %v, want the seeded lesson id", h0["path"])
	}
	next, ok := sc["next"].([]any)
	if !ok || len(next) == 0 {
		t.Fatalf("structuredContent.next empty/wrong type: %v", sc["next"])
	}
	if next[0].(map[string]any)["tool"] != "skill_get" {
		t.Errorf("next hop tool = %v, want skill_get", next[0].(map[string]any)["tool"])
	}
}
