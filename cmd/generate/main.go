package main

import (
	"encoding/gob"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/benitogf/detritus/internal/chunk"
	"github.com/blevesearch/bleve/v2"
	"github.com/knights-analytics/hugot"
)

const (
	modelName = "KnightsAnalytics/all-MiniLM-L6-v2"
	batchSize = 32
)

type ChunkMeta struct {
	DocName  string
	Section  string
	Position int
}

type GeneratedData struct {
	Chunks     []ChunkMeta
	Vectors    [][]float32
	BlevePath  string
	ToolDesc   string
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
	modelsDir := "models"
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

	log.Println("downloading model (if needed)...")
	modelPath := filepath.Join(modelsDir, strings.ReplaceAll(modelName, "/", "_"))
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		modelPath, err = hugot.DownloadModel(modelName, modelsDir, hugot.NewDownloadOptions())
		if err != nil {
			log.Fatalf("download model: %v", err)
		}
	}
	log.Printf("model at %s", modelPath)

	log.Println("generating embeddings...")
	vectors, err := generateEmbeddings(modelPath, chunks)
	if err != nil {
		log.Fatalf("embeddings: %v", err)
	}
	log.Printf("generated %d vectors of dim %d", len(vectors), len(vectors[0]))

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
		Vectors:     vectors,
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

func generateEmbeddings(modelPath string, chunks []chunk.Chunk) ([][]float32, error) {
	session, err := hugot.NewGoSession()
	if err != nil {
		return nil, fmt.Errorf("hugot session: %w", err)
	}
	defer session.Destroy()

	config := hugot.FeatureExtractionConfig{
		ModelPath:    modelPath,
		Name:         "embedder",
		OnnxFilename: "model.onnx",
	}
	pipeline, err := hugot.NewPipeline(session, config)
	if err != nil {
		return nil, fmt.Errorf("hugot pipeline: %w", err)
	}

	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.DocName + " " + c.Section + " " + c.Content
	}

	var allVectors [][]float32
	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[i:end]
		result, err := pipeline.RunPipeline(batch)
		if err != nil {
			return nil, fmt.Errorf("hugot run batch %d: %w", i/batchSize, err)
		}
		allVectors = append(allVectors, result.Embeddings...)
		log.Printf("  embedded %d/%d chunks", len(allVectors), len(texts))
	}

	return allVectors, nil
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

	categoryOrder := []string{"core", "patterns", "testing", "principles", "meta", "scaffold", "plan", "style", "other"}
	for _, cat := range categoryOrder {
		catDocs, ok := categoryDocs[cat]
		if !ok {
			continue
		}
		label := strings.ToUpper(cat)
		var parts []string
		for _, doc := range catDocs {
			triggers := strings.Join(doc.Frontmatter.Triggers, ", ")
			parts = append(parts, fmt.Sprintf("%s (%s)", doc.Name, triggers))
		}
		fmt.Fprintf(&b, "%s: %s\n", label, strings.Join(parts, " | "))
	}

	return b.String()
}
