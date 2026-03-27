package search

import (
	"encoding/gob"
	"os"
	"testing"
	"testing/fstest"
)

func buildTestData() GeneratedData {
	return GeneratedData{
		Chunks: []ChunkMeta{
			{DocName: "ooo/package", Section: "", Position: 0},
			{DocName: "ooo/package", Section: "Server Setup", Position: 1},
			{DocName: "ooo/package", Section: "Filters", Position: 2},
			{DocName: "patterns/state-management", Section: "", Position: 0},
			{DocName: "patterns/state-management", Section: "Single Writer", Position: 1},
			{DocName: "testing/go-backend-async", Section: "", Position: 0},
			{DocName: "testing/go-backend-async", Section: "WaitGroup Patterns", Position: 1},
		},
		Vectors: [][]float32{
			makeVec(0.1, 0.2, 0.3),
			makeVec(0.4, 0.5, 0.6),
			makeVec(0.7, 0.8, 0.9),
			makeVec(0.2, 0.3, 0.1),
			makeVec(0.5, 0.6, 0.4),
			makeVec(0.3, 0.1, 0.2),
			makeVec(0.6, 0.4, 0.5),
		},
		ToolDesc: "Get full knowledge document by name.\nCORE: ooo/package (ooo, server setup, filters)\n",
		DocMetadata: map[string]DocMeta{
			"ooo/package": {
				Description: "ooo package - core real-time state management system",
				Category:    "core",
				Triggers:    []string{"ooo", "server setup", "filters"},
				Sections:    []string{"", "Server Setup", "Filters"},
			},
			"patterns/state-management": {
				Description: "State mutation patterns",
				Category:    "patterns",
				Triggers:    []string{"state", "single writer"},
				Sections:    []string{"", "Single Writer"},
			},
			"testing/go-backend-async": {
				Description: "Async testing patterns",
				Category:    "testing",
				Triggers:    []string{"WaitGroup", "async"},
				Sections:    []string{"", "WaitGroup Patterns"},
			},
		},
	}
}

func makeVec(vals ...float32) []float32 {
	v := make([]float32, 384)
	for i, val := range vals {
		if i < len(v) {
			v[i] = val
		}
	}
	return v
}

func buildTestFS() fstest.MapFS {
	return fstest.MapFS{
		"docs/ooo/package.md": &fstest.MapFile{
			Data: []byte("---\ndescription: ooo package\ncategory: core\ntriggers:\n  - ooo\n  - server setup\n  - filters\n---\n\n# ooo Package\n\nIntro content about ooo.\n\n## Server Setup\n\nHow to set up an ooo server.\n\n## Filters\n\nReadObjectFilter, WriteFilter, AfterWriteFilter.\n"),
		},
		"docs/patterns/state-management.md": &fstest.MapFile{
			Data: []byte("---\ndescription: State mutation patterns\ncategory: patterns\ntriggers:\n  - state\n  - single writer\n---\n\n# State Management\n\nIntro.\n\n## Single Writer\n\nNever have two writers.\n"),
		},
		"docs/testing/go-backend-async.md": &fstest.MapFile{
			Data: []byte("---\ndescription: Async testing patterns\ncategory: testing\ntriggers:\n  - WaitGroup\n  - async\n---\n\n# Async Testing\n\nIntro.\n\n## WaitGroup Patterns\n\nUse sync.WaitGroup for deterministic tests.\n"),
		},
	}
}

func createTestEngine(t *testing.T) *Engine {
	t.Helper()

	data := buildTestData()
	docsFS := buildTestFS()

	tmpFile := t.TempDir() + "/data.gob"
	f, err := os.Create(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := gob.NewEncoder(f).Encode(data); err != nil {
		t.Fatal(err)
	}
	f.Close()

	dataFS := fstest.MapFS{
		"data.gob": &fstest.MapFile{Data: mustReadFile(t, tmpFile)},
	}

	engine, err := New(dataFS, "data.gob", docsFS, "docs")
	if err != nil {
		t.Fatal("engine init:", err)
	}
	return engine
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestSearchBM25(t *testing.T) {
	engine := createTestEngine(t)
	defer engine.Close()

	results, err := engine.Search("WaitGroup", 5)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) == 0 {
		t.Fatal("expected results for WaitGroup query")
	}

	foundAsync := false
	for _, r := range results {
		t.Logf("result: %s/%s score=%.3f", r.DocName, r.Section, r.Score)
		if r.DocName == "testing/go-backend-async" {
			foundAsync = true
		}
	}
	if !foundAsync {
		t.Fatal("expected testing/go-backend-async in WaitGroup results")
	}
}

func TestSearchNoResults(t *testing.T) {
	engine := createTestEngine(t)
	defer engine.Close()

	results, err := engine.Search("xyznonexistent123", 5)
	if err != nil {
		t.Fatal(err)
	}

	for _, r := range results {
		t.Logf("gibberish result: %s/%s score=%.3f", r.DocName, r.Section, r.Score)
		if r.Score > 0.6 {
			t.Fatalf("gibberish query should not produce high-confidence results, got score %.3f for %s", r.Score, r.DocName)
		}
	}
}

func TestGetDoc(t *testing.T) {
	engine := createTestEngine(t)
	defer engine.Close()

	content, err := engine.GetDoc("ooo/package")
	if err != nil {
		t.Fatal(err)
	}
	if len(content) < 50 {
		t.Fatal("content too short")
	}
}

func TestGetDocNotFound(t *testing.T) {
	engine := createTestEngine(t)
	defer engine.Close()

	_, err := engine.GetDoc("nonexistent/doc")
	if err == nil {
		t.Fatal("expected error for nonexistent doc")
	}
}

func TestGetSection(t *testing.T) {
	engine := createTestEngine(t)
	defer engine.Close()

	content, err := engine.GetSection("ooo/package", "Filters")
	if err != nil {
		t.Fatal(err)
	}
	if content == "" {
		t.Fatal("empty section content")
	}
	if len(content) > 500 {
		t.Fatalf("section too long (%d), probably returned full doc", len(content))
	}
	t.Log("section content:", content)
}

func TestToolDescription(t *testing.T) {
	engine := createTestEngine(t)
	defer engine.Close()

	desc := engine.ToolDescription()
	if desc == "" {
		t.Fatal("empty tool description")
	}
	if len(desc) < 20 {
		t.Fatal("tool description too short")
	}
	t.Log("tool description:", desc)
}

func TestMMRDiversity(t *testing.T) {
	engine := createTestEngine(t)
	defer engine.Close()

	results, err := engine.Search("ooo server filters", 5)
	if err != nil {
		t.Fatal(err)
	}

	docCounts := map[string]int{}
	for _, r := range results {
		docCounts[r.DocName]++
		t.Logf("result: %s/%s score=%.3f", r.DocName, r.Section, r.Score)
	}

	if len(results) > 3 {
		uniqueDocs := len(docCounts)
		if uniqueDocs < 2 {
			t.Fatalf("MMR should diversify results, but all %d results from same doc", len(results))
		}
	}
}

func TestCosineSimilarity(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{1, 0, 0}
	sim := cosineSimilarity(a, b)
	if sim < 0.99 {
		t.Fatalf("identical vectors should have similarity ~1.0, got %f", sim)
	}

	c := []float32{0, 1, 0}
	sim = cosineSimilarity(a, c)
	if sim > 0.01 {
		t.Fatalf("orthogonal vectors should have similarity ~0.0, got %f", sim)
	}
}
