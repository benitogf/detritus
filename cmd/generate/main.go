package main

import (
	"encoding/gob"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/benitogf/detritus/internal/chunk"
	"github.com/blevesearch/bleve/v2"
)

type ChunkMeta struct {
	DocName  string
	Section  string
	Position int
}

type GeneratedData struct {
	Chunks      []ChunkMeta
	BlevePath   string
	ToolDesc    string
	DocMetadata map[string]DocMeta
}

type DocMeta struct {
	Description string
	Category    string
	Triggers    []string
	When        string
	Related     []string
	Sections    []string
}

func main() {
	docsDir := "docs"
	outputDir := "generated"

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		log.Fatalf("mkdir output: %v", err)
	}

	log.Println("parsing docs...")
	docs, err := chunk.ParseAll(os.DirFS("."), docsDir)
	if err != nil {
		log.Fatalf("parse docs: %v", err)
	}
	log.Printf("parsed %d docs", len(docs))

	log.Println("enriching triggers with auto-extracted keywords...")
	enrichTriggers(docs)

	chunks := chunk.ChunkDocs(docs)
	log.Printf("chunked into %d sections", len(chunks))

	log.Println("building bleve index...")
	blevePath := filepath.Join(outputDir, "search.bleve")
	os.RemoveAll(blevePath)
	index, err := buildBleveIndex(blevePath, docs, chunks)
	if err != nil {
		log.Fatalf("bleve index: %v", err)
	}
	index.Close()
	log.Printf("bleve index built at %s", blevePath)

	docMetadata := buildDocMetadata(docs)
	toolDesc := buildToolDescription(docs)

	metas := make([]ChunkMeta, len(chunks))
	for i, c := range chunks {
		metas[i] = ChunkMeta{
			DocName:  c.DocName,
			Section:  c.Section,
			Position: c.Position,
		}
	}

	data := GeneratedData{
		Chunks:      metas,
		BlevePath:   blevePath,
		ToolDesc:    toolDesc,
		DocMetadata: docMetadata,
	}

	dataPath := filepath.Join(outputDir, "data.gob")
	f, err := os.Create(dataPath)
	if err != nil {
		log.Fatalf("create data file: %v", err)
	}
	defer f.Close()
	if err := gob.NewEncoder(f).Encode(data); err != nil {
		log.Fatalf("encode data: %v", err)
	}
	log.Printf("wrote %s", dataPath)

	log.Println("done")
}

func buildBleveIndex(path string, docs []chunk.Doc, chunks []chunk.Chunk) (bleve.Index, error) {
	mapping := bleve.NewIndexMapping()

	docMapping := bleve.NewDocumentMapping()
	docMapping.AddFieldMappingsAt("doc_name", bleve.NewTextFieldMapping())
	docMapping.AddFieldMappingsAt("section", bleve.NewTextFieldMapping())
	docMapping.AddFieldMappingsAt("content", bleve.NewTextFieldMapping())
	docMapping.AddFieldMappingsAt("triggers", bleve.NewTextFieldMapping())
	docMapping.AddFieldMappingsAt("category", bleve.NewTextFieldMapping())
	mapping.AddDocumentMapping("chunk", docMapping)
	mapping.DefaultMapping = docMapping

	index, err := bleve.New(path, mapping)
	if err != nil {
		return nil, fmt.Errorf("bleve new: %w", err)
	}

	triggerMap := map[string]string{}
	for _, doc := range docs {
		triggerMap[doc.Name] = strings.Join(doc.Frontmatter.Triggers, " ")
	}

	batch := index.NewBatch()
	for i, c := range chunks {
		id := fmt.Sprintf("%s#%d", c.DocName, c.Position)
		doc := map[string]string{
			"doc_name": c.DocName,
			"section":  c.Section,
			"content":  c.Content,
			"triggers": triggerMap[c.DocName],
		}
		batch.Index(id, doc)
		if (i+1)%100 == 0 {
			if err := index.Batch(batch); err != nil {
				return nil, fmt.Errorf("bleve batch: %w", err)
			}
			batch = index.NewBatch()
		}
	}
	if batch.Size() > 0 {
		if err := index.Batch(batch); err != nil {
			return nil, fmt.Errorf("bleve batch final: %w", err)
		}
	}

	return index, nil
}

func buildDocMetadata(docs []chunk.Doc) map[string]DocMeta {
	metadata := make(map[string]DocMeta, len(docs))
	for _, doc := range docs {
		sections := make([]string, len(doc.Sections))
		for i, s := range doc.Sections {
			sections[i] = s.Heading
		}
		metadata[doc.Name] = DocMeta{
			Description: doc.Frontmatter.Description,
			Category:    doc.Frontmatter.Category,
			Triggers:    doc.Frontmatter.Triggers,
			When:        doc.Frontmatter.When,
			Related:     doc.Frontmatter.Related,
			Sections:    sections,
		}
	}
	return metadata
}

