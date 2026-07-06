package main

import (
	"context"
	"strings"
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

// connectTestClient wires an in-memory client to a freshly built server and
// returns the connected client session (both sides closed via t.Cleanup).
func connectTestClient(t *testing.T) (context.Context, *mcp.ClientSession) {
	t.Helper()
	server := buildMCPServer(newTestEngine(t))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)

	clientT, serverT := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { cs.Close() })
	return ctx, cs
}

// TestToolAnnotations asserts every read-only tool advertises ReadOnlyHint +
// OpenWorldHint=false. All remaining tools (kb_*/code_*) are read-only.
func TestToolAnnotations(t *testing.T) {
	ctx, cs := connectTestClient(t)
	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	byName := map[string]*mcp.Tool{}
	for _, tl := range tools.Tools {
		byName[tl.Name] = tl
	}

	readOnly := []string{
		"kb_get", "kb_search", "kb_list", "kb_sections",
		"code_graph", "code_outline", "code_map",
	}
	for _, name := range readOnly {
		tl := byName[name]
		if tl == nil {
			t.Errorf("%s: not registered", name)
			continue
		}
		if tl.Annotations == nil {
			t.Errorf("%s: no annotations", name)
			continue
		}
		if !tl.Annotations.ReadOnlyHint {
			t.Errorf("%s: ReadOnlyHint not set", name)
		}
		if tl.Annotations.OpenWorldHint == nil || *tl.Annotations.OpenWorldHint {
			t.Errorf("%s: OpenWorldHint want *false, got %v", name, tl.Annotations.OpenWorldHint)
		}
	}
}

// TestKBSearchStructuredOutput asserts kb_search advertises an inferred output
// schema, returns a populated typed structuredContent {hits,next}, and emits
// resource_link content items for its top hits — alongside the back-compat text.
func TestKBSearchStructuredOutput(t *testing.T) {
	ctx, cs := connectTestClient(t)

	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	for _, tl := range tools.Tools {
		if tl.Name == "kb_search" && tl.OutputSchema == nil {
			t.Fatal("kb_search: OutputSchema not inferred from typed result")
		}
	}

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "kb_search",
		Arguments: map[string]any{"query": "WaitGroup"},
	})
	if err != nil {
		t.Fatalf("kb_search: %v", err)
	}
	if res.IsError {
		t.Fatal("kb_search returned error")
	}
	// Back-compat: first content item is still text.
	if _, ok := res.Content[0].(*mcp.TextContent); !ok {
		t.Fatalf("kb_search: first content not text, got %T", res.Content[0])
	}
	// resource_link items present for top hits.
	var links int
	for _, c := range res.Content {
		if rl, ok := c.(*mcp.ResourceLink); ok {
			links++
			if !strings.HasPrefix(rl.URI, "kb://") {
				t.Errorf("resource_link URI %q missing kb:// scheme", rl.URI)
			}
			if rl.MIMEType != "text/markdown" {
				t.Errorf("resource_link %q MIME = %q, want text/markdown", rl.URI, rl.MIMEType)
			}
		}
	}
	if links == 0 {
		t.Fatal("kb_search emitted no resource_link items")
	}
	// Structured content: typed {hits:[{path,section,score,snippet}], next:[...]}.
	sc, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent not an object: %T", res.StructuredContent)
	}
	hits, ok := sc["hits"].([]any)
	if !ok || len(hits) == 0 {
		t.Fatalf("structuredContent.hits empty/wrong type: %v", sc["hits"])
	}
	h0 := hits[0].(map[string]any)
	for _, k := range []string{"path", "section", "score", "snippet"} {
		if _, present := h0[k]; !present {
			t.Errorf("hit missing field %q", k)
		}
	}
	next, ok := sc["next"].([]any)
	if !ok || len(next) == 0 {
		t.Fatalf("structuredContent.next empty/wrong type: %v", sc["next"])
	}
	n0 := next[0].(map[string]any)
	if n0["tool"] != "kb_sections" {
		t.Errorf("first next hop tool = %v, want kb_sections", n0["tool"])
	}
}

// TestKBDocResourceResolves asserts a KB doc is addressable as kb://<docname>
// and resolvable via resources/read, returning the doc markdown.
func TestKBDocResourceResolves(t *testing.T) {
	ctx, cs := connectTestClient(t)

	list, err := cs.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}
	var found bool
	for _, r := range list.Resources {
		if r.URI == "kb://ooo/package" {
			found = true
		}
	}
	if !found {
		t.Fatal("kb://ooo/package not in resources/list")
	}

	res, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "kb://ooo/package"})
	if err != nil {
		t.Fatalf("read resource: %v", err)
	}
	if len(res.Contents) == 0 || len(res.Contents[0].Text) < 100 {
		t.Fatalf("kb://ooo/package resolved to empty/short content")
	}
	if res.Contents[0].MIMEType != "text/markdown" {
		t.Errorf("resource MIME = %q, want text/markdown", res.Contents[0].MIMEType)
	}
}

// TestNextHopToolsAreRegistered guards against a dead `next` hop: it collects
// every tool name emitted in a `next` hop by kb_search and asserts each is a
// tool the server actually registers, so a future tool rename can't leave a hop
// pointing at a nonexistent tool. It uses an identifier-like kb_search query so
// kb_search emits its code_graph hop too.
func TestNextHopToolsAreRegistered(t *testing.T) {
	ctx, cs := connectTestClient(t)

	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	registered := map[string]bool{}
	for _, tl := range tools.Tools {
		registered[tl.Name] = true
	}

	// nextTools returns the tool names in the `next` hops of a tool call result.
	nextTools := func(name string, args map[string]any) []string {
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if res.IsError {
			t.Fatalf("%s returned error: %v", name, res.Content)
		}
		sc, ok := res.StructuredContent.(map[string]any)
		if !ok {
			t.Fatalf("%s: structuredContent not an object: %T", name, res.StructuredContent)
		}
		next, _ := sc["next"].([]any)
		var out []string
		for _, h := range next {
			if tool, ok := h.(map[string]any)["tool"].(string); ok {
				out = append(out, tool)
			}
		}
		return out
	}

	// "WaitGroup" is identifier-like, so kb_search emits kb_sections + kb_get +
	// code_graph.
	kbHops := nextTools("kb_search", map[string]any{"query": "WaitGroup"})

	if len(kbHops) == 0 {
		t.Fatal("kb_search emitted no next hops to validate")
	}
	// Assert we actually reached the code_graph hop (the identifier-like branch),
	// so that hardcoded name is genuinely exercised.
	if !containsStr(kbHops, "code_graph") {
		t.Errorf("expected kb_search to emit a code_graph hop for an identifier query; got %v", kbHops)
	}

	for _, hop := range kbHops {
		if !registered[hop] {
			t.Errorf("next hop names unregistered tool %q (registered: %v)", hop, toolNames(tools.Tools))
		}
	}
}

func containsStr(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
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
