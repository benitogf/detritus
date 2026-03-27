package chunk

import (
	"fmt"
	"io/fs"
	"strings"
)

type Frontmatter struct {
	Description string
	Category    string
	Triggers    []string
	When        string
	Related     []string
}

type Section struct {
	Heading string
	Content string
}

type Chunk struct {
	DocName  string
	Section  string
	Content  string
	Position int
}

type Doc struct {
	Name        string
	Frontmatter Frontmatter
	Sections    []Section
	RawContent  string
}

func ParseAll(docsFS fs.FS, root string) ([]Doc, error) {
	var docs []Doc
	err := fs.WalkDir(docsFS, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		content, err := fs.ReadFile(docsFS, path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		name := strings.TrimSuffix(strings.TrimPrefix(path, root+"/"), ".md")
		doc := ParseDoc(name, string(content))
		docs = append(docs, doc)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return docs, nil
}

func ParseDoc(name, content string) Doc {
	fm, body := parseFrontmatter(content)
	sections := parseSections(body)
	return Doc{
		Name:        name,
		Frontmatter: fm,
		Sections:    sections,
		RawContent:  content,
	}
}

func ChunkDocs(docs []Doc) []Chunk {
	var chunks []Chunk
	for _, doc := range docs {
		for i, sec := range doc.Sections {
			chunks = append(chunks, Chunk{
				DocName:  doc.Name,
				Section:  sec.Heading,
				Content:  sec.Content,
				Position: i,
			})
		}
	}
	return chunks
}

func parseFrontmatter(content string) (Frontmatter, string) {
	var fm Frontmatter
	if !strings.HasPrefix(content, "---\n") {
		return fm, content
	}
	end := strings.Index(content[4:], "\n---")
	if end == -1 {
		return fm, content
	}
	fmBlock := content[4 : 4+end]
	body := content[4+end+4:]

	var currentList *[]string
	for _, line := range strings.Split(fmBlock, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") && currentList != nil {
			*currentList = append(*currentList, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
			continue
		}
		currentList = nil
		if strings.HasPrefix(trimmed, "description:") {
			fm.Description = strings.TrimSpace(strings.TrimPrefix(trimmed, "description:"))
		} else if strings.HasPrefix(trimmed, "category:") {
			fm.Category = strings.TrimSpace(strings.TrimPrefix(trimmed, "category:"))
		} else if strings.HasPrefix(trimmed, "when:") {
			fm.When = strings.TrimSpace(strings.TrimPrefix(trimmed, "when:"))
		} else if trimmed == "triggers:" {
			currentList = &fm.Triggers
		} else if trimmed == "related:" {
			currentList = &fm.Related
		}
	}
	return fm, body
}

func parseSections(body string) []Section {
	lines := strings.Split(body, "\n")
	var sections []Section
	var currentHeading string
	var currentLines []string

	flush := func() {
		content := strings.TrimSpace(strings.Join(currentLines, "\n"))
		if content != "" {
			sections = append(sections, Section{
				Heading: currentHeading,
				Content: content,
			})
		}
	}

	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			flush()
			currentHeading = strings.TrimPrefix(line, "## ")
			currentLines = nil
			continue
		}
		currentLines = append(currentLines, line)
	}
	flush()

	return sections
}
