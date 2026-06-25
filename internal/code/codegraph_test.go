package code

import (
	"strings"
	"testing"
)

func TestCodeGraphWhoCallsAndReachable(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	root := sampleProject(t) // core.Compute is called by UseA and UseB

	out, err := BuildCodeGraph(GraphQuery{Symbol: "Compute", Scope: root})
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
	out2, err := BuildCodeGraph(GraphQuery{Symbol: "UseA", Scope: root})
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

	out, err := BuildCodeGraph(GraphQuery{Symbol: "Greeter", Scope: root})
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

	out, err := BuildCodeGraph(GraphQuery{Symbol: "Run", Scope: root})
	if err != nil {
		t.Fatalf("fallback should not error: %v", err)
	}
	if !strings.Contains(out, "falling back to the structural map") {
		t.Errorf("expected graceful fallback note\n---\n%s", out)
	}
}

func TestCodeGraphRequiresSymbol(t *testing.T) {
	if _, err := BuildCodeGraph(GraphQuery{Scope: t.TempDir()}); err == nil {
		t.Error("expected error when symbol is empty")
	}
}
