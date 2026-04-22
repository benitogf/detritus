package code

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterTools registers every code_* MCP tool on server.
// reg owns any opened pack indexes and should be closed when the server shuts down.
// detritusVersion is stamped into manifests written via code_pack.
func RegisterTools(server *mcp.Server, reg *Registry, detritusVersion string) {
	registerList(server)
	registerTree(server, reg)
	registerSearch(server, reg)
	registerGet(server, reg)
	registerOutline(server, reg)
	registerPack(server, reg, detritusVersion)
}

type listArgs struct{}

func registerList(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "code_list",
		Description: "List every packed workspace with its roots, file count, and token estimate. Packs are created with code_pack or `detritus --pack`.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listArgs) (*mcp.CallToolResult, any, error) {
		manifests, err := ListManifests()
		if err != nil {
			return codeErrResult("list packs: " + err.Error()), nil, nil
		}
		if len(manifests) == 0 {
			return codeTextResult("No packs yet. Run `code_pack` or `detritus --pack` to create one."), nil, nil
		}
		var b strings.Builder
		for _, m := range manifests {
			fmt.Fprintf(&b, "- **%s** — %d files, ~%d tokens, packed %s\n",
				m.Name, m.FileCount, m.TotalTokens, m.PackedAt.Format("2006-01-02 15:04"))
			for _, r := range m.Roots {
				fmt.Fprintf(&b, "    - %s\n", r)
			}
		}
		return codeTextResult(b.String()), nil, nil
	})
}

type treeArgs struct {
	Pack string `json:"pack" jsonschema:"Pack name (from code_list)."`
	Root string `json:"root,omitempty" jsonschema:"Optional: restrict the tree to one of the pack's roots (exact absolute path match)."`
}

func registerTree(server *mcp.Server, reg *Registry) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "code_tree",
		Description: "Show the directory tree of a pack. Use before code_search to orient, or to see what's in a root.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args treeArgs) (*mcp.CallToolResult, any, error) {
		if args.Pack == "" {
			return codeErrResult("pack required"), nil, nil
		}
		idx, err := reg.Open(args.Pack)
		if err != nil {
			return codeErrResult(err.Error()), nil, nil
		}
		paths, err := allPathsInPack(idx, args.Root)
		if err != nil {
			return codeErrResult("enumerate: " + err.Error()), nil, nil
		}
		return codeTextResult(renderTree(paths)), nil, nil
	})
}

type searchArgs struct {
	Pack  string `json:"pack" jsonschema:"Pack name (from code_list)."`
	Query string `json:"query" jsonschema:"Search terms. Matches across file content and paths, ranked by relevance."`
	Root  string `json:"root,omitempty" jsonschema:"Optional: scope results to one of the pack's roots (exact absolute path match)."`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum number of results (default 10)."`
}

func registerSearch(server *mcp.Server, reg *Registry) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "code_search",
		Description: "Full-text search over a pack's indexed source files. Returns ranked hits with file path and a short snippet. Prefer this over Grep when a pack covers the workspace — results are ranked by relevance, not just matched.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args searchArgs) (*mcp.CallToolResult, any, error) {
		if args.Pack == "" {
			return codeErrResult("pack required"), nil, nil
		}
		if args.Query == "" {
			return codeErrResult("query required"), nil, nil
		}
		idx, err := reg.Open(args.Pack)
		if err != nil {
			return codeErrResult(err.Error()), nil, nil
		}
		limit := args.Limit
		if limit <= 0 {
			limit = 10
		}
		// When a root filter is set we over-fetch so post-filtering can still
		// deliver `limit` hits. Cap at 1000 to avoid runaway requests.
		fetchLimit := limit
		if args.Root != "" {
			fetchLimit = limit * 10
			if fetchLimit > 1000 {
				fetchLimit = 1000
			}
		}
		q := bleve.NewMatchQuery(args.Query)
		q.SetField("content")
		req2 := bleve.NewSearchRequestOptions(q, fetchLimit, 0, false)
		req2.Fields = []string{"root", "path_rel"}
		req2.Highlight = bleve.NewHighlight()
		req2.Highlight.AddField("content")
		res, err := idx.Search(req2)
		if err != nil {
			return codeErrResult("search: " + err.Error()), nil, nil
		}
		multi := hasMultiRoot(res)
		var b strings.Builder
		emitted := 0
		for _, hit := range res.Hits {
			if emitted >= limit {
				break
			}
			root, _ := hit.Fields["root"].(string)
			pathRel, _ := hit.Fields["path_rel"].(string)
			if args.Root != "" && root != args.Root {
				continue
			}
			display := pathRel
			if multi {
				display = root + "|" + pathRel
			}
			fmt.Fprintf(&b, "## %s (score: %.3f)\n", display, hit.Score)
			if frags, ok := hit.Fragments["content"]; ok && len(frags) > 0 {
				for _, f := range frags {
					fmt.Fprintf(&b, "%s\n", strings.TrimSpace(f))
				}
			}
			b.WriteString("\n")
			emitted++
		}
		if emitted == 0 {
			if args.Root != "" {
				return codeTextResult(fmt.Sprintf("No matches for %q under root %s", args.Query, args.Root)), nil, nil
			}
			return codeTextResult("No matches for: " + args.Query), nil, nil
		}
		return codeTextResult(b.String()), nil, nil
	})
}

