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
	"github.com/benitogf/detritus/internal/memory"
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
		case "--todo-guard":
			// PreToolUse hook handler (installed by --setup for Claude Code).
			// Reads the hook payload on stdin; denies main-session writes to the
			// /todo cross-session store. Always exits 0 (fails open).
			_ = runTodoGuard()
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
		case "--readme":
			if err := writeReadmeCommands("README.md"); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		case "--plugin-commands":
			if err := writePluginCommands(pluginCommandsDir); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		case "--setup":
			dryRun := len(os.Args) > 2 && os.Args[2] == "--dry-run"
			self, _ := os.Executable()
			if err := RunSetup(self, dryRun); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		case "--candyland-up":
			// detritus --candyland-up
			// Bring the candyland sidecar online (health check, start it detached if
			// down, poll until ready) WITHOUT starting any run — the bare entry the
			// dashboard / observability flows use before launching work, and the one
			// /babysit uses to be sure the sidecar is reachable before it watches a PR.
			self, _ := os.Executable()
			if err := runCandylandUp(self); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		case "--candyland-run":
			// detritus --candyland-run <prompt-file> [folder ...]
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "usage: detritus --candyland-run <prompt-file> [folder ...]")
				os.Exit(1)
			}
			self, _ := os.Executable()
			cwd, _ := os.Getwd()
			if err := runCandyland(self, os.Args[2], os.Args[3:], cwd); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		case "--quest-run":
			// detritus --quest-run <objective-file> [folder ...]
			// Starts a standalone Candyland-native iterative quest (the /quest
			// command): a BOUNDED loop that converges to one PR per repo. The
			// DELIVERY mode is classified from the objective against live gh state
			// (resolveLaunchDelivery, the gh-mirror classifier all launchers
			// share); the launch summary states the mode honestly. There is no
			// autonomy axis — once planning settles, work runs to done.
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "usage: detritus --quest-run <objective-file> [folder ...]")
				os.Exit(1)
			}
			self, _ := os.Executable()
			cwd, _ := os.Getwd()
			if err := runQuestCmd(self, os.Args[2], os.Args[3:], "quest", "converge", cwd); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		case "--adventure-run":
			// detritus --adventure-run <objective-file> [folder ...]
			// Starts an open-ended freeseeking adventure (the /adventure command):
			// the same quest machinery with a per-finding delivery policy — a PR
			// per accepted finding, perpetual until stopped. Same gh-mirror
			// classification as --quest-run.
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "usage: detritus --adventure-run <objective-file> [folder ...]")
				os.Exit(1)
			}
			self, _ := os.Executable()
			cwd, _ := os.Getwd()
			if err := runQuestCmd(self, os.Args[2], os.Args[3:], "adventure", "perFinding", cwd); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		case "--campaign-run":
			// detritus --campaign-run <input-file> [folder ...]
			// Starts a Candyland program-level campaign (the /campaign command)
			// from a high-level goal, partial brief, or detailed plan. The DELIVERY
			// mode is classified from the input against live gh state
			// (resolveLaunchDelivery, the gh-mirror classifier all launchers
			// share): a feedback/review-on-PR input updates/reviews that PR,
			// otherwise it opens a new PR (deliver:"pr", the default). There is no
			// autonomy axis.
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "usage: detritus --campaign-run <input-file> [folder ...]")
				os.Exit(1)
			}
			self, _ := os.Executable()
			cwd, _ := os.Getwd()
			if err := runCampaignCmd(self, os.Args[2], os.Args[3:], cwd); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		case "--contribute":
			// detritus --contribute [--repo owner/name] [--dir path] [--dry-run]
			// The lesson gateway: gather EVERY local lesson and ship it into a
			// shared lessons/ dir in the target repo via the normal PR flow. Not a
			// gate/filter — the PR review is the gate; a conservative secret
			// redactor is the only content transform (transport hygiene). --dry-run
			// prints the branch/files/PR it would open and makes NO git/gh changes.
			cwd, _ := os.Getwd()
			if err := runContribute(cwd, os.Args[2:]); err != nil {
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
			fmt.Println("  detritus --readme                                     Regenerate the README command table from docs/flows/")
			fmt.Println("  detritus --plugin-commands                            Regenerate plugin command shims from docs/flows/")
			fmt.Println("  detritus --setup [--dry-run]                          Configure all detected IDEs")
			fmt.Println("  detritus --candyland-up                               Bring the candyland sidecar online (no run)")
			fmt.Println("  detritus --candyland-run <prompt-file> [folder ...]   Start a candyland sidecar build run over REST")
			fmt.Println("  detritus --quest-run <objective-file> [folder ...]   Start a candyland-native bounded quest over REST")
			fmt.Println("  detritus --adventure-run <objective-file> [folder ...] Start a candyland open-ended adventure over REST")
			fmt.Println("  detritus --campaign-run <input-file> [folder ...]    Start a candyland program-level campaign over REST")
			fmt.Println("  detritus --contribute [--repo o/n] [--dir d] [--dry-run] Ship all local lessons into a repo's lessons/ dir via PR")
			fmt.Println("  detritus --update [--dry-run]                         Self-update to latest release")
			fmt.Println("  detritus --todo-guard                                 PreToolUse hook handler (internal; installed by --setup)")
			fmt.Println("  detritus --upsert-mcp <file> <key> <cmd>              Upsert MCP config entry")
			fmt.Println("  detritus --upsert-vscode-settings <file>              Upsert VS Code settings")
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

	server := buildMCPServer(engine)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}

