package code

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestOutlinePathFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "x.go")
	writeGo(t, dir, "x.go", "package p\n\nimport \"fmt\"\n\ntype T struct{ a int }\n\nfunc F(n int) string { return fmt.Sprint(n) }\n")

	out, err := OutlinePath(f)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"package p", "type T struct", "func F(n int) string"} {
		if !strings.Contains(out, want) {
			t.Errorf("outline missing %q\n---\n%s", want, out)
		}
	}
}

func TestOutlinePathDirectory(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", "package p\n\nfunc Aaa() {}\n")
	writeGo(t, dir, "b.go", "package p\n\nfunc Bbb() {}\n")

	out, err := OutlinePath(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"=== a.go ===", "func Aaa()", "=== b.go ===", "func Bbb()"} {
		if !strings.Contains(out, want) {
			t.Errorf("directory outline missing %q\n---\n%s", want, out)
		}
	}
}

func TestOutlinePathMissing(t *testing.T) {
	if _, err := OutlinePath(filepath.Join(t.TempDir(), "nope.go")); err == nil {
		t.Error("expected error for missing path")
	}
}

// TestToolRegistrationNoCollision guards that the legacy code_* tools and the
// seamless tools register on one server without a duplicate-name panic — the
// pack-based code_outline was replaced, not duplicated.
func TestToolRegistrationNoCollision(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("tool registration panicked (likely a duplicate tool name): %v", r)
		}
	}()
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	reg := NewRegistry()
	defer reg.Close()
	RegisterTools(server, reg, "test")
	RegisterSeamlessTools(server)
}

func TestBuildCodeMapDefaultsToCwdProject(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	root := sampleProject(t)

	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.Chdir(filepath.Join(root)); err != nil {
		t.Fatal(err)
	}
	// Empty scope → resolve the current project (the temp root has go.mod).
	out, err := BuildCodeMap(MapOptions{Budget: 100000})
	if err != nil {
		t.Fatal(err)
	}
	if linePos(out, "core.go") < 0 {
		t.Errorf("default-scope map should include core.go\n---\n%s", out)
	}
}
