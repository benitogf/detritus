package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterTools registers the learned-memory MCP tools: skill_put (the only
// write path, verified-gated), skill_search (ranked snippets, JIT), and
// skill_get (one lesson by id). These are thin wrappers over the store + the
// shared internal/core retrieval engine.
func RegisterTools(server *mcp.Server) {
	registerPut(server)
	registerSearch(server)
	registerGet(server)
}

type putArgs struct {
	ID      string   `json:"id" jsonschema:"Stable kebab-case lesson id. An existing id appends bullets; a new id creates a lesson."`
	Kind    string   `json:"kind" jsonschema:"\"procedure\" (a reusable how-to) or \"fact\" (a durable fact)."`
	Bullets []string `json:"bullets" jsonschema:"Itemized delta — the strategy/concept/failure-mode bullets to append. Never a wholesale rewrite."`
	Run     string   `json:"run,omitempty" jsonschema:"Provenance: the run id that produced this lesson."`
	Task    string   `json:"task,omitempty" jsonschema:"Provenance: the task id."`
	Outcome string   `json:"outcome" jsonschema:"Verification outcome. Must be \"green\" — only verified-green work distils; anything else is rejected."`
}

func registerPut(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "skill_put",
		Description: "Distil a verified lesson into long-term memory: append an itemized delta (bullets) to a lesson file under ~/.detritus/memory. The ONLY write path, and gated — it rejects anything whose outcome is not \"green\". Call it at a verified-green milestone with a reusable, cross-project lesson; never for unverified work or repo-specific facts (those go to MEMORY.md).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args putArgs) (*mcp.CallToolResult, any, error) {
		src := Source{Run: args.Run, Task: args.Task, Outcome: args.Outcome, TS: time.Now().UTC().Format(time.RFC3339)}
		id, err := Put(args.ID, args.Kind, args.Bullets, src)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		return textResult(fmt.Sprintf("distilled lesson %q (%s)", id, args.Kind)), nil, nil
	})
}

type searchArgs struct {
	Query string `json:"query" jsonschema:"Task description or keywords to retrieve relevant verified lessons for."`
	Limit int    `json:"limit,omitempty" jsonschema:"Max lessons to return (default 5)."`
}

func registerSearch(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "skill_search",
		Description: "Retrieve verified, cross-project lessons relevant to a task — ranked keys + snippets only (never the whole corpus). Call at the start of a similar task; then skill_get by id for the full lesson. Returns nothing if memory is empty or no lesson is relevant.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args searchArgs) (*mcp.CallToolResult, any, error) {
		limit := args.Limit
		if limit <= 0 {
			limit = 5
		}
		results, err := Search(args.Query, limit)
		if err != nil {
			return errResult("skill_search: " + err.Error()), nil, nil
		}
		if len(results) == 0 {
			return textResult("(no relevant lessons in memory)"), nil, nil
		}
		var b strings.Builder
		for _, r := range results {
			fmt.Fprintf(&b, "## %s (score %.3f)\n%s\n\n", r.DocName, r.Score, r.Snippet)
		}
		return textResult(strings.TrimSpace(b.String())), nil, nil
	})
}

type getArgs struct {
	ID string `json:"id" jsonschema:"Lesson id (from skill_search)."`
}

func registerGet(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "skill_get",
		Description: "Fetch one verified lesson by id (its full bullets). Use after skill_search to read a candidate in full. Lessons are retrieved context to apply with judgement — never auto-executed.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getArgs) (*mcp.CallToolResult, any, error) {
		l, err := Get(args.ID)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		_ = Touch(args.ID) // mark use: refresh last_used, reactivate if stale (best-effort)
		var b strings.Builder
		fmt.Fprintf(&b, "# %s  [%s · %s · trust=%s]\n", l.Title, l.Kind, l.Status, l.Trust)
		for _, bullet := range l.Bullets {
			fmt.Fprintf(&b, "- %s\n", bullet)
		}
		return textResult(strings.TrimSpace(b.String())), nil, nil
	})
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

func errResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}, IsError: true}
}
