package code

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// chainProject writes a linear call chain Compute <- UseA <- UseB <- UseC <- UseD
// plus an in-package test that calls Compute, so the reverse-transitive
// impacted-by walk and affected-tests detection have a known shape. Returns root.
func chainProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeGo(t, root, "go.mod", "module chain\n\ngo 1.25\n")
	writeGo(t, root, "core.go", "package chain\n\nfunc Compute() int { return 42 }\n")
	writeGo(t, root, "a.go", "package chain\n\nfunc UseA() int { return Compute() }\n")
	writeGo(t, root, "b.go", "package chain\n\nfunc UseB() int { return UseA() }\n")
	writeGo(t, root, "c.go", "package chain\n\nfunc UseC() int { return UseB() }\n")
	writeGo(t, root, "d.go", "package chain\n\nfunc UseD() int { return UseC() }\n")
	writeGo(t, root, "core_test.go", "package chain\n\nimport \"testing\"\n\nfunc TestCompute(t *testing.T) { _ = Compute() }\n")
	return root
}

func TestCodeGraphWhoCallsAndReachable(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	root := sampleProject(t) // core.Compute is called by UseA and UseB

	out, _, err := BuildCodeGraph(GraphQuery{Symbol: "Compute", Scope: root})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "who-calls Compute") {
		t.Fatalf("missing who-calls header\n---\n%s", out)
	}
	for _, caller := range []string{"UseA", "UseB"} {
		if !strings.Contains(out, caller) {
			t.Errorf("who-calls Compute should list %s\n---\n%s", caller, out)
		}
	}

	// reachable-from a caller includes the function it calls.
	out2, _, err := BuildCodeGraph(GraphQuery{Symbol: "UseA", Scope: root})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out2, "reachable from UseA") || !strings.Contains(out2, "Compute") {
		t.Errorf("reachable from UseA should include Compute\n---\n%s", out2)
	}
}

func TestCodeGraphImplementers(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	root := t.TempDir()
	writeGo(t, root, "go.mod", "module impl\n\ngo 1.25\n")
	writeGo(t, root, "greet.go", `package impl

type Greeter interface{ Greet() string }

type Robot struct{}

func (r Robot) Greet() string { return "beep" }

type Mute struct{}
`)

	out, _, err := BuildCodeGraph(GraphQuery{Symbol: "Greeter", Scope: root})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "implementers of") {
		t.Fatalf("missing implementers header\n---\n%s", out)
	}
	if !strings.Contains(out, "Robot") {
		t.Errorf("Robot implements Greeter but is missing\n---\n%s", out)
	}
	if strings.Contains(out, "Mute") {
		t.Errorf("Mute does not implement Greeter but was listed\n---\n%s", out)
	}
}

func TestCodeGraphFallbackOnNonCompiling(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	root := t.TempDir()
	writeGo(t, root, "go.mod", "module broken\n\ngo 1.25\n")
	// Type error: calls an undefined function — package does not compile.
	writeGo(t, root, "broken.go", "package broken\n\nfunc Run() { doesNotExist() }\n")

	out, _, err := BuildCodeGraph(GraphQuery{Symbol: "Run", Scope: root})
	if err != nil {
		t.Fatalf("fallback should not error: %v", err)
	}
	if !strings.Contains(out, "falling back to the structural map") {
		t.Errorf("expected graceful fallback note\n---\n%s", out)
	}
}

func TestCodeGraphRequiresSymbol(t *testing.T) {
	if _, _, err := BuildCodeGraph(GraphQuery{Scope: t.TempDir()}); err == nil {
		t.Error("expected error when symbol is empty")
	}
}