type getArgs struct {
	Pack  string `json:"pack" jsonschema:"Pack name (from code_list)."`
	File  string `json:"file" jsonschema:"File path. Single-root packs accept the relative path (e.g. main.go). Multi-root packs need the full ID: <root-abs-path>|<path_rel>."`
	Range string `json:"range,omitempty" jsonschema:"Optional line range, 1-indexed inclusive, e.g. 40-120."`
}

func registerGet(server *mcp.Server, reg *Registry) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "code_get",
		Description: "Fetch a file's content from a pack. Lossless cleanup is applied (trailing whitespace stripped, runs of blank lines collapsed). Use `range` to fetch a slice instead of the whole file.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getArgs) (*mcp.CallToolResult, any, error) {
		if args.Pack == "" || args.File == "" {
			return codeErrResult("pack and file required"), nil, nil
		}
		idx, err := reg.Open(args.Pack)
		if err != nil {
			return codeErrResult(err.Error()), nil, nil
		}
		doc, err := resolveDocument(idx, args.Pack, args.File, []string{"content"})
		if err != nil {
			return codeErrResult(err.Error()), nil, nil
		}
		content, _ := doc.Fields["content"].(string)
		cleaned := CleanContent([]byte(content))
		if args.Range != "" {
			start, end, perr := parseRange(args.Range)
			if perr != nil {
				return codeErrResult("bad range: " + perr.Error()), nil, nil
			}
			cleaned = SliceLines(cleaned, start, end)
		}
		return codeTextResult(cleaned), nil, nil
	})
}

type outlineArgs struct {
	Pack string `json:"pack" jsonschema:"Pack name (from code_list)."`
	File string `json:"file" jsonschema:"File path (same format as code_get)."`
}

func registerOutline(server *mcp.Server, reg *Registry) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "code_outline",
		Description: "Return a signature-only view of a file: package, imports, types, and function signatures. Much cheaper than code_get for understanding file shape before deciding what to read in full.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args outlineArgs) (*mcp.CallToolResult, any, error) {
		if args.Pack == "" || args.File == "" {
			return codeErrResult("pack and file required"), nil, nil
		}
		idx, err := reg.Open(args.Pack)
		if err != nil {
			return codeErrResult(err.Error()), nil, nil
		}
		doc, err := resolveDocument(idx, args.Pack, args.File, []string{"content", "language"})
		if err != nil {
			return codeErrResult(err.Error()), nil, nil
		}
		content, _ := doc.Fields["content"].(string)
		language, _ := doc.Fields["language"].(string)
		out := Outline(language, []byte(content))
		if out == "" {
			return codeTextResult("(no outline available for language " + language + "; fetch with code_get for full content)"), nil, nil
		}
		return codeTextResult(out), nil, nil
	})
}

type packArgs struct {
	Name  string   `json:"name" jsonschema:"Pack name. If the pack exists it is refreshed; otherwise it is created."`
	Roots []string `json:"roots,omitempty" jsonschema:"Absolute paths of root directories to include. Required when creating a new pack."`
}

