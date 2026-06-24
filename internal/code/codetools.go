package code

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterSeamlessTools registers the live, zero-setup code-context tools:
// code_map (PageRank-ranked structural overview) and code_outline (raw
// signatures). Neither needs a pack or a stored index — both read live from
// disk via the mtime-keyed parse cache, so they are always fresh and work in
// non-git and not-yet-initialized folders.
func RegisterSeamlessTools(server *mcp.Server) {
	registerMap(server)
	registerSeamlessOutline(server)
}

type mapToolArgs struct {
	Scope  string   `json:"scope,omitempty" jsonschema:"Directory to map. Default: the caller's current project (nearest go.mod/.git/marker). Pass a subdir for a narrower map or a workspace root for a cross-repo map."`
	Focus  []string `json:"focus,omitempty" jsonschema:"Identifiers and/or repo-relative path fragments to bias the ranking toward."`
	Budget int      `json:"budget,omitempty" jsonschema:"Approximate token budget for the map (default 1024)."`
}

func registerMap(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "code_map",
		Description: "Ranked, token-budgeted structural map of a Go project: the most-referenced files first, each with its top-level signatures. PageRank over the symbol reference graph picks what matters; `focus` biases toward named identifiers/paths. No setup — start here to orient in an unfamiliar area before reading files.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args mapToolArgs) (*mcp.CallToolResult, any, error) {
		out, err := BuildCodeMap(MapOptions{Scope: args.Scope, Focus: args.Focus, Budget: args.Budget})
		if err != nil {
			return codeErrResult("code_map: " + err.Error()), nil, nil
		}
		if out == "" {
			return codeTextResult("(no Go files found in scope)"), nil, nil
		}
		return codeTextResult(out), nil, nil
	})
}

type outlineToolArgs struct {
	Path string `json:"path" jsonschema:"File or directory path (absolute or relative to the current directory). A directory outlines every source file under it."`
}

func registerSeamlessOutline(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "code_outline",
		Description: "Signature-only view of a file or directory: package, imports, types, and function signatures (go/ast for Go; regex for other languages). Much cheaper than reading full files when you only need a file's shape.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args outlineToolArgs) (*mcp.CallToolResult, any, error) {
		if args.Path == "" {
			return codeErrResult("path required"), nil, nil
		}
		out, err := OutlinePath(args.Path)
		if err != nil {
			return codeErrResult("code_outline: " + err.Error()), nil, nil
		}
		if out == "" {
			return codeTextResult("(no outline available; the path may hold no recognized source)"), nil, nil
		}
		return codeTextResult(out), nil, nil
	})
}
