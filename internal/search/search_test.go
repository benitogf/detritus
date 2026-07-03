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
		ToolDesc: "Get full knowledge document by name.\nCORE: ooo/package (ooo, server setup, filters)\n",
		DocMetadata: []DocEntry{
			{Name: "ooo/package", Meta: DocMeta{
				Description: "ooo package - core real-time state management system",
				Category:    "core",
				Triggers:    []string{"ooo", "server setup", "filters"},
				Sections:    []string{"", "Server Setup", "Filters"},
			}},
			{Name: "patterns/state-management", Meta: DocMeta{
				Description: "State mutation patterns",
				Category:    "patterns",
				Triggers:    []string{"state", "single writer"},
				Sections:    []string{"", "Single Writer"},
			}},
			{Name: "testing/go-backend-async", Meta: DocMeta{
				Description: "Async testing patterns",
				Category:    "testing",
				Triggers:    []string{"WaitGroup", "async"},
				Sections:    []string{"", "WaitGroup Patterns"},
			}},
		},
	}
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

func TestGetSections(t *testing.T) {
	engine := createTestEngine(t)
	defer engine.Close()

	sections, err := engine.GetSections("ooo/package")
	if err != nil {
		t.Fatal(err)
	}
	if len(sections) == 0 {
		t.Fatal("expected at least one section")
	}
	found := false
	for _, s := range sections {
		t.Logf("section: %q", s)
		if s == "Server Setup" || s == "Filters" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected Server Setup or Filters in sections")
	}
}

func TestGetSectionsNotFound(t *testing.T) {
	engine := createTestEngine(t)
	defer engine.Close()

	_, err := engine.GetSections("nonexistent/doc")
	if err == nil {
		t.Fatal("expected error for nonexistent doc")
	}
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

// engineFromData builds an Engine from an explicit dataset + docs FS, mirroring
// createTestEngine's gob round-trip but letting a test supply its own corpus.
func engineFromData(t *testing.T, data GeneratedData, docsFS fstest.MapFS) *Engine {
	t.Helper()
	tmpFile := t.TempDir() + "/data.gob"
	f, err := os.Create(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := gob.NewEncoder(f).Encode(data); err != nil {
		t.Fatal(err)
	}
	f.Close()

	dataFS := fstest.MapFS{"data.gob": &fstest.MapFile{Data: mustReadFile(t, tmpFile)}}
	engine, err := New(dataFS, "data.gob", docsFS, "docs")
	if err != nil {
		t.Fatal("engine init:", err)
	}
	return engine
}

// TestFieldBoostTitleOutranksBody proves the P5-12 field boost: a query term that
// matches a doc's name/section (its "title") outranks a doc that only mentions the
// term repeatedly in body content. Without per-field boosting the body-only doc
// wins on raw term frequency; the boost flips it.
func TestFieldBoostTitleOutranksBody(t *testing.T) {
	// titleDoc matches "sprocket" only in its doc_name; its body never says it.
	// bodyDoc never has "sprocket" in name/section but repeats it in body.
	data := GeneratedData{
		Chunks: []ChunkMeta{
			{DocName: "reference/sprocket", Section: "Overview", Position: 0},
			{DocName: "guides/hardware", Section: "Parts", Position: 0},
		},
		DocMetadata: []DocEntry{
			{Name: "guides/hardware", Meta: DocMeta{Sections: []string{"Parts"}}},
			{Name: "reference/sprocket", Meta: DocMeta{Sections: []string{"Overview"}}},
		},
	}
	docsFS := fstest.MapFS{
		"docs/reference/sprocket.md": &fstest.MapFile{
			Data: []byte("# Reference\n\n## Overview\n\nGeneral information about the component and its settings.\n"),
		},
		"docs/guides/hardware.md": &fstest.MapFile{
			Data: []byte("# Hardware\n\n## Parts\n\nThe sprocket connects here. Install the sprocket. Tighten the sprocket. Replace the sprocket when it is worn.\n"),
		},
	}

	engine := engineFromData(t, data, docsFS)
	defer engine.Close()

	results, err := engine.Search("sprocket", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected results for sprocket query")
	}
	for i, r := range results {
		t.Logf("rank %d: %s/%s score=%.3f", i, r.DocName, r.Section, r.Score)
	}
	if results[0].DocName != "reference/sprocket" {
		t.Fatalf("field boost failed: expected name-match reference/sprocket to rank first, got %s", results[0].DocName)
	}
}
