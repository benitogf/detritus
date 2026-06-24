package code

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeGo(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// linePos returns the index of the line that exactly equals rel (a file header
// in the map), or -1 if absent.
func linePos(out, rel string) int {
	for i, ln := range strings.Split(out, "\n") {
		if ln == rel {
			return i
		}
	}
	return -1
}

func countHeaders(out string, rels ...string) int {
	n := 0
	for _, r := range rels {
		if linePos(out, r) >= 0 {
			n++
		}
	}
	return n
}

// sampleProject writes a 4-file project: core defines Compute/Helper (referenced
// by a and b), lonely is isolated. Returns the project root.
func sampleProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeGo(t, root, "go.mod", "module sample\n\ngo 1.25\n")
	writeGo(t, root, "core.go", "package sample\n\nfunc Compute() int { return 42 }\n\nfunc Helper() int { return 1 }\n")
	writeGo(t, root, "a.go", "package sample\n\nfunc UseA() int { return Compute() + Helper() }\n")
	writeGo(t, root, "b.go", "package sample\n\nfunc UseB() int { return Compute() }\n")
	writeGo(t, root, "lonely.go", "package sample\n\nfunc Lonely() {}\n")
	return root
}

func TestBuildCodeMapRanksReferencedFileHighest(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	root := sampleProject(t)

	out, err := BuildCodeMap(MapOptions{Scope: root, Budget: 100000})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"core.go", "a.go", "b.go", "lonely.go"} {
		if linePos(out, f) < 0 {
			t.Errorf("map missing %q\n---\n%s", f, out)
		}
	}
	// core.go is referenced by a and b → highest PageRank → listed first.
	if pc, pa := linePos(out, "core.go"), linePos(out, "a.go"); pc >= pa {
		t.Errorf("core.go (pos %d) should rank above a.go (pos %d)\n---\n%s", pc, pa, out)
	}
	if pc, pl := linePos(out, "core.go"), linePos(out, "lonely.go"); pc >= pl {
		t.Errorf("core.go (pos %d) should rank above lonely.go (pos %d)", pc, pl)
	}
	// signatures present
	if !strings.Contains(out, "func Compute() int") {
		t.Errorf("expected Compute signature in map\n---\n%s", out)
	}
}

func TestBuildCodeMapRespectsBudget(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	root := sampleProject(t)
	rels := []string{"core.go", "a.go", "b.go", "lonely.go"}

	big, err := BuildCodeMap(MapOptions{Scope: root, Budget: 100000})
	if err != nil {
		t.Fatal(err)
	}
	small, err := BuildCodeMap(MapOptions{Scope: root, Budget: 3})
	if err != nil {
		t.Fatal(err)
	}
	bigN, smallN := countHeaders(big, rels...), countHeaders(small, rels...)
	if bigN != 4 {
		t.Errorf("big budget should list all 4 files, got %d", bigN)
	}
	if smallN >= bigN {
		t.Errorf("small budget (%d files) should list fewer than big (%d)", smallN, bigN)
	}
	if smallN < 1 {
		t.Error("a tiny budget must still emit the top file")
	}
}

func TestBuildCodeMapFocusReorders(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	root := sampleProject(t)

	noFocus, err := BuildCodeMap(MapOptions{Scope: root, Budget: 100000})
	if err != nil {
		t.Fatal(err)
	}
	if linePos(noFocus, "core.go") >= linePos(noFocus, "lonely.go") {
		t.Fatal("baseline: core.go should outrank lonely.go without focus")
	}

	// Focus by path fragment lifts lonely.go to the top.
	pathFocus, err := BuildCodeMap(MapOptions{Scope: root, Budget: 100000, Focus: []string{"lonely.go"}})
	if err != nil {
		t.Fatal(err)
	}
	if linePos(pathFocus, "lonely.go") >= linePos(pathFocus, "core.go") {
		t.Errorf("path focus should lift lonely.go above core.go\n---\n%s", pathFocus)
	}

	// Focus by identifier (Lonely is defined only in lonely.go) does the same.
	identFocus, err := BuildCodeMap(MapOptions{Scope: root, Budget: 100000, Focus: []string{"Lonely"}})
	if err != nil {
		t.Fatal(err)
	}
	if linePos(identFocus, "lonely.go") >= linePos(identFocus, "core.go") {
		t.Errorf("identifier focus should lift lonely.go above core.go\n---\n%s", identFocus)
	}
}
