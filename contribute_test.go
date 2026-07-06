package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/benitogf/detritus/internal/memory"
)

// writeLesson drops a raw lesson markdown file under the memory lessons dir
// (DETRITUS_HOME points there for the test).
func writeLesson(t *testing.T, id, content string) {
	t.Helper()
	if err := os.MkdirAll(memory.LessonsDir(), 0o755); err != nil {
		t.Fatalf("mkdir lessons: %v", err)
	}
	if err := os.WriteFile(filepath.Join(memory.LessonsDir(), id+".md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write lesson %s: %v", id, err)
	}
}

func TestGatherLessonDocsShipsAllVerbatim(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	// A stale, unconfirmed lesson still ships (no eligibility filter).
	writeLesson(t, "stale-one", "---\nid: stale-one\nstatus: stale\nconfirmed: 0\n---\n# stale-one\n- some advice\n")
	secretContent := "---\nid: with-secret\nstatus: active\n---\n# with-secret\n- set password: topsecretvalue123 in the config\n"
	writeLesson(t, "with-secret", secretContent)

	docs, err := gatherLessonDocs(memory.LessonsDir(), "lessons")
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("want 2 lessons shipped (no filter), got %d", len(docs))
	}
	// Sorted by id: stale-one, with-secret.
	if docs[0].ID != "stale-one" || docs[1].ID != "with-secret" {
		t.Fatalf("unexpected ids/order: %q, %q", docs[0].ID, docs[1].ID)
	}
	if docs[0].RelPath != "lessons/stale-one.md" {
		t.Errorf("bad rel path: %q", docs[0].RelPath)
	}
	// Content ships AS-IS — no scrubbing, no transform. The PR review loop is
	// the filter, not this CLI.
	if docs[1].Content != secretContent {
		t.Errorf("content must ship verbatim.\n--- want ---\n%s\n--- got ---\n%s", secretContent, docs[1].Content)
	}
	// Metadata rides along as data (status preserved verbatim).
	if !strings.Contains(docs[0].Content, "status: stale") {
		t.Errorf("expected status metadata preserved: %q", docs[0].Content)
	}
}

func TestDryRunMakesNoMutationsAndListsPlan(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	writeLesson(t, "alpha", "---\nid: alpha\n---\n# alpha\n- do the thing\n")

	// Any external command in a dry run is a bug: fail loudly if the seam fires.
	orig := contribRun
	t.Cleanup(func() { contribRun = orig })
	contribRun = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		t.Fatalf("dry-run must make NO git/gh calls, but ran: %s %v", name, args)
		return nil, nil
	}

	plan, err := buildContributionPlan(context.Background(), contributeOptions{Repo: "acme/kb", Dir: "lessons", From: memory.LessonsDir(), DryRun: true}, t.TempDir())
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if plan.Repo != "acme/kb" {
		t.Errorf("repo not honored: %q", plan.Repo)
	}
	if !strings.HasPrefix(plan.Branch, "lessons-contribution-") {
		t.Errorf("unexpected branch: %q", plan.Branch)
	}
	if len(plan.Docs) != 1 || plan.Docs[0].RelPath != "lessons/alpha.md" {
		t.Fatalf("unexpected docs: %+v", plan.Docs)
	}

	var sb strings.Builder
	printContributionPlan(&sb, plan)
	out := sb.String()
	for _, want := range []string{"acme/kb", plan.Branch, "lessons/alpha.md", "PR title:", "Contribute 1 learned"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q\n%s", want, out)
		}
	}
}

// fakeCall records one contribRun invocation.
type fakeCall struct {
	dir  string
	name string
	args []string
}