func registerPack(server *mcp.Server, reg *Registry, version string) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "code_pack",
		Description: "Create or incrementally refresh a workspace pack. On first call `roots` is required. Subsequent calls can omit roots to re-walk the existing ones; only changed files are re-read.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args packArgs) (*mcp.CallToolResult, any, error) {
		if args.Name == "" {
			return codeErrResult("name required"), nil, nil
		}
		reg.Forget(args.Name)
		stats, err := Pack(args.Name, args.Roots, Options{DetritusVersion: version})
		if err != nil {
			return codeErrResult("pack: " + err.Error()), nil, nil
		}
		return codeTextResult(formatPackStats(args.Name, stats)), nil, nil
	})
}

// --- helpers ---

func codeTextResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}
}

func codeErrResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
		IsError: true,
	}
}

func parseRange(s string) (int, int, error) {
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected start-end")
	}
	start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, err
	}
	end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, err
	}
	return start, end, nil
}

// resolveDocument finds a document in the pack matching the user-supplied file.
// Accepts either "<root>|<path_rel>" or just "<path_rel>" (for single-root packs,
// or any pack where only one root holds a file with that path).
func resolveDocument(idx bleve.Index, pack, file string, fields []string) (*bleveHit, error) {
	m, err := LoadManifest(pack)
	if err != nil {
		return nil, fmt.Errorf("load manifest: %w", err)
	}
	var candidates []string
	if strings.Contains(file, "|") {
		candidates = []string{file}
	} else {
		for _, r := range m.Roots {
			candidates = append(candidates, r+"|"+file)
		}
	}
	q := bleve.NewDocIDQuery(candidates)
	req := bleve.NewSearchRequestOptions(q, len(candidates), 0, false)
	req.Fields = fields
	res, err := idx.Search(req)
	if err != nil {
		return nil, fmt.Errorf("lookup: %w", err)
	}
	if len(res.Hits) == 0 {
		return nil, fmt.Errorf("file %q not found in pack %q", file, pack)
	}
	if len(res.Hits) > 1 {
		var paths []string
		for _, h := range res.Hits {
			paths = append(paths, h.ID)
		}
		return nil, fmt.Errorf("file %q is ambiguous in pack %q (matches: %s) — prefix with <root>|", file, pack, strings.Join(paths, ", "))
	}
	hit := res.Hits[0]
	return &bleveHit{ID: hit.ID, Fields: hit.Fields}, nil
}

type bleveHit struct {
	ID     string
	Fields map[string]any
}

func allPathsInPack(idx bleve.Index, rootFilter string) ([]string, error) {
	const pageSize = 1000
	from := 0
	var out []string
	for {
		q := bleve.NewMatchAllQuery()
		req := bleve.NewSearchRequestOptions(q, pageSize, from, false)
		req.Fields = []string{"root", "path_rel"}
		res, err := idx.Search(req)
		if err != nil {
			return nil, err
		}
		if len(res.Hits) == 0 {
			break
		}
		for _, hit := range res.Hits {
			root, _ := hit.Fields["root"].(string)
			pathRel, _ := hit.Fields["path_rel"].(string)
			if rootFilter != "" && root != rootFilter {
				continue
			}
			out = append(out, pathRel)
		}
		if len(res.Hits) < pageSize {
			break
		}
		from += pageSize
	}
	return out, nil
}

func renderTree(paths []string) string {
	sort.Strings(paths)
	var b strings.Builder
	for _, p := range paths {
		depth := strings.Count(p, "/")
		indent := strings.Repeat("  ", depth)
		fmt.Fprintf(&b, "%s%s\n", indent, lastSegment(p))
	}
	return b.String()
}

func lastSegment(p string) string {
	i := strings.LastIndex(p, "/")
	if i < 0 {
		return p
	}
	return p[i+1:]
}

func hasMultiRoot(res *bleve.SearchResult) bool {
	var first string
	for _, hit := range res.Hits {
		root, _ := hit.Fields["root"].(string)
		if first == "" {
			first = root
			continue
		}
		if root != first {
			return true
		}
	}
	return false
}

func formatPackStats(name string, s *PackStats) string {
	return fmt.Sprintf("Pack %q — %d files, ~%d tokens (%dB) — new:%d modified:%d deleted:%d unchanged:%d — %s",
		name, s.Files, s.Tokens, s.Bytes, s.New, s.Modified, s.Deleted, s.Unchanged, s.Duration.Round(1e6))
}
