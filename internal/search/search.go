package search

import (
	"encoding/gob"
	"fmt"
	"io/fs"
	"strings"

	"github.com/benitogf/detritus/internal/core"
	"github.com/blevesearch/bleve/v2"
)

type ChunkMeta struct {
	DocName  string
	Section  string
	Position int
}

type DocMeta struct {
	Description string
	Category    string
	Triggers    []string
	When        string
	Related     []string
	Sections    []string
}

// DocEntry pairs a doc name with its metadata. Serialized as a slice (sorted by
// Name in cmd/generate) rather than a map so data.gob is byte-reproducible —
// gob encodes maps in randomized iteration order, which made every `go generate`
// produce a different blob.
type DocEntry struct {
	Name string
	Meta DocMeta
}

type GeneratedData struct {
	Chunks      []ChunkMeta
	BlevePath   string
	ToolDesc    string
	DocMetadata []DocEntry
}

// Result is the shared retrieval hit type (core.Result), aliased so kb_* callers
// keep referencing search.Result unchanged.
type Result = core.Result

type Engine struct {
	data         GeneratedData
	docMeta      map[string]DocMeta
	index        bleve.Index
	docsFS       fs.FS
	docsDir      string
	chunkContent []string
}

func New(dataFS fs.FS, dataPath string, docsFS fs.FS, docsDir string) (*Engine, error) {
	f, err := dataFS.Open(dataPath)
	if err != nil {
		return nil, fmt.Errorf("open data: %w", err)
	}
	defer f.Close()

	var data GeneratedData
	if err := gob.NewDecoder(f).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode data: %w", err)
	}

	docMeta := make(map[string]DocMeta, len(data.DocMetadata))
	for _, entry := range data.DocMetadata {
		docMeta[entry.Name] = entry.Meta
	}

	chunkContent, err := loadChunkContent(docsFS, docsDir, data)
	if err != nil {
		return nil, fmt.Errorf("load chunk content: %w", err)
	}

	index, err := buildIndex(data, chunkContent)
	if err != nil {
		return nil, fmt.Errorf("build index: %w", err)
	}

	return &Engine{
		data:         data,
		docMeta:      docMeta,
		index:        index,
		docsFS:       docsFS,
		docsDir:      docsDir,
		chunkContent: chunkContent,
	}, nil
}

func (e *Engine) Close() error {
	return e.index.Close()
}

func (e *Engine) Search(query string, topN int) ([]Result, error) {
	if topN <= 0 {
		topN = 10
	}

	raw, err := e.bleveSearch(query, topN*3)
	if err != nil {
		return nil, err
	}
	// Shared pipeline: normalize → MMR-rerank → threshold-filter.
	return core.Rank(raw, core.DefaultMMRLambda, topN, core.DefaultMinScore), nil
}

func (e *Engine) ToolDescription() string {
	return e.data.ToolDesc
}

func (e *Engine) DocMetadata() map[string]DocMeta {
	return e.docMeta
}

func (e *Engine) GetDoc(name string) (string, error) {
	content, err := fs.ReadFile(e.docsFS, e.docsDir+"/"+name+".md")
	if err != nil {
		return "", fmt.Errorf("doc %q not found", name)
	}
	return string(content), nil
}

func (e *Engine) GetSection(name, section string) (string, error) {
	content, err := e.GetDoc(name)
	if err != nil {
		return "", err
	}
	return extractSection(content, section), nil
}

func (e *Engine) GetSections(name string) ([]string, error) {
	meta, ok := e.docMeta[name]
	if !ok {
		return nil, fmt.Errorf("doc %q not found", name)
	}
	var sections []string
	for _, s := range meta.Sections {
		if s != "" {
			sections = append(sections, s)
		}
	}
	return sections, nil
}

func (e *Engine) bleveSearch(query string, topN int) ([]Result, error) {
	q := bleve.NewMatchQuery(query)
	req := bleve.NewSearchRequestOptions(q, topN, 0, false)
	req.Fields = []string{"doc_name", "section", "content"}
	searchResult, err := e.index.Search(req)
	if err != nil {
		return nil, fmt.Errorf("bleve search: %w", err)
	}

	var results []Result
	for _, hit := range searchResult.Hits {
		docName, _ := hit.Fields["doc_name"].(string)
		section, _ := hit.Fields["section"].(string)
		content, _ := hit.Fields["content"].(string)
		results = append(results, Result{
			DocName: docName,
			Section: section,
			Score:   hit.Score,
			Snippet: core.Truncate(content, 200),
		})
	}
	return results, nil
}

func buildIndex(data GeneratedData, chunkContent []string) (bleve.Index, error) {
	mapping := bleve.NewIndexMapping()

	docMapping := bleve.NewDocumentMapping()
	docMapping.AddFieldMappingsAt("doc_name", bleve.NewTextFieldMapping())
	docMapping.AddFieldMappingsAt("section", bleve.NewTextFieldMapping())
	docMapping.AddFieldMappingsAt("content", bleve.NewTextFieldMapping())
	docMapping.AddFieldMappingsAt("triggers", bleve.NewTextFieldMapping())
	mapping.AddDocumentMapping("chunk", docMapping)
	mapping.DefaultMapping = docMapping

	index, err := bleve.NewMemOnly(mapping)
	if err != nil {
		return nil, err
	}

	triggerMap := map[string]string{}
	for _, entry := range data.DocMetadata {
		triggerMap[entry.Name] = strings.Join(entry.Meta.Triggers, " ")
	}

	batch := index.NewBatch()
	for i, chunk := range data.Chunks {
		id := fmt.Sprintf("%s#%d", chunk.DocName, chunk.Position)
		content := ""
		if i < len(chunkContent) {
			content = chunkContent[i]
		}
		doc := map[string]string{
			"doc_name": chunk.DocName,
			"section":  chunk.Section,
			"content":  content,
			"triggers": triggerMap[chunk.DocName],
		}
		batch.Index(id, doc)
	}
	if err := index.Batch(batch); err != nil {
		return nil, err
	}

	return index, nil
}

func loadChunkContent(docsFS fs.FS, docsDir string, data GeneratedData) ([]string, error) {
	docContent := map[string]string{}
	for _, entry := range data.DocMetadata {
		content, err := fs.ReadFile(docsFS, docsDir+"/"+entry.Name+".md")
		if err != nil {
			return nil, fmt.Errorf("read doc %s: %w", entry.Name, err)
		}
		docContent[entry.Name] = string(content)
	}

	result := make([]string, len(data.Chunks))
	for i, chunk := range data.Chunks {
		raw, ok := docContent[chunk.DocName]
		if !ok {
			continue
		}
		result[i] = extractSection(raw, chunk.Section)
	}
	return result, nil
}

func extractSection(content, section string) string {
	if section == "" {
		return content
	}
	lines := strings.Split(content, "\n")
	var sectionLines []string
	inSection := false
	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			heading := strings.TrimSpace(strings.TrimPrefix(line, "## "))
			if strings.EqualFold(heading, section) {
				inSection = true
				continue
			}
			if inSection {
				break
			}
		}
		if inSection {
			sectionLines = append(sectionLines, line)
		}
	}
	if len(sectionLines) == 0 {
		return content
	}
	return strings.TrimSpace(strings.Join(sectionLines, "\n"))
}