func TestExecuteContributionUsesSeamAndWritesFiles(t *testing.T) {
	orig := contribRun
	t.Cleanup(func() { contribRun = orig })

	var calls []fakeCall
	var cloneDir string
	writtenFiles := map[string]string{} // captured before the executor's deferred cleanup
	contribRun = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		calls = append(calls, fakeCall{dir: dir, name: name, args: args})
		// Simulate `gh repo clone <repo> <dir>` by creating the clone dir so the
		// subsequent file writes have somewhere to land — no real network.
		if name == "gh" && len(args) >= 4 && args[0] == "repo" && args[1] == "clone" {
			cloneDir = args[3]
			if err := os.MkdirAll(cloneDir, 0o755); err != nil {
				return nil, err
			}
		}
		// `git add` runs after the files are written and before the executor
		// returns (and RemoveAll's the temp clone), so capture them here.
		if name == "git" && len(args) >= 1 && args[0] == "add" {
			for _, id := range []string{"alpha", "beta"} {
				b, err := os.ReadFile(filepath.Join(cloneDir, "lessons", id+".md"))
				if err == nil {
					writtenFiles[id] = string(b)
				}
			}
		}
		if name == "gh" && len(args) >= 2 && args[0] == "pr" && args[1] == "create" {
			return []byte("https://github.com/acme/kb/pull/42\n"), nil
		}
		return nil, nil
	}

	plan := contributionPlan{
		Repo:   "acme/kb",
		Branch: "lessons-contribution-20260706-000000",
		Dir:    "lessons",
		Docs: []contribLessonDoc{
			{ID: "alpha", RelPath: "lessons/alpha.md", Content: "# alpha\n- x\n"},
			{ID: "beta", RelPath: "lessons/beta.md", Content: "# beta\n- y\n"},
		},
		Title: "Contribute 2 learned lesson(s)",
		Body:  "body text",
	}
	if err := executeContribution(context.Background(), plan); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if cloneDir == "" {
		t.Fatal("clone was never invoked via the seam")
	}
	// Files were written into the (faked) clone with the doc content intact.
	for _, id := range []string{"alpha", "beta"} {
		if _, ok := writtenFiles[id]; !ok {
			t.Errorf("expected lesson file lessons/%s.md written into the clone", id)
		}
	}
	if got := writtenFiles["alpha"]; got != "# alpha\n- x\n" {
		t.Errorf("alpha content mismatch: %q", got)
	}

	find := func(pred func(fakeCall) bool) (fakeCall, bool) {
		for _, c := range calls {
			if pred(c) {
				return c, true
			}
		}
		return fakeCall{}, false
	}
	if _, ok := find(func(c fakeCall) bool {
		return c.name == "git" && len(c.args) >= 3 && c.args[0] == "checkout" && c.args[1] == "-b" && c.args[2] == plan.Branch
	}); !ok {
		t.Errorf("expected `git checkout -b %s`; calls=%v", plan.Branch, calls)
	}
	if _, ok := find(func(c fakeCall) bool {
		return c.name == "git" && len(c.args) >= 1 && c.args[0] == "commit"
	}); !ok {
		t.Errorf("expected a git commit; calls=%v", calls)
	}
	if _, ok := find(func(c fakeCall) bool {
		return c.name == "git" && len(c.args) >= 3 && c.args[0] == "push"
	}); !ok {
		t.Errorf("expected a git push; calls=%v", calls)
	}
	prCall, ok := find(func(c fakeCall) bool {
		return c.name == "gh" && len(c.args) >= 2 && c.args[0] == "pr" && c.args[1] == "create"
	})
	if !ok {
		t.Fatalf("expected `gh pr create`; calls=%v", calls)
	}
	joined := strings.Join(prCall.args, " ")
	for _, want := range []string{"--repo acme/kb", "--head " + plan.Branch, "--title", plan.Title} {
		if !strings.Contains(joined, want) {
			t.Errorf("gh pr create missing %q; got %q", want, joined)
		}
	}
}

