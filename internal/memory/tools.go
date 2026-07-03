package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/benitogf/detritus/internal/core"
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
	registerStaleness(server)
}

type stalenessArgs struct {
	Dir string `json:"dir,omitempty" jsonschema:"Directory of the Go tree to check references against (any dir under it works; resolved to its project root). Default: the current project."`
}

func registerStaleness(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "skill_staleness",
		Description: "Advisory: scan active verified lessons for code references (backticked symbols like `Foo()`/`pkg.Foo`/`T.Method`, and source-file paths like `internal/code/store.go`) that have gone DEAD against the live tree at dir. Reports only — never edits, supersedes, or deletes a lesson. Symbols on a tree that doesn't compile are reported as \"unknown\" (never dead). Use to find lessons whose code has drifted and may need re-verifying.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)},
	}, func(ctx context.Context, req *mcp.CallToolRequest, args stalenessArgs) (*mcp.CallToolResult, StalenessResult, error) {
		res := CheckStaleness(args.Dir)
		return textResult(renderStaleness(res)), res, nil
	})
}

// renderStaleness renders a staleness result as a readable text block for the
// back-compat content channel (the structured result carries the machine shape).
func renderStaleness(res StalenessResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "checked %d active lesson(s)\n", res.Checked)
	if res.Note != "" {
		fmt.Fprintf(&b, "note: %s\n", res.Note)
	}
	if len(res.Stale) == 0 {
		b.WriteString("\nno stale code references found\n")
	} else {
		fmt.Fprintf(&b, "\nstale lessons (%d):\n", len(res.Stale))
		for _, l := range res.Stale {
			fmt.Fprintf(&b, "  %s — %s\n", l.ID, l.Title)
			for _, r := range l.DeadRefs {
				fmt.Fprintf(&b, "    dead %s: %s\n", r.Kind, r.Ref)
			}
		}
	}
	if len(res.Unknown) > 0 {
		fmt.Fprintf(&b, "\nunverifiable references (%d lesson(s) — tree did not fully load):\n", len(res.Unknown))
		for _, l := range res.Unknown {
			fmt.Fprintf(&b, "  %s — %s\n", l.ID, l.Title)
			for _, r := range l.UnknownRefs {
				fmt.Fprintf(&b, "    unknown %s: %s\n", r.Kind, r.Ref)
			}
		}
	}
	if res.Truncated {
		fmt.Fprintf(&b, "\n… truncated at %d lessons (corpus is large)\n", maxLessonsScanned)
	}
	return strings.TrimSpace(b.String())
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
		// Explicit supersede runs BEFORE the write so the replaced lesson is
		// already stale when the contradiction check (P3-4) scans active
		// lessons — this is how a caller resolves a flagged conflict.
		var supMsg string
		if args.Supersedes != "" {
			if err := Supersede(args.Supersedes); err != nil {
				supMsg = fmt.Sprintf("; supersede %q skipped: %v", args.Supersedes, err)
			} else {
				supMsg = fmt.Sprintf("; superseded %q (marked stale)", args.Supersedes)
			}
		}
		id, err := Put(args.ID, args.Kind, args.Bullets, src)
		if err != nil {
			return errResult(err.Error()), nil, nil
		}
		return textResult(fmt.Sprintf("distilled lesson %q (%s)%s", id, args.Kind, supMsg)), nil, nil
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
		// P3-2: read each hit's confirmation count and re-rank so a
		// more-corroborated lesson surfaces first among comparably-relevant hits.
		confirmed := map[string]int{}
		for _, r := range results {
			if l, err := Get(r.DocName); err == nil {
				confirmed[r.DocName] = l.Confirmed
			}
		}
		results = rerankByConfirmation(results, confirmed)
		var b strings.Builder
		for _, r := range results {
			c := confirmed[r.DocName]
			fmt.Fprintf(&b, "## %s (score %.3f · confirmed %d)\n%s\n\n", r.DocName, r.Score, c, r.Snippet)
			out.Hits = append(out.Hits, searchHit{Path: r.DocName, Section: r.Section, Score: r.Score, Snippet: r.Snippet, Confirmed: c})
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
	Path      string  `json:"path"`
	Section   string  `json:"section"`
	Score     float64 `json:"score"`
	Snippet   string  `json:"snippet"`
	Confirmed int     `json:"confirmed"`
}

// confirmationBoostPerHit lifts a lesson's effective rank by this fraction per
// re-confirmation, capped at confirmationBoostCap confirmations, so corroborated
// lessons win ties among comparably-relevant hits without letting a heavily
// re-confirmed but weakly-relevant lesson swamp a strong lexical match.
const (
	confirmationBoostPerHit = 0.1
	confirmationBoostCap    = 10
)

// rerankByConfirmation stable-sorts hits by relevance score scaled by a bounded
// confirmation boost. It reorders only the already-retrieved set (relevance
// gating/MMR upstream is untouched); the displayed Score stays the normalized
// relevance so the boost is a ranking-only tiebreak.
func rerankByConfirmation(results []core.Result, confirmed map[string]int) []core.Result {
	weight := func(r core.Result) float64 {
		c := confirmed[r.DocName]
		if c > confirmationBoostCap {
			c = confirmationBoostCap
		}
		return r.Score * (1 + confirmationBoostPerHit*float64(c))
	}
	out := append([]core.Result(nil), results...)
	sort.SliceStable(out, func(i, j int) bool {
		return weight(out[i]) > weight(out[j])
	})
	return out
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
