package memory

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/benitogf/detritus/internal/core"
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

// TestRerankByConfirmation checks the P3-2 ranking helper is a true ≤5% tiebreak:
// confirmation nudges the order among comparably-relevant hits but can never flip
// a meaningful relevance gap.
func TestRerankByConfirmation(t *testing.T) {
	// (a) No-swamp: a high-relevance hit with 0 confirmations must stay ahead of a
	// mid-relevance hit maxed out at 10 confirmations (0.5*1.05 = 0.525 < 1.0).
	noSwamp := rerankByConfirmation(
		[]core.Result{{DocName: "high", Score: 1.0}, {DocName: "mid", Score: 0.5}},
		map[string]int{"mid": 10})
	if noSwamp[0].DocName != "high" {
		t.Errorf("confirmation must not overturn a 2× relevance gap, got %q first", noSwamp[0].DocName)
	}

	// (b) Tiebreak fires: on a near-tie, 10 confirmations wins
	// (0.79*1.05 = 0.8295 > 0.80*1.0 = 0.80).
	tie := rerankByConfirmation(
		[]core.Result{{DocName: "fresh", Score: 0.80}, {DocName: "confirmed", Score: 0.79}},
		map[string]int{"confirmed": 10})
	if tie[0].DocName != "confirmed" {
		t.Errorf("among near-equal hits the confirmed one should lead, got %q first", tie[0].DocName)
	}
}

// TestSkillSearchSurfacesConfirmed seeds and re-confirms a lesson, then asserts
// skill_search reports its confirmation count in the structured hit.
func TestSkillSearchSurfacesConfirmed(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	for i := 0; i < 2; i++ { // one create + one confirmation → Confirmed=1
		if _, err := Put("context-deadline-propagation", "procedure",
			[]string{"always propagate the context deadline to downstream calls"},
			greenSource()); err != nil {
			t.Fatalf("seed lesson: %v", err)
		}
	}
	ctx, cs := connectToolsClient(t)
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "skill_search",
		Arguments: map[string]any{"query": "context deadline propagation downstream"},
	})
	if err != nil {
		t.Fatalf("skill_search: %v", err)
	}
	sc := res.StructuredContent.(map[string]any)
	hits := sc["hits"].([]any)
	if len(hits) == 0 {
		t.Fatal("no hits")
	}
	h0 := hits[0].(map[string]any)
	if got := h0["confirmed"]; got != float64(1) {
		t.Errorf("hit confirmed = %v, want 1", got)
	}
}

// TestSkillPutConflictIsError checks the tool surfaces a P3-4 contradiction as
// an isError result naming the conflicting id, and that re-putting with
// supersedes resolves it.
func TestSkillPutConflictIsError(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	ctx, cs := connectToolsClient(t)

	put := func(args map[string]any) *mcp.CallToolResult {
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "skill_put", Arguments: args})
		if err != nil {
			t.Fatalf("skill_put transport error: %v", err)
		}
		return res
	}

	ok := put(map[string]any{
		"id": "lock-ordering-to-avoid-deadlock", "kind": "procedure", "outcome": "green",
		"bullets": []any{"always acquire locks in a global order"},
	})
	if ok.IsError {
		t.Fatalf("first put should succeed: %v", ok.Content[0])
	}

	conflict := put(map[string]any{
		"id": "lock-ordering-to-avoid-deadlock-v2", "kind": "procedure", "outcome": "green",
		"bullets": []any{"never impose a lock order; use a single global lock instead"},
	})
	if !conflict.IsError {
		t.Fatal("expected a contradiction to be reported as isError")
	}
	if txt, _ := conflict.Content[0].(*mcp.TextContent); txt == nil ||
		!strings.Contains(txt.Text, "lock-ordering-to-avoid-deadlock") {
		t.Errorf("conflict message must name the existing id: %+v", conflict.Content[0])
	}

	// Resolve via the explicit supersedes path — the old lesson is staled first,
	// so the write then passes the conflict check.
	resolved := put(map[string]any{
		"id": "lock-ordering-to-avoid-deadlock-v2", "kind": "procedure", "outcome": "green",
		"bullets":    []any{"never impose a lock order; use a single global lock instead"},
		"supersedes": "lock-ordering-to-avoid-deadlock",
	})
	if resolved.IsError {
		t.Fatalf("supersedes should resolve the conflict: %+v", resolved.Content[0])
	}
	if _, err := Get("lock-ordering-to-avoid-deadlock-v2"); err != nil {
		t.Errorf("resolved lesson not written: %v", err)
	}
	if old, _ := Get("lock-ordering-to-avoid-deadlock"); old.Status != "stale" {
		t.Errorf("superseded lesson status = %q, want stale", old.Status)
	}
}