func TestExecuteContributionDeletesBranchWhenPRCreateFails(t *testing.T) {
	orig := contribRun
	t.Cleanup(func() { contribRun = orig })

	var calls []fakeCall
	contribRun = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		calls = append(calls, fakeCall{dir: dir, name: name, args: args})
		if name == "gh" && len(args) >= 4 && args[0] == "repo" && args[1] == "clone" {
			if err := os.MkdirAll(args[3], 0o755); err != nil {
				return nil, err
			}
		}
		// PR creation fails after the branch has been pushed.
		if name == "gh" && len(args) >= 2 && args[0] == "pr" && args[1] == "create" {
			return []byte("HTTP 422: validation failed"), errFake
		}
		return nil, nil
	}

	plan := contributionPlan{
		Repo:   "acme/kb",
		Branch: "lessons-contribution-20260706-000000",
		Dir:    "lessons",
		Docs:   []contribLessonDoc{{ID: "alpha", RelPath: "lessons/alpha.md", Content: "# alpha\n"}},
		Title:  "Contribute 1 learned lesson(s)",
		Body:   "body",
	}
	err := executeContribution(context.Background(), plan)
	if err == nil {
		t.Fatal("expected an error when gh pr create fails")
	}
	if !strings.Contains(err.Error(), plan.Branch) {
		t.Errorf("error should name the pushed branch %q; got %q", plan.Branch, err.Error())
	}

	// The dangling remote branch must have a delete attempted.
	var deleted bool
	for _, c := range calls {
		if c.name == "git" && len(c.args) >= 3 && c.args[0] == "push" && c.args[1] == "origin" && c.args[2] == "--delete" && len(c.args) >= 4 && c.args[3] == plan.Branch {
			deleted = true
		}
	}
	if !deleted {
		t.Errorf("expected `git push origin --delete %s` after PR-create failure; calls=%v", plan.Branch, calls)
	}
}

// errFake is a sentinel used by seam fakes that need to return a non-nil error.
var errFake = fmt.Errorf("fake command failure")

// failSeam replaces contribRun with a fake that fails the test if any external
// command is attempted, and restores the original on cleanup.
func failSeam(t *testing.T) {
	t.Helper()
	orig := contribRun
	t.Cleanup(func() { contribRun = orig })
	contribRun = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		t.Fatalf("no external command should run for a rejected input, but ran: %s %v", name, args)
		return nil, nil
	}
}

func TestRunContributeRejectsUnsafeRepoAndDir(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"dir escapes clone", []string{"--dir", "../../etc"}},
		{"repo arg injection", []string{"--repo", "-foo"}},
		{"repo not owner/name", []string{"--repo", "not-a-repo"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			failSeam(t) // any seam call is a failure
			if err := runContribute(t.TempDir(), tc.args); err == nil {
				t.Fatalf("expected rejection for args %v", tc.args)
			}
		})
	}
}

func TestDryRunResolvesRepoViaGhOnly(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	writeLesson(t, "alpha", "---\nid: alpha\n---\n# alpha\n- do the thing\n")

	orig := contribRun
	t.Cleanup(func() { contribRun = orig })
	var calls []fakeCall
	contribRun = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		calls = append(calls, fakeCall{dir: dir, name: name, args: args})
		if name == "gh" && len(args) >= 2 && args[0] == "repo" && args[1] == "view" {
			return []byte("acme/kb\n"), nil
		}
		t.Fatalf("dry-run must make no mutating call, but ran: %s %v", name, args)
		return nil, nil
	}

	// --dry-run with NO --repo: repo resolution must run, nothing else.
	if err := runContribute(t.TempDir(), []string{"--dry-run"}); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected exactly one seam call (gh repo view), got %d: %v", len(calls), calls)
	}
	c := calls[0]
	if c.name != "gh" || len(c.args) < 2 || c.args[0] != "repo" || c.args[1] != "view" {
		t.Fatalf("the only seam call must be read-only `gh repo view`; got %s %v", c.name, c.args)
	}
}

