package chunk

import (
	"testing"
	"testing/fstest"
)

const testDoc = `---
description: Test doc description
category: core
triggers:
  - foo
  - bar
when: When testing
related:
  - other/doc
---

# Title

Some intro text before sections.

## First Section

Content of first section.

More content here.

## Second Section

Content of second section.

### Subsection

Still part of second section.
`

func TestParseDoc(t *testing.T) {
	doc := ParseDoc("test/doc", testDoc)

	if doc.Name != "test/doc" {
		t.Fatalf("expected name test/doc, got %s", doc.Name)
	}
	if doc.Frontmatter.Description != "Test doc description" {
		t.Fatalf("expected description 'Test doc description', got %q", doc.Frontmatter.Description)
	}
	if doc.Frontmatter.Category != "core" {
		t.Fatalf("expected category 'core', got %q", doc.Frontmatter.Category)
	}
	if len(doc.Frontmatter.Triggers) != 2 || doc.Frontmatter.Triggers[0] != "foo" || doc.Frontmatter.Triggers[1] != "bar" {
		t.Fatalf("expected triggers [foo bar], got %v", doc.Frontmatter.Triggers)
	}
	if doc.Frontmatter.When != "When testing" {
		t.Fatalf("expected when 'When testing', got %q", doc.Frontmatter.When)
	}
	if len(doc.Frontmatter.Related) != 1 || doc.Frontmatter.Related[0] != "other/doc" {
		t.Fatalf("expected related [other/doc], got %v", doc.Frontmatter.Related)
	}
	if len(doc.Sections) != 3 {
		t.Fatalf("expected 3 sections, got %d", len(doc.Sections))
	}
	if doc.Sections[0].Heading != "" {
		t.Fatalf("expected empty heading for intro section, got %q", doc.Sections[0].Heading)
	}
	if doc.Sections[1].Heading != "First Section" {
		t.Fatalf("expected 'First Section', got %q", doc.Sections[1].Heading)
	}
	if doc.Sections[2].Heading != "Second Section" {
		t.Fatalf("expected 'Second Section', got %q", doc.Sections[2].Heading)
	}
}

func TestChunkDocs(t *testing.T) {
	doc := ParseDoc("test/doc", testDoc)
	chunks := ChunkDocs([]Doc{doc})
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	for _, c := range chunks {
		if c.DocName != "test/doc" {
			t.Fatalf("expected doc name test/doc, got %s", c.DocName)
		}
		if c.Content == "" {
			t.Fatalf("empty content for chunk section %q", c.Section)
		}
	}
}

func TestParseAll(t *testing.T) {
	testFS := fstest.MapFS{
		"docs/ooo/package.md": &fstest.MapFile{
			Data: []byte(testDoc),
		},
		"docs/meta/grow.md": &fstest.MapFile{
			Data: []byte("---\ndescription: Grow doc\ncategory: meta\ntriggers:\n  - grow\n---\n\n## Only Section\n\nContent.\n"),
		},
	}
	docs, err := ParseAll(testFS, "docs")
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 {
		t.Fatalf("expected 2 docs, got %d", len(docs))
	}
	names := map[string]bool{}
	for _, d := range docs {
		names[d.Name] = true
	}
	if !names["ooo/package"] || !names["meta/grow"] {
		t.Fatalf("unexpected doc names: %v", names)
	}
}
