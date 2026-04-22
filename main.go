package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/benitogf/detritus/internal/code"
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
			fmt.Fprintln(os.Stderr, "--init is deprecated; use --setup instead")
			os.Exit(1)
		case "--update":
			dryRun := len(os.Args) > 2 && os.Args[2] == "--dry-run"
			self, _ := os.Executable()
			if err := RunUpdate(self, dryRun); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		case "--upsert-mcp":
			// detritus --upsert-mcp <file> <parent-key> <command-path>
			if len(os.Args) != 5 {
				fmt.Fprintln(os.Stderr, "usage: detritus --upsert-mcp <file> <parent-key> <command-path>")
				os.Exit(1)
			}
			upsertMCP(os.Args[2], os.Args[3], os.Args[4])
			return
		case "--upsert-vscode-settings":
			// detritus --upsert-vscode-settings <file>
			if len(os.Args) != 3 {
				fmt.Fprintln(os.Stderr, "usage: detritus --upsert-vscode-settings <file>")
				os.Exit(1)
			}
			upsertVSCodeSettings(os.Args[2])
			return
		case "--setup":
			dryRun := len(os.Args) > 2 && os.Args[2] == "--dry-run"
			self, _ := os.Executable()
			if err := RunSetup(self, dryRun); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		case "--pack":
			if err := runPack(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		case "--packs":
			if err := runPacks(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		case "--refresh":
			if err := runRefresh(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		case "--unpack":
			if err := runUnpack(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		case "--help", "-h":
			fmt.Println("detritus " + version)
			fmt.Println("MCP knowledge base server (stdio transport)")
			fmt.Println("")
			fmt.Println("Usage:")
			fmt.Println("  detritus                                              Start MCP server")
			fmt.Println("  detritus --version                                    Print version")
			fmt.Println("  detritus --list                                       List embedded documents")
			fmt.Println("  detritus --setup [--dry-run]                          Configure all detected IDEs")
			fmt.Println("  detritus --update [--dry-run]                         Self-update to latest release")
			fmt.Println("  detritus --upsert-mcp <file> <key> <cmd>              Upsert MCP config entry")
			fmt.Println("  detritus --upsert-vscode-settings <file>              Upsert VS Code settings")
			fmt.Println("  detritus --pack [name] [root...]                      Create/refresh a workspace pack")
			fmt.Println("  detritus --packs                                      List all packs")
			fmt.Println("  detritus --refresh <name>                             Refresh an existing pack")
			fmt.Println("  detritus --unpack <name>                              Delete a pack")
			fmt.Println("  detritus --help                                       Print this help")
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

	// Build reverse alias map: alias -> canonical doc name
	aliasToDoc := map[string]string{}
	for name := range engine.DocMetadata() {
		alias := aliasForDoc(name)
		aliasToDoc[alias] = name
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "detritus",
		Version: version,
	}, nil)

	codeRegistry := code.NewRegistry()
	defer codeRegistry.Close()
	code.RegisterTools(server, codeRegistry, version)

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
		Name    string `json:"name" jsonschema:"Document name without .md extension (e.g. ooo/package, patterns/coding-style, plan/analyze)"`
		Section string `json:"section,omitempty" jsonschema:"Optional: specific h2 section heading to retrieve instead of full document"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "kb_get",
		Description: engine.ToolDescription(),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetArgs) (*mcp.CallToolResult, any, error) {
		name := resolveDocName(args.Name, aliasToDoc)
		if args.Section != "" {
			content, err := engine.GetSection(name, args.Section)
			if err != nil {
				return errResult(fmt.Sprintf("Document '%s' not found. Use kb_list to see available documents.", args.Name)), nil, nil
			}
			return textResult(content), nil, nil
		}
		content, err := engine.GetDoc(name)
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
		Description: "Search across all knowledge base documents for a specific topic, pattern, or API name. Returns matching lines with context.",
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

	type SectionsArgs struct {
		Name string `json:"name" jsonschema:"Document name (e.g. ooo/package). Use kb_list to find valid names."`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "kb_sections",
		Description: "List the h2 sections available in a document. Use before kb_get with section= to retrieve only the relevant part of large documents instead of the full content.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args SectionsArgs) (*mcp.CallToolResult, any, error) {
		name := resolveDocName(args.Name, aliasToDoc)
		sections, err := engine.GetSections(name)
		if err != nil {
			return errResult(fmt.Sprintf("Document '%s' not found. Use kb_list to see available documents.", args.Name)), nil, nil
		}
		if len(sections) == 0 {
			return textResult(fmt.Sprintf("Document '%s' has no named sections (single block).", name)), nil, nil
		}
		var b strings.Builder
		fmt.Fprintf(&b, "Sections in %s:\n", name)
		for _, s := range sections {
			fmt.Fprintf(&b, "- %s\n", s)
		}
		return textResult(b.String()), nil, nil
	})

	var resourceSummary strings.Builder
	resourceSummary.WriteString("# Detritus Knowledge Base\n\n")
	resourceSummary.WriteString("Available documents and tools: kb_get(name, section?), kb_list(), kb_search(query), kb_sections(name)\n\n")
	for name, meta := range engine.DocMetadata() {
		fmt.Fprintf(&resourceSummary, "- **%s**: %s\n", name, meta.Description)
	}

	server.AddResource(&mcp.Resource{
		URI:         "mcp://detritus",
		Name:        "detritus",
		Description: "Summary of all available knowledge base documents and tools",
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

// resolveDocName normalises a requested name into a canonical doc path.
// It handles exact matches, aliases (e.g. "plan" -> "plan/analyze"),
// underscore/slash-prefixed variants (e.g. "_truthseeker" -> "meta/truthseeker"),
// and hyphen-to-slash fallback (e.g. "ooo-package" -> "ooo/package").
func resolveDocName(raw string, aliasToDoc map[string]string) string {
	// Strip leading slashes and underscores
	norm := strings.TrimLeft(raw, "/_")

	// 1. Raw is a known alias (e.g. "plan")
	if doc, ok := aliasToDoc[raw]; ok {
		return doc
	}

	// 2. Normalised form is a known alias (e.g. "_truthseeker" -> "truthseeker")
	if doc, ok := aliasToDoc[norm]; ok {
		return doc
	}

	// 3. Normalised form is already a canonical doc path (e.g. "meta/truthseeker")
	if strings.Contains(norm, "/") {
		return norm
	}

	// 4. Fallback: return normalised form (let GetDoc report not-found)
	return norm
}

func aliasForDoc(name string) string {
	parts := strings.SplitN(name, "/", 2)
	leaf := parts[len(parts)-1]
	switch {
	case leaf == "index" && len(parts) == 2:
		return parts[0] // testing/index -> testing, plan/index -> plan
	case strings.HasPrefix(name, "testing/go-backend-"):
		return "testing-" + leaf
	case strings.HasPrefix(name, "ooo/"):
		return "ooo-" + leaf
	default:
		return leaf
	}
}

// upsertMCP reads a JSON file, sets .<parentKey>.detritus = {command, args:[]},
// and writes it back. Creates the file if it doesn't exist.
func upsertMCP(file, parentKey, command string) {
	data := map[string]any{}
	if raw, err := os.ReadFile(file); err == nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, &data); err != nil {
			fmt.Fprintf(os.Stderr, "failed to parse %s: %v\n", file, err)
			os.Exit(1)
		}
	}

	parent, ok := data[parentKey].(map[string]any)
	if !ok {
		parent = map[string]any{}
	}
	parent["detritus"] = map[string]any{
		"command": command,
		"args":    []any{},
	}
	data[parentKey] = parent

	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to marshal JSON: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create directory: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(file, append(out, '\n'), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write %s: %v\n", file, err)
		os.Exit(1)
	}
	fmt.Printf("Updated detritus in %s\n", file)
}

// upsertVSCodeSettings reads a VS Code settings.json and sets the
// chat.promptFilesLocations, chat.instructionsFilesLocations, and
// chat.agentFilesLocations keys. Creates the file if it doesn't exist.
func upsertVSCodeSettings(file string) {
	data := map[string]any{}
	if raw, err := os.ReadFile(file); err == nil {
		if err := json.Unmarshal(raw, &data); err != nil {
			fmt.Fprintf(os.Stderr, "failed to parse %s: %v\n", file, err)
			os.Exit(1)
		}
	}

	setLocationMap := func(key string, entries map[string]bool) {
		existing, _ := data[key].(map[string]any)
		if existing == nil {
			existing = map[string]any{}
		}
		for k, v := range entries {
			existing[k] = v
		}
		data[key] = existing
	}

	setLocationMap("chat.promptFilesLocations", map[string]bool{
		".github/prompts":    false,
		"~/.copilot/prompts": true,
	})
	setLocationMap("chat.instructionsFilesLocations", map[string]bool{
		"~/.copilot/instructions": true,
	})
	setLocationMap("chat.agentFilesLocations", map[string]bool{
		"~/.copilot/agents": true,
	})

	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to marshal JSON: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create directory: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(file, append(out, '\n'), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write %s: %v\n", file, err)
		os.Exit(1)
	}
	fmt.Printf("Updated %s\n", file)
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
