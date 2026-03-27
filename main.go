package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var version = "dev"

//go:embed docs
var docsFS embed.FS

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
		case "--help", "-h":
			fmt.Println("detritus " + version)
			fmt.Println("MCP knowledge base server (stdio transport)")
			fmt.Println("")
			fmt.Println("Usage:")
			fmt.Println("  detritus              Start MCP server (used by Windsurf)")
			fmt.Println("  detritus --version    Print version")
			fmt.Println("  detritus --list       List embedded documents (name<TAB>description)")
			fmt.Println("  detritus --help       Print this help")
			fmt.Println("")
			fmt.Println("This server communicates via stdio using the Model Context Protocol.")
			fmt.Println("It is not meant to be run interactively — Windsurf spawns it automatically.")
			return
		default:
			fmt.Fprintf(os.Stderr, "unknown flag: %s\nRun 'detritus --help' for usage.\n", os.Args[1])
			os.Exit(1)
		}
	}

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
		err := fs.WalkDir(docsFS, "docs", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			name := strings.TrimSuffix(strings.TrimPrefix(path, "docs/"), ".md")
			content, _ := fs.ReadFile(docsFS, path)
			desc := extractDescription(string(content))
			fmt.Fprintf(&b, "- **%s**: %s\n", name, desc)
			return nil
		})
		if err != nil {
			return errResult("failed to read docs: " + err.Error()), nil, nil
		}
		return textResult(b.String()), nil, nil
	})

	type GetArgs struct {
		Name string `json:"name" jsonschema:"Document name without .md extension (e.g. ooo/package, scaffold/create, plan/analyze)"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name: "kb_get",
		Description: "Get full knowledge document by name. Available documents and trigger keywords:\n" +
			"SCAFFOLD: scaffold/create (create app, new app, build app, make app, web app, new project, create project, new service, create service, " +
			"scaffold, build me, make me, full-stack, frontend, backend, react app, desktop app)\n" +
			"CORE: ooo/package (ooo.Server, filters: ReadObjectFilter/ReadListFilter/WriteFilter/AfterWriteFilter/DeleteFilter/OpenFilter/LimitFilter/NoopObjectFilter/NoopListFilter/NoopFilter, " +
			"CRUD: ooo.Get/ooo.GetList/ooo.Set/ooo.Push/ooo.Delete/ooo.Patch, meta.Object, WebSocket client.Subscribe/client.SubscribeList/SubscribeEvents/SubscribeListEvents, " +
			"custom server.Endpoint/EndpointConfig, remote io.RemoteGet/io.RemoteSet/io.RemotePush/io.RemoteDelete/io.RemotePatch/RemoteConfig, REST API, glob paths, " +
			"storage.Database, ko.EmbeddedStorage, storage.LayeredConfig, storage.NewMemoryLayer, storage.WatchStorageNoop, layered storage, NoopHook, NoopNotify, NoBroadcastKeys, Static) | " +
			"ooo/filters-internals (filter bypass, direct storage, store.Set, storage.Set, LimitFilter internals, AfterWrite, WriteFilter enforcement, filters gate writes, filters read-side)\n" +
			"HISTORY: ooo/nopog (nopog.Storage, long-term historical data, millions of records, time-range queries, GetN, GetNRange, KeysRange, analytics, logs, audit trail)\n" +
			"SYNC: ooo/pivot (clustering, distributed, AP system, multi-instance, pivot.Config, pivot.Setup, pivot.GetInstance, pivot.Key, ClusterURL, NodesKey, leader/follower, node discovery, Attach)\n" +
			"AUTH: ooo/auth (JWT, github.com/benitogf/auth, auth.New, auth.NewJwtStore, tokenAuth.Verify/Router, /register, /authorize, /verify, Audit middleware, Bearer token)\n" +
			"FRONTEND: ooo/client-js (JavaScript, React, npm, WebSocket client, ooo-client, subscribe, onmessage, publish, unpublish, JSON Patch, TypeScript, useOoo hook, useSubscribe, usePublish, HTTP fallback)\n" +
			"TESTING: testing/index (index, decision table) | testing/go-backend-async (sync.WaitGroup, deterministic, wg.Add/Wait/Done, callbacks, no sleep, no require.Eventually, no channels, flaky tests) | " +
			"testing/go-backend-mock (mocking, SendFunc, function injection, boundary, simple state toggle, connected.Store, onSend callback) | " +
			"testing/go-backend-e2e (end-to-end, lifecycle, state transitions, phase pattern, consolidated test, ordering)\n" +
			"PATTERNS: patterns/async-events (general async principles, synchronization, race conditions, event-driven, never sleep, prove don't assume, fan-out, idempotent, observability) | " +
			"patterns/go-modern (Go 1.22+/1.24+, gopls modernize -fix, for range n, any, t.Context(), b.Loop(), slices, maps, clear, cmp.Or, errors.Join) | " +
			"patterns/coding-style (naming, rename, self-documenting, extract function, readability, side effects in name, refactor) | " +
			"patterns/state-management (state mutation, wasted write, double write, single writer, consolidate, counter, increment, pending flag, deferred action, schedule) | " +
			"patterns/line-of-sight (error handling, nested if, happy path, line of sight, early return, guard clause, nesting, flat code, if err == nil, err != nil)\n" +
			"PLANNING: plan/analyze (requirements analysis, feedback, design, specification, implementation plan, insights, questions) | " +
			"plan/export (export, planning document, generate document, PDF, architecture document, design document) | " +
			"plan/diagrams (mermaid, diagram, flowchart, sequence diagram, ER diagram, state diagram, class diagram, gantt, architecture, data model, visual)\n" +
			"PRINCIPLES: meta/truthseeker (pushback, evidence, question assumptions, prove before acting, radical honesty, intellectual humility, confirmation bias)\n" +
			"META: meta/grow (learn from corrections, conversation review, missed guidance, rule violation, feedback loop, distill fixes into KB) | " +
			"meta/optimize (re-index, optimize docs, agent retrieval, detection efficiency, keyword density, anti-patterns, triggers audit) | " +
			"meta/research-first (uncertain about API, how does this work, is this true, does this work, can you verify, asking user to confirm)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args GetArgs) (*mcp.CallToolResult, any, error) {
		content, err := fs.ReadFile(docsFS, "docs/"+args.Name+".md")
		if err != nil {
			return errResult(fmt.Sprintf("Document '%s' not found. Use kb_list to see available documents.", args.Name)), nil, nil
		}
		return textResult(string(content)), nil, nil
	})

	type SearchArgs struct {
		Query string `json:"query" jsonschema:"Search term, API name, or topic to find across all documents"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "kb_search",
		Description: "Search across all ooo ecosystem knowledge base documents for a specific topic, pattern, or API name. Returns matching lines with context.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args SearchArgs) (*mcp.CallToolResult, any, error) {
		queryLower := strings.ToLower(args.Query)
		var b strings.Builder
		err := fs.WalkDir(docsFS, "docs", func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			content, _ := fs.ReadFile(docsFS, path)
			contentStr := string(content)
			if !strings.Contains(strings.ToLower(contentStr), queryLower) {
				return nil
			}
			name := strings.TrimSuffix(strings.TrimPrefix(path, "docs/"), ".md")
			lines := strings.Split(contentStr, "\n")
			var matches []string
			for i, line := range lines {
				if strings.Contains(strings.ToLower(line), queryLower) {
					matches = append(matches, fmt.Sprintf("  L%d: %s", i+1, strings.TrimSpace(line)))
				}
			}
			fmt.Fprintf(&b, "## %s (%d matches)\n", name, len(matches))
			limit := min(len(matches), 10)
			for _, m := range matches[:limit] {
				fmt.Fprintln(&b, m)
			}
			if len(matches) > 10 {
				fmt.Fprintf(&b, "  ... and %d more matches\n", len(matches)-10)
			}
			b.WriteString("\n")
			return nil
		})
		if err != nil {
			return errResult("failed to search docs: " + err.Error()), nil, nil
		}
		if b.Len() == 0 {
			return textResult("No results found for: " + args.Query), nil, nil
		}
		return textResult(b.String()), nil, nil
	})

	var resourceSummary strings.Builder
	resourceSummary.WriteString("# ooo Knowledge Base\n\n")
	resourceSummary.WriteString("Available documents and tools: kb_get(name), kb_list(), kb_search(query)\n\n")
	_ = fs.WalkDir(docsFS, "docs", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		name := strings.TrimSuffix(strings.TrimPrefix(path, "docs/"), ".md")
		content, _ := fs.ReadFile(docsFS, path)
		desc := extractDescription(string(content))
		fmt.Fprintf(&resourceSummary, "- **%s**: %s\n", name, desc)
		return nil
	})

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