func TestParseContributeArgs(t *testing.T) {
	opts, err := parseContributeArgs([]string{"--repo", "o/n", "--dir", "kb", "--from", "/tmp/staged", "--dry-run"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Repo != "o/n" || opts.Dir != "kb" || opts.From != "/tmp/staged" || !opts.DryRun {
		t.Fatalf("bad parse: %+v", opts)
	}
	if _, err := parseContributeArgs([]string{"--bogus"}); err == nil {
		t.Fatal("expected error on unknown flag")
	}
	if _, err := parseContributeArgs([]string{"--repo"}); err == nil {
		t.Fatal("expected error on --repo without value")
	}
	if _, err := parseContributeArgs([]string{"--from"}); err == nil {
		t.Fatal("expected error on --from without value")
	}
	if _, err := parseContributeArgs([]string{"--from", "-x"}); err == nil {
		t.Fatal("expected error on --from beginning with '-'")
	}
	// --from accepts an ABSOLUTE path (it is a local source dir, not an in-repo
	// path — the filepath.IsLocal guard applies only to --dir).
	if _, err := parseContributeArgs([]string{"--from", "/abs/staging/dir"}); err != nil {
		t.Fatalf("--from absolute path should be allowed: %v", err)
	}
	// Defaults: dir=lessons, from=memory lessons store.
	def, _ := parseContributeArgs(nil)
	if def.Dir != "lessons" {
		t.Fatalf("default dir should be lessons, got %q", def.Dir)
	}
	if def.From != memory.LessonsDir() {
		t.Fatalf("default from should be the memory lessons dir %q, got %q", memory.LessonsDir(), def.From)
	}
}

func TestContributeFromStagingDir(t *testing.T) {
	// --from points at an arbitrary local staging dir (where a /grow flow drops
	// generalized lessons); those files — not the memory store — are shipped.
	t.Setenv("DETRITUS_HOME", t.TempDir()) // memory store is a DIFFERENT dir, must be ignored
	writeLesson(t, "store-only", "---\nid: store-only\n---\n# store-only\n- must NOT ship\n")

	staging := t.TempDir()
	if err := os.WriteFile(filepath.Join(staging, "one.md"), []byte("# one\n- generalized principle A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "two.md"), []byte("# two\n- generalized principle B\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A non-.md file in the staging dir must be ignored.
	if err := os.WriteFile(filepath.Join(staging, "notes.txt"), []byte("ignore me\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := contribRun
	t.Cleanup(func() { contribRun = orig })
	contribRun = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		t.Fatalf("dry-run must make NO git/gh calls, but ran: %s %v", name, args)
		return nil, nil
	}

	plan, err := buildContributionPlan(context.Background(),
		contributeOptions{Repo: "acme/kb", Dir: "lessons", From: staging, DryRun: true}, t.TempDir())
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if len(plan.Docs) != 2 {
		t.Fatalf("want 2 staged lessons shipped, got %d: %+v", len(plan.Docs), plan.Docs)
	}
	// Sorted by id; the store-only lesson is absent (came from a different dir).
	if plan.Docs[0].ID != "one" || plan.Docs[1].ID != "two" {
		t.Fatalf("unexpected ids/order: %q, %q", plan.Docs[0].ID, plan.Docs[1].ID)
	}
	if plan.Docs[0].RelPath != "lessons/one.md" {
		t.Errorf("bad rel path: %q", plan.Docs[0].RelPath)
	}
	if plan.Docs[0].Content != "# one\n- generalized principle A\n" {
		t.Errorf("content mismatch: %q", plan.Docs[0].Content)
	}
	for _, d := range plan.Docs {
		if d.ID == "store-only" {
			t.Fatalf("memory-store lesson leaked into a --from contribution: %+v", plan.Docs)
		}
	}
}

func TestContributeFromNonexistentDirShipsNothing(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	// Populate the memory store to prove --from (not the store) is the source:
	// a missing --from must ship nothing regardless of what the store holds.
	writeLesson(t, "store-only", "---\nid: store-only\n---\n# store-only\n- do not ship\n")

	failSeam(t) // a zero-lessons run must make NO seam call (repo is passed in)

	missing := filepath.Join(t.TempDir(), "does-not-exist")
	if err := runContribute(t.TempDir(), []string{"--repo", "acme/kb", "--from", missing}); err != nil {
		t.Fatalf("nonexistent --from should be a no-op, got error: %v", err)
	}
}
