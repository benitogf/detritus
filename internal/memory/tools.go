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
	ID         string   `json:"id" jsonschema:"Stable kebab-case lesson id. An existing id appends bullets; a new id creates a lesson."`
	Kind       string   `json:"kind" jsonschema:"\"procedure\" (a reusable how-to) or \"fact\" (a durable fact)."`
	Bullets    []string `json:"bullets" jsonschema:"Itemized delta — the strategy/concept/failure-mode bullets to append. Never a wholesale rewrite."`
	Run        string   `json:"run,omitempty" jsonschema:"Provenance: the run id that produced this lesson."`
	Task       string   `json:"task,omitempty" jsonschema:"Provenance: the task id."`
	Outcome    string   `json:"outcome" jsonschema:"Verification outcome. Must be \"green\" — only verified-green work distils; anything else is rejected."`
	Supersedes string   `json:"supersedes,omitempty" jsonschema:"Optional: id of a lesson this one contradicts/replaces. It is marked stale (kept for audit), not deleted."`
}

func registerPut(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "skill_put",
		Description: "Distil a verified lesson into long-term memory: append an itemized delta (bullets) to a lesson file under ~/.detritus/memory. The ONLY write path, and gated — it rejects anything whose outcome is not \"green\". Call it at a verified-green milestone with a reusable, cross-project lesson; never for unverified work or repo-specific facts (those go to MEMORY.md).",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, IdempotentHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args putArgs) (*mcp.CallToolResult, any, error) {
		src := Source{Run: args.Run, Task: args.Task, Outcome: args.Outcome, TS: time.Now().UTC().Format(time.RFC3339)}
		id, err := Put(args.ID, args.Kind, args.Bullets, src)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		msg := fmt.Sprintf("distilled lesson %q (%s)", id, args.Kind)
		if args.Supersedes != "" {
			if err := Supersede(args.Supersedes); err != nil {
				msg += fmt.Sprintf("; supersede %q skipped: %v", args.Supersedes, err)
			} else {
				msg += fmt.Sprintf("; superseded %q (marked stale)", args.Supersedes)
			}
		}
		return textResult(msg), nil, nil
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
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args searchArgs) (*mcp.CallToolResult, searchResult, error) {
		out := searchResult{Hits: []searchHit{}, Next: []nextHop{}}
		limit := args.Limit
		if limit <= 0 {
			limit = 5
		}
		results, err := SearchAndLog(args.Query, limit)
		if err != nil {
			return errResult("skill_search: " + err.Error()), out, nil
		}
		if len(results) == 0 {
			return textResult("(no relevant lessons in memory)"), out, nil
		}
		var b strings.Builder
		for _, r := range results {
			fmt.Fprintf(&b, "## %s (score %.3f)\n%s\n\n", r.DocName, r.Score, r.Snippet)
			out.Hits = append(out.Hits, searchHit{Path: r.DocName, Section: r.Section, Score: r.Score, Snippet: r.Snippet})
		}
		// next: fetch the top lesson in full — the natural hop after a skill_search.
		out.Next = append(out.Next, nextHop{
			Tool: "skill_get",
			Args: map[string]string{"id": results[0].DocName},
			Why:  "read the top-matching lesson in full",
		})
		return textResult(strings.TrimSpace(b.String())), out, nil
	})
}

type getArgs struct {
	ID string `json:"id" jsonschema:"Lesson id (from skill_search)."`
}

func registerGet(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "skill_get",
		Description: "Fetch one verified lesson by id (its full bullets). Use after skill_search to read a candidate in full. Lessons are retrieved context to apply with judgement — never auto-executed.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)},
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

// searchHit is one structured retrieval hit returned by skill_search, mirroring
// kb_search's shape so the SDK emits an outputSchema + structuredContent. For a
// lesson, Path is the lesson id and Section is empty.
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

// searchResult is the typed structured result of skill_search.
type searchResult struct {
	Hits []searchHit `json:"hits"`
	Next []nextHop   `json:"next"`
}

// ptr returns a pointer to v — used for the optional *bool tool-annotation hints.
func ptr[T any](v T) *T { return &v }

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

func errResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}, IsError: true}
}