// TestCodeGraphImpactedBy proves the reverse-transitive walk reports transitive
// dependents up to the depth bound (and only those), that overly-deep dependents
// are reported as truncated rather than silently dropped, and that the structured
// result mirrors the text.
func TestCodeGraphImpactedBy(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	root := chainProject(t)

	out, res, err := BuildCodeGraph(GraphQuery{Symbol: "Compute", Scope: root})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "impacted-by Compute") {
		t.Fatalf("missing impacted-by header\n---\n%s", out)
	}
	// UseA (depth 1), UseB (depth 2), UseC (depth 3) transitively reach Compute.
	for _, dep := range []string{"UseA", "UseB", "UseC"} {
		if !strings.Contains(out, dep) {
			t.Errorf("impacted-by Compute should list %s\n---\n%s", dep, out)
		}
	}
	// UseD is at depth 4 — beyond the impactDepth bound — so it must NOT appear,
	// and the walk must be flagged truncated (never silently dropped).
	if strings.Contains(out, "UseD") {
		t.Errorf("UseD is beyond depth %d and must not be listed\n---\n%s", impactDepth, out)
	}
	if !strings.Contains(out, "truncated at depth") {
		t.Errorf("expected an explicit depth-truncation note\n---\n%s", out)
	}
	if !res.Truncated {
		t.Error("structured result should report Truncated=true")
	}

	// TestCompute (defined in core_test.go) calls Compute, but a test func is NOT
	// a production caller/dependent — it must be filtered out of who-calls and
	// impacted-by (text + structured), and instead surface as an affected test.
	if strings.Contains(out, "TestCompute") {
		t.Errorf("TestCompute is a _test.go func and must not appear in who-calls/impacted-by\n---\n%s", out)
	}
	names := refNames(res.ImpactedBy)
	for _, dep := range []string{"chain.UseA", "chain.UseB", "chain.UseC"} {
		if !names[dep] {
			t.Errorf("structured impacted_by missing %s; got %v", dep, names)
		}
	}
	if names["chain.TestCompute"] {
		t.Errorf("structured impacted_by must not include the test func chain.TestCompute; got %v", names)
	}
	if names["chain.UseD"] {
		t.Errorf("structured impacted_by should not include the out-of-depth chain.UseD; got %v", names)
	}

	// The test that exercises Compute is instead reported as an affected test.
	if len(res.AffectedTests) != 1 || !strings.HasSuffix(res.AffectedTests[0], "core_test.go") {
		t.Errorf("affected_tests should be [.../core_test.go]; got %v", res.AffectedTests)
	}
}

// TestCodeGraphAffectedTests proves _test.go files that transitively exercise the
// symbol are reported, in both the text and the structured result.
func TestCodeGraphAffectedTests(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	root := chainProject(t)

	out, res, err := BuildCodeGraph(GraphQuery{Symbol: "Compute", Scope: root})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "affected tests:") {
		t.Fatalf("missing affected tests header\n---\n%s", out)
	}
	if !strings.Contains(out, "core_test.go") {
		t.Errorf("core_test.go exercises Compute and should be an affected test\n---\n%s", out)
	}
	if len(res.AffectedTests) != 1 || !strings.HasSuffix(res.AffectedTests[0], "core_test.go") {
		t.Errorf("structured affected_tests = %v, want exactly [.../core_test.go]", res.AffectedTests)
	}

	// A leaf symbol with no test in its dependent set reports none.
	_, res2, err := BuildCodeGraph(GraphQuery{Symbol: "UseD", Scope: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.AffectedTests) != 0 {
		t.Errorf("UseD has no dependent tests, got %v", res2.AffectedTests)
	}
}

// TestCodeGraphStructuredOutput exercises code_graph exactly as an MCP consumer
// and asserts the SDK inferred an OutputSchema and populated structuredContent
// with the typed shape, alongside the back-compat text block.
func TestCodeGraphStructuredOutput(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	root := chainProject(t)

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	RegisterSeamlessTools(server)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	for _, tl := range tools.Tools {
		if tl.Name == "code_graph" && tl.OutputSchema == nil {
			t.Fatal("code_graph: OutputSchema not inferred from typed result")
		}
	}

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "code_graph",
		Arguments: map[string]any{"symbol": "Compute", "scope": root},
	})
	if err != nil {
		t.Fatalf("code_graph: %v", err)
	}
	if res.IsError {
		t.Fatalf("code_graph returned error: %v", res.Content)
	}
	if _, ok := res.Content[0].(*mcp.TextContent); !ok {
		t.Fatalf("code_graph: first content not text, got %T", res.Content[0])
	}

	sc, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent not an object: %T", res.StructuredContent)
	}
	if sc["symbol"] != "Compute" {
		t.Errorf("structuredContent.symbol = %v, want Compute", sc["symbol"])
	}
	impacted, ok := sc["impacted_by"].([]any)
	if !ok || len(impacted) == 0 {
		t.Fatalf("structuredContent.impacted_by empty/wrong type: %v", sc["impacted_by"])
	}
	if _, ok := impacted[0].(map[string]any)["name"]; !ok {
		t.Errorf("impacted_by entry missing name field: %v", impacted[0])
	}
	tests, ok := sc["affected_tests"].([]any)
	if !ok || len(tests) == 0 {
		t.Fatalf("structuredContent.affected_tests empty/wrong type: %v", sc["affected_tests"])
	}
}

func refNames(refs []GraphRef) map[string]bool {
	m := map[string]bool{}
	for _, r := range refs {
		m[r.Name] = true
	}
	return m
}