// buildMCPServer constructs the detritus MCP server with all code/memory/kb
// tools and the summary resource registered, ready to be served over the stdio
// transport. detritus is a passive stdio MCP server: each consumer (a VSCode
// Claude session, or a candyland-spawned agent) runs its own detritus child
// over stdio, so there is no long-lived shared server.
func buildMCPServer(engine *search.Engine) *mcp.Server {
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

	code.RegisterSeamlessTools(server)
	memory.RegisterTools(server)

	type ListArgs struct{}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "kb_list",
		Description: "List all available knowledge base documents with descriptions",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ListArgs) (*mcp.CallToolResult, any, error) {
		var b strings.Builder
		for name, meta := range engine.DocMetadata() {
			fmt.Fprintf(&b, "- **%s**: %s\n", name, meta.Description)
		}
		return textResult(b.String()), nil, nil
	})

	type GetArgs struct {
		Name    string `json:"name" jsonschema:"Document name without .md extension (e.g. ooo/package, flows/principles/coding-style, flows/plan/plan)"`
		Section string `json:"section,omitempty" jsonschema:"Optional: specific h2 section heading to retrieve instead of full document"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "kb_get",
		Description: engine.ToolDescription(),
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)},
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
		Description: "Search across all knowledge base documents for a specific topic, pattern, or API name. Returns matching lines with context, a structured hits/next result, and resource_link items for the top docs.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args SearchArgs) (*mcp.CallToolResult, searchResult, error) {
		out := searchResult{Hits: []searchHit{}, Next: []nextHop{}}
		results, err := engine.Search(args.Query, 10)
		if err != nil {
			return errResult("search failed: " + err.Error()), out, nil
		}
		if len(results) == 0 {
			return textResult("No results found for: " + args.Query), out, nil
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
			out.Hits = append(out.Hits, searchHit{Path: r.DocName, Section: r.Section, Score: r.Score, Snippet: r.Snippet})
		}

		// next: natural follow-up hops after a kb_search. The top hit is the
		// obvious drill-down target (scope its sections, then fetch); when the
		// query reads like a single code identifier the user is likely hunting a
		// symbol, so code_graph on that symbol is the next hop.
		top := results[0].DocName
		out.Next = append(out.Next,
			nextHop{Tool: "kb_sections", Args: map[string]string{"name": top}, Why: "list the top hit's sections before fetching"},
			nextHop{Tool: "kb_get", Args: map[string]string{"name": top}, Why: "fetch the top-matching document"},
		)
		if isIdentifierLike(args.Query) {
			out.Next = append(out.Next, nextHop{Tool: "code_graph", Args: map[string]string{"symbol": args.Query}, Why: "the query looks like a code symbol — navigate its call/impl graph"})
		}

		// resource_links let the model pull a full doc on demand (kb://<doc>)
		// instead of dumping every match. One link per distinct top doc.
		content := []mcp.Content{&mcp.TextContent{Text: b.String()}}
		seen := map[string]bool{}
		meta := engine.DocMetadata()
		for _, r := range results {
			if seen[r.DocName] || len(seen) >= 3 {
				continue
			}
			seen[r.DocName] = true
			content = append(content, &mcp.ResourceLink{
				URI:         "kb://" + r.DocName,
				Name:        r.DocName,
				Description: meta[r.DocName].Description,
				MIMEType:    "text/markdown",
			})
		}
		return &mcp.CallToolResult{Content: content}, out, nil
	})

	type SectionsArgs struct {
		Name string `json:"name" jsonschema:"Document name (e.g. ooo/package). Use kb_list to find valid names."`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "kb_sections",
		Description: "List the h2 sections available in a document. Use before kb_get with section= to retrieve only the relevant part of large documents instead of the full content.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)},
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

	// Register each KB doc as an addressable resource kb://<docname>, resolvable
	// via resources/read. The doc set is fixed at startup (embedded), so static
	// per-doc resources are cleaner than a URI template: they also enumerate in
	// resources/list, and kb_search's resource_link items point straight at them
	// so the model can pull full text on demand instead of dumping every doc.
	for name, meta := range engine.DocMetadata() {
		docName := name // capture
		uri := "kb://" + docName
		server.AddResource(&mcp.Resource{
			URI:         uri,
			Name:        docName,
			Description: meta.Description,
			MIMEType:    "text/markdown",
		}, func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			content, err := engine.GetDoc(docName)
			if err != nil {
				return nil, err
			}
			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{{
					URI:      uri,
					MIMEType: "text/markdown",
					Text:     content,
				}},
			}, nil
		})
	}

	return server
}

// searchHit is one structured retrieval hit returned by kb_search (and,
// mirrored, by skill_search) so the SDK emits an outputSchema + structuredContent.
type searchHit struct {
	Path    string  `json:"path"`
	Section string  `json:"section"`
	Score   float64 `json:"score"`
	Snippet string  `json:"snippet"`
}

// nextHop is a suggested follow-up tool call — the natural next hop after a search.
type nextHop struct {
	Tool string            `json:"tool"`
	Args map[string]string `json:"args"`
	Why  string            `json:"why"`
}

// searchResult is the typed structured result of kb_search.
type searchResult struct {
	Hits []searchHit `json:"hits"`
	Next []nextHop   `json:"next"`
}

// ptr returns a pointer to v — used for the optional *bool tool-annotation hints.
func ptr[T any](v T) *T { return &v }

// isIdentifierLike reports whether s reads like a single code identifier (no
// whitespace, valid Go-identifier characters), the signal that a kb_search query
// is really a symbol hunt and code_graph is the right next hop.
func isIdentifierLike(s string) bool {
	if s == "" || strings.ContainsAny(s, " \t\n") {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_':
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

// resolveDocName normalises a requested name into a canonical doc path.
// It handles exact matches, aliases (e.g. "plan" -> "flows/plan/plan"),
// underscore/slash-prefixed variants (e.g. "_truthseeker" -> "flows/principles/truthseeker"),
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

	// 3. Normalised form is already a canonical doc path (e.g. "flows/principles/truthseeker")
	if strings.Contains(norm, "/") {
		return norm
	}

	// 4. Fallback: return normalised form (let GetDoc report not-found)
	return norm
}

// aliasForDoc maps a canonical doc name to its short kb_get alias. The alias is
// the leaf filename — folder placement carries no command meaning beyond the
// flows/ surfacing filter (see isFlowDoc). The one special case is ooo/, whose
// leaves (package, auth, pivot…) are generic enough to collide, so they keep an
// "ooo-" prefix. Filenames under flows/ already encode their command name
// (e.g. flows/testing/testing-go-backend-mock), so no other special-casing.
func aliasForDoc(name string) string {
	leaf := name[strings.LastIndex(name, "/")+1:]
	if strings.HasPrefix(name, "ooo/") {
		return "ooo-" + leaf
	}
	return leaf
}

// upsertMCP reads a JSON file, sets .<parentKey>.detritus = {command, args:[]},
// and writes it back. Creates the file if it doesn't exist.
func upsertMCP(file, parentKey, command string) {
	upsertMCPServer(file, parentKey, "detritus", command, nil)
}

// upsertMCPServer upserts a named stdio MCP server into file under parentKey,
// preserving any other servers already registered there. args is the server's
// argv tail (nil → empty).
func upsertMCPServer(file, parentKey, name, command string, args []any) {
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
	if args == nil {
		args = []any{}
	}
	entry := map[string]any{
		"command": command,
		"args":    args,
	}
	// VS Code's native MCP host keys servers under "servers" and silently
	// skips any entry without an explicit transport "type". Without this the
	// gateway initializes but never starts the server, so its tools never
	// reach Copilot Chat. Other hosts (Cursor/Windsurf use "mcpServers") infer
	// stdio and don't need it, so only stamp it for the VS Code schema.
	if parentKey == "servers" {
		entry["type"] = "stdio"
	}
	parent[name] = entry
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
	fmt.Printf("Updated %s in %s\n", name, file)
}

// removeMCPServer deletes a named stdio MCP server from file under parentKey,
// preserving every other server. It is a no-op (and never creates the file) when
// the file is missing/empty/unparseable, the parent key is absent, or the named
// entry isn't present — so it never reformat-thrashes a config with nothing to remove.
func removeMCPServer(file, parentKey, name string) {
	raw, err := os.ReadFile(file)
	if err != nil || len(raw) == 0 {
		return
	}
	data := map[string]any{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return
	}
	parent, ok := data[parentKey].(map[string]any)
	if !ok {
		return
	}
	if _, present := parent[name]; !present {
		return
	}
	delete(parent, name)
	data[parentKey] = parent

	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to marshal JSON: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(file, append(out, '\n'), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write %s: %v\n", file, err)
		os.Exit(1)
	}
	fmt.Printf("Removed stale %s from %s\n", name, file)
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
