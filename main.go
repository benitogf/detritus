package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

//go:embed docs/*.md
var docsFS embed.FS

func main() {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "ooo-knowledge-base",
		Version: "v1.0.0",
	}, nil)

	type ListArgs struct{}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "kb_list",
		Description: "List all available knowledge base documents with descriptions",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args ListArgs) (*mcp.CallToolResult, any, error) {
		entries, err := fs.ReadDir(docsFS, "docs")
		if err != nil {
			return errResult("failed to read docs: " + err.Error()), nil, nil
		}
		var b strings.Builder
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), ".md")
			content, _ := fs.ReadFile(docsFS, "docs/"+entry.Name())
			desc := extractDescription(string(content))
			fmt.Fprintf(&b, "- **%s**: %s\n", name, desc)
		}
		return textResult(b.String()), nil, nil
	})

	type GetArgs struct {
		Name string `json:"name" jsonschema:"Document name without .md extension (e.g. ooo-package, testing-go-backend-async)"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name: "kb_get",
		Description: "Get full knowledge document by name. Available documents and trigger keywords:\n" +
			"CORE: ooo-package (ooo.Server, filters: ReadObjectFilter/ReadListFilter/WriteFilter/AfterWriteFilter/DeleteFilter/OpenFilter/LimitFilter/NoopObjectFilter/NoopListFilter/NoopFilter, " +
			"CRUD: ooo.Get/ooo.Set/ooo.Push/ooo.Delete/ooo.Patch, meta.Object, WebSocket client.Subscribe/client.SubscribeList/SubscribeEvents/SubscribeListEvents, " +
			"custom server.Endpoint/EndpointConfig, remote io.RemoteGet/io.RemoteSet/io.RemotePush/io.RemoteDelete/io.RemotePatch/RemoteConfig, REST API, glob paths, storage.Database)\n" +
			"STORAGE: ooo-ko (LevelDB, ko.Storage, ko.EmbeddedStorage, storage.LayeredConfig, storage.NewMemoryLayer, storage.WatchStorageNoop, layered storage, db path) | " +
			"ooo-nopog (PostgreSQL, nopog.Storage, GetN, GetNRange, KeysRange, millions of records, bulk data, history, SQL)\n" +
			"SYNC: ooo-pivot (clustering, distributed, AP system, multi-instance, pivot.Config, pivot.Setup, pivot.GetInstance, pivot.Key, ClusterURL, NodesKey, leader/follower, node discovery, Attach)\n" +
			"AUTH: ooo-auth (JWT, github.com/benitogf/auth, auth.New, auth.NewJwtStore, tokenAuth.Verify/Router, /register, /authorize, /verify, Audit middleware)\n" +
			"FRONTEND: ooo-client-js (JavaScript, React, npm, WebSocket client, ooo-client, subscribe, onmessage, publish, unpublish, JSON Patch, TypeScript, useOoo hook, HTTP fallback)\n" +
			"TESTING: testing (index, decision table) | testing-go-backend-async (sync.WaitGroup, deterministic, wg.Add/Wait/Done, callbacks, no sleep, no require.Eventually, no channels, flaky tests) | " +
			"testing-go-backend-mock (mocking, SendFunc, function injection, boundary, simple state toggle, connected.Store, onSend callback) | " +
			"testing-go-backend-e2e (end-to-end, lifecycle, state transitions, phase pattern, consolidated test, ordering)\n" +
			"GO: go-modern (Go 1.22+/1.24+, gopls modernize -fix, for range n, any, t.Context(), b.Loop(), slices, maps, clear, cmp.Or, errors.Join)\n" +
			"PRINCIPLES: _truthseeker (pushback, evidence, question assumptions, prove before acting, radical honesty, intellectual humility, confirmation bias)\n" +
			"WORKFLOW: plan (requirements analysis, feedback, design, specification, implementation plan, insights, questions) | " +
			"scaffold-simple-service (new service, create service, scaffold, ooo+ko template, dockerfile, router, startup)",
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
		entries, err := fs.ReadDir(docsFS, "docs")
		if err != nil {
			return errResult("failed to read docs: " + err.Error()), nil, nil
		}
		var b strings.Builder
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			content, _ := fs.ReadFile(docsFS, "docs/"+entry.Name())
			contentStr := string(content)
			if !strings.Contains(strings.ToLower(contentStr), queryLower) {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), ".md")
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
		}
		if b.Len() == 0 {
			return textResult("No results found for: " + args.Query), nil, nil
		}
		return textResult(b.String()), nil, nil
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
