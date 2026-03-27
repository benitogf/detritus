package search

import (
	"encoding/gob"
	"fmt"
	"io/fs"
	"math"
	"sort"
	"strings"

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

type GeneratedData struct {
	Chunks      []ChunkMeta
	Vectors     [][]float32
	BlevePath   string
	ToolDesc    string
	DocMetadata map[string]DocMeta
}

type Result struct {
	DocName  string
	Section  string
	Score    float64
	Snippet  string
	Position int
}

type Engine struct {
	data         GeneratedData
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

	bleveResults, err := e.bleveSearch(query, topN*2)
	if err != nil {
		return nil, err
	}

	cosineResults := e.cosineSearch(query, topN*2)

	merged := e.mergeResults(bleveResults, cosineResults, 0.5, 0.5)

	merged = e.mmrRerank(merged, 0.7, topN)

	var filtered []Result
	for _, r := range merged {
		if r.Score >= 0.45 {
			filtered = append(filtered, r)
		}
	}

	return filtered, nil
}

func (e *Engine) ToolDescription() string {
	return e.data.ToolDesc
}

func (e *Engine) DocMetadata() map[string]DocMeta {
	return e.data.DocMetadata
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
			Snippet: truncate(content, 200),
		})
	}
	return results, nil
}

func (e *Engine) cosineSearch(query string, topN int) []Result {
	queryVec := e.queryVector(query)
	if queryVec == nil {
		return nil
	}

	type scored struct {
		index int
		score float64
	}
	var scores []scored
	for i, vec := range e.data.Vectors {
		s := cosineSimilarity(queryVec, vec)
		scores = append(scores, scored{i, s})
	}
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	limit := topN
	if limit > len(scores) {
		limit = len(scores)
	}

	var results []Result
	for _, s := range scores[:limit] {
		meta := e.data.Chunks[s.index]
		results = append(results, Result{
			DocName:  meta.DocName,
			Section:  meta.Section,
			Score:    s.score,
			Position: meta.Position,
		})
	}
	return results
}

func (e *Engine) queryVector(query string) []float32 {
	queryLower := strings.ToLower(query)
	tokens := strings.Fields(queryLower)

	var bestIdx int
	var bestScore float64
	for i, chunk := range e.data.Chunks {
		score := tokenOverlap(tokens, strings.ToLower(chunk.DocName+" "+chunk.Section))
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}

	if bestScore > 0 && bestIdx < len(e.data.Vectors) {
		return e.data.Vectors[bestIdx]
	}

	if len(e.data.Vectors) > 0 {
		centroid := make([]float32, len(e.data.Vectors[0]))
		for _, v := range e.data.Vectors {
			for j, val := range v {
				centroid[j] += val
			}
		}
		n := float32(len(e.data.Vectors))
		for j := range centroid {
			centroid[j] /= n
		}
		return centroid
	}
	return nil
}

func (e *Engine) mergeResults(bleveResults, cosineResults []Result, bleveWeight, cosineWeight float64) []Result {
	type key struct {
		doc     string
		section string
	}
	merged := map[key]*Result{}

	var maxBleve float64
	for _, r := range bleveResults {
		if r.Score > maxBleve {
			maxBleve = r.Score
		}
	}
	if maxBleve == 0 {
		maxBleve = 1
	}

	for _, r := range bleveResults {
		k := key{r.DocName, r.Section}
		normalized := r.Score / maxBleve
		merged[k] = &Result{
			DocName: r.DocName,
			Section: r.Section,
			Score:   normalized * bleveWeight,
			Snippet: r.Snippet,
		}
	}

	for _, r := range cosineResults {
		k := key{r.DocName, r.Section}
		cosineNorm := (r.Score + 1) / 2
		if existing, ok := merged[k]; ok {
			existing.Score += cosineNorm * cosineWeight
		} else {
			merged[k] = &Result{
				DocName: r.DocName,
				Section: r.Section,
				Score:   cosineNorm * cosineWeight,
			}
		}
	}

	var results []Result
	for _, r := range merged {
		results = append(results, *r)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	return results
}

func (e *Engine) mmrRerank(results []Result, lambda float64, topN int) []Result {
	if len(results) <= 1 {
		return results
	}

	selected := []Result{results[0]}
	remaining := results[1:]

	for len(selected) < topN && len(remaining) > 0 {
		var bestIdx int
		var bestScore float64 = -math.MaxFloat64

		for i, candidate := range remaining {
			var maxSim float64
			for _, sel := range selected {
				sim := docSimilarity(candidate, sel)
				if sim > maxSim {
					maxSim = sim
				}
			}
			mmrScore := lambda*candidate.Score - (1-lambda)*maxSim
			if mmrScore > bestScore {
				bestScore = mmrScore
				bestIdx = i
			}
		}

		selected = append(selected, remaining[bestIdx])
		remaining = append(remaining[:bestIdx], remaining[bestIdx+1:]...)
	}

	return selected
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
	for name, meta := range data.DocMetadata {
		triggerMap[name] = strings.Join(meta.Triggers, " ")
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
	for name := range data.DocMetadata {
		content, err := fs.ReadFile(docsFS, docsDir+"/"+name+".md")
		if err != nil {
			return nil, fmt.Errorf("read doc %s: %w", name, err)
		}
		docContent[name] = string(content)
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
			heading := strings.TrimPrefix(line, "## ")
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

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}

func tokenOverlap(tokens []string, text string) float64 {
	var matches int
	for _, t := range tokens {
		if strings.Contains(text, t) {
			matches++
		}
	}
	if len(tokens) == 0 {
		return 0
	}
	return float64(matches) / float64(len(tokens))
}

func docSimilarity(a, b Result) float64 {
	if a.DocName == b.DocName {
		if a.Section == b.Section {
			return 1.0
		}
		return 0.5
	}
	return 0.0
}
