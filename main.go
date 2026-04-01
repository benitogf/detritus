package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/benitogf/detritus/internal/search"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

//go:generate go run ./cmd/generate/

var version = "dev"

//go:embed docs
var docsFS embed.FS

//go:embed generated/data.gob
var dataFS embed.FS

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v":
			fmt.Println("detritus " + version)
			return
		case "--list", "-l":
			_ = fs.WalkDir(docsFS, "docs", func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
					return nil
				}
				name := strings.TrimSuffix(strings.TrimPrefix(path, "docs/"), ".md")
				content, _ := fs.ReadFile(docsFS, path)
				desc := extractDescription(string(content))
				fmt.Printf("%s\t%s\n", name, desc)
				return nil
			})
			return
		case "--init":
			initPromptFiles()
			return
		case "--help", "-h":
			fmt.Println("detritus " + version)
			fmt.Println("MCP knowledge base server (stdio transport)")
			fmt.Println("")
			fmt.Println("Usage:")
			fmt.Println("  detritus              Start MCP server (used by Windsurf/VS Code)")
			fmt.Println("  detritus --version    Print version")
			fmt.Println("  detritus --list       List embedded documents (name<TAB>description)")
			fmt.Println("  detritus --init       Generate .github/prompts/ for VS Code slash commands")
			fmt.Println("  detritus --help       Print this help")
			fmt.Println("")
			fmt.Println("This server communicates via stdio using the Model Context Protocol.")
			fmt.Println("Windsurf or VS Code spawns it automatically via MCP config.")
			return
		default:
			fmt.Fprintf(os.Stderr, "unknown flag: %s\nRun 'detritus --help' for usage.\n", os.Args[1])
			os.Exit(1)
		}
	}

	engine, err := search.New(dataFS, "generated/data.gob", docsFS, "docs")
	if err != nil {
		log.Fatalf("search engine init: %v", err)
	}
	defer engine.Close()

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "ooo-knowledge-base",
		Version: version,
	}, nil)

	type ListArgs struct{}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "kb_list",
		Description: "List all available knowledge base documents with descriptions",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ListArgs) (*mcp.CallToolResult, any, error) {
		var b strings.Builder
		for name, meta := range engine.DocMetadata() {
			fmt.Fprintf(&b, "- **%s**: %s\n", name, meta.Description)
		}
		return textResult(b.String()), nil, nil
	})

	type GetArgs struct {
		Name    string `json:"name" jsonschema:"Document name without .md extension (e.g. ooo/package, scaffold/create, plan/analyze)"`
		Section string `json:"section,omitempty" jsonschema:"Optional: specific h2 section heading to retrieve instead of full document"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "kb_get",
		Description: engine.ToolDescription(),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetArgs) (*mcp.CallToolResult, any, error) {
		if args.Section != "" {
			content, err := engine.GetSection(args.Name, args.Section)
			if err != nil {
				return errResult(fmt.Sprintf("Document '%s' not found. Use kb_list to see available documents.", args.Name)), nil, nil
			}
			return textResult(content), nil, nil
		}
		content, err := engine.GetDoc(args.Name)
		if err != nil {
			return errResult(fmt.Sprintf("Document '%s' not found. Use kb_list to see available documents.", args.Name)), nil, nil
		}
		return textResult(content), nil, nil
	})

	type SearchArgs struct {
		Query string `json:"query" jsonschema:"Search term, API name, or topic to find across all documents"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "kb_search",
		Description: "Search across all ooo ecosystem knowledge base documents for a specific topic, pattern, or API name. Returns matching lines with context.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args SearchArgs) (*mcp.CallToolResult, any, error) {
		results, err := engine.Search(args.Query, 10)
		if err != nil {
			return errResult("search failed: " + err.Error()), nil, nil
		}
		if len(results) == 0 {
			return textResult("No results found for: " + args.Query), nil, nil
		}
		var b strings.Builder
		for _, r := range results {
			section := r.Section
			if section == "" {
				section = "(intro)"
			}
			fmt.Fprintf(&b, "## %s — %s (score: %.3f)\n", r.DocName, section, r.Score)
			if r.Snippet != "" {
				fmt.Fprintf(&b, "%s\n", r.Snippet)
			}
			b.WriteString("\n")
		}
		return textResult(b.String()), nil, nil
	})

	var resourceSummary strings.Builder
	resourceSummary.WriteString("# ooo Knowledge Base\n\n")
	resourceSummary.WriteString("Available documents and tools: kb_get(name, section?), kb_list(), kb_search(query)\n\n")
	for name, meta := range engine.DocMetadata() {
		fmt.Fprintf(&resourceSummary, "- **%s**: %s\n", name, meta.Description)
	}

	server.AddResource(&mcp.Resource{
		URI:         "mcp://detritus",
		Name:        "ooo-knowledge-base",
		Description: "Summary of all available ooo ecosystem knowledge base documents and tools",
		MIMEType:    "text/markdown",
	}, func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:      "mcp://detritus",
				MIMEType: "text/markdown",
				Text:     resourceSummary.String(),
			}},
		}, nil
	})

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}

func aliasForDoc(name string) string {
	parts := strings.SplitN(name, "/", 2)
	leaf := parts[len(parts)-1]
	switch {
	case name == "plan/analyze":
		return "plan"
	case name == "plan/export":
		return "plan-export"
	case name == "plan/diagrams":
		return "diagrams"
	case name == "testing/index":
		return "testing"
	case strings.HasPrefix(name, "testing/go-backend-"):
		return "testing-" + leaf
	case strings.HasPrefix(name, "ooo/"):
		return "ooo-" + leaf
	default:
		return leaf
	}
}

func initPromptFiles() {
	promptsDir := filepath.Join(".github", "prompts")
	if err := os.MkdirAll(promptsDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create %s: %v\n", promptsDir, err)
		os.Exit(1)
	}

	// Track which files we generate so we can clean stale ones
	generated := map[string]bool{}

	count := 0
	_ = fs.WalkDir(docsFS, "docs", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		name := strings.TrimSuffix(strings.TrimPrefix(path, "docs/"), ".md")
		content, _ := fs.ReadFile(docsFS, path)
		desc := extractDescription(string(content))
		alias := aliasForDoc(name)
		filename := alias + ".prompt.md"
		generated[filename] = true

		prompt := fmt.Sprintf("---\ndescription: %s\nagent: agent\ntools: [\"detritus/*\"]\n---\n\nCall kb_get(name=\"%s\") and follow the instructions in the returned document.\n", desc, name)

		fpath := filepath.Join(promptsDir, filename)
		if err := os.WriteFile(fpath, []byte(prompt), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "  warning: could not write %s: %v\n", fpath, err)
			return nil
		}
		count++
		return nil
	})

	// Remove stale detritus-generated prompt files
	entries, _ := os.ReadDir(promptsDir)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".prompt.md") {
			continue
		}
		if generated[e.Name()] {
			continue
		}
		fpath := filepath.Join(promptsDir, e.Name())
		data, err := os.ReadFile(fpath)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), "kb_get") {
			os.Remove(fpath)
			fmt.Printf("  removed stale: %s\n", e.Name())
		}
	}

	fmt.Printf("Generated %d prompt files in %s/\n", count, promptsDir)
	fmt.Println("Reload VS Code window (Developer: Reload Window) to activate slash commands.")
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}
}

func errResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
		IsError: true,
	}
}

func extractDescription(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "description:") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "description:"))
		}
	}
	return ""
}