func buildToolDescription(docs []chunk.Doc) string {
	var b strings.Builder
	b.WriteString("Get full knowledge document by name. Available documents and trigger keywords:\n")

	categoryDocs := map[string][]chunk.Doc{}
	for _, doc := range docs {
		cat := doc.Frontmatter.Category
		if cat == "" {
			cat = "other"
		}
		categoryDocs[cat] = append(categoryDocs[cat], doc)
	}

	categoryOrder := []string{"core", "patterns", "testing", "principles", "meta", "plan", "style", "other"}
	for _, cat := range categoryOrder {
		catDocs, ok := categoryDocs[cat]
		if !ok {
			continue
		}
		label := strings.ToUpper(cat)
		var parts []string
		for _, doc := range catDocs {
			triggers := strings.Join(doc.Frontmatter.Triggers, ", ")
			// Include section headings for richer keyword matching
			var sectionNames []string
			for _, s := range doc.Sections {
				if s.Heading != "" {
					sectionNames = append(sectionNames, s.Heading)
				}
			}
			entry := doc.Name
			if len(sectionNames) > 0 {
				entry += " [" + strings.Join(sectionNames, ", ") + "]"
			}
			entry += " (" + triggers + ")"
			parts = append(parts, entry)
		}
		fmt.Fprintf(&b, "%s: %s\n", label, strings.Join(parts, " | "))
	}

	return b.String()
}

// enrichTriggers auto-generates trigger keywords from doc content using TF-IDF
// and merges them with the manually curated triggers from frontmatter.
func enrichTriggers(docs []chunk.Doc) {
	df := map[string]int{}
	docTerms := make([]map[string]int, len(docs))

	for i, doc := range docs {
		tf := termFrequency(doc.RawContent)
		docTerms[i] = tf
		for term := range tf {
			df[term]++
		}
	}

	n := float64(len(docs))
	for i, doc := range docs {
		type scored struct {
			term  string
			score float64
		}
		var candidates []scored
		tf := docTerms[i]
		for term, count := range tf {
			if df[term] < 1 {
				continue
			}
			idf := math.Log(n / float64(df[term]))
			tfidf := float64(count) * idf
			candidates = append(candidates, scored{term, tfidf})
		}
		sort.Slice(candidates, func(a, b int) bool {
			return candidates[a].score > candidates[b].score
		})

		existing := map[string]bool{}
		for _, t := range doc.Frontmatter.Triggers {
			existing[strings.ToLower(t)] = true
		}

		added := 0
		maxAuto := 10
		for _, c := range candidates {
			if added >= maxAuto {
				break
			}
			if existing[c.term] {
				continue
			}
			docs[i].Frontmatter.Triggers = append(docs[i].Frontmatter.Triggers, c.term)
			added++
		}
		log.Printf("  %s: %d manual + %d auto triggers", doc.Name, len(existing), added)
	}
}

var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
	"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
	"with": true, "by": true, "from": true, "is": true, "are": true, "was": true,
	"were": true, "be": true, "been": true, "being": true, "have": true, "has": true,
	"had": true, "do": true, "does": true, "did": true, "will": true, "would": true,
	"could": true, "should": true, "may": true, "might": true, "shall": true,
	"can": true, "it": true, "its": true, "this": true, "that": true, "these": true,
	"those": true, "not": true, "no": true, "if": true, "then": true, "else": true,
	"when": true, "where": true, "how": true, "what": true, "which": true, "who": true,
	"all": true, "each": true, "every": true, "any": true, "some": true, "such": true,
	"only": true, "also": true, "just": true, "than": true, "both": true, "into": true,
	"about": true, "up": true, "out": true, "so": true, "as": true, "more": true,
	"most": true, "very": true, "too": true, "here": true, "there": true, "you": true,
	"your": true, "we": true, "our": true, "they": true, "them": true, "their": true,
	"my": true, "me": true, "he": true, "she": true, "him": true, "her": true,
	"use": true, "used": true, "using": true, "see": true, "like": true, "make": true,
	"new": true, "one": true, "two": true, "first": true, "get": true, "set": true,
	"example": true, "need": true, "want": true, "take": true, "run": true,
}

func termFrequency(text string) map[string]int {
	tf := map[string]int{}
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_'
	})
	for _, w := range words {
		if len(w) < 3 {
			continue
		}
		if stopWords[w] {
			continue
		}
		if w == "true" || w == "false" || w == "nil" || w == "null" || w == "string" {
			continue
		}
		tf[w]++
	}
	return tf
}
