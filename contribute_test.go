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

func TestRedactSecretsMasksCredentials(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		masked []string // substrings that must NOT survive
		keep   []string // substrings that MUST survive (label preserved)
	}{
		{
			name:   "openai key",
			in:     "use key sk-abcdef012345678901234567890 for the call",
			masked: []string{"sk-abcdef012345678901234567890"},
			keep:   []string{"[REDACTED]", "for the call"},
		},
		{
			name:   "github token",
			in:     "token ghp_abcdefghijklmnopqrstuvwxyz012345 leaked",
			masked: []string{"ghp_abcdefghijklmnopqrstuvwxyz012345"},
			keep:   []string{"[REDACTED]"},
		},
		{
			name:   "bearer header",
			in:     "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.payload.sig",
			masked: []string{"eyJhbGciOiJIUzI1NiJ9.payload.sig"},
			keep:   []string{"Authorization:", "Bearer", "[REDACTED]"},
		},
		{
			name:   "password assignment",
			in:     "password: hunter2supersecret",
			masked: []string{"hunter2supersecret"},
			keep:   []string{"password:", "[REDACTED]"},
		},
		{
			name:   "api_key assignment quoted",
			in:     `api_key = "abcd1234efgh5678"`,
			masked: []string{"abcd1234efgh5678"},
			keep:   []string{"api_key", "[REDACTED]"},
		},
		{
			name:   "pem block",
			in:     "before\n-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA\n-----END RSA PRIVATE KEY-----\nafter",
			masked: []string{"MIIEpAIBAAKCAQEA"},
			keep:   []string{"before", "after", "[REDACTED]"},
		},
		{
			name:   "connection string password",
			in:     "dsn postgres://admin:SuperSecret123@db.internal:5432/app",
			masked: []string{"SuperSecret123"},
			keep:   []string{"postgres://admin:", "@db.internal", "***"},
		},
		{
			name:   "stripe sk_live key",
			in:     "stripe key sk_live_abcdef0123456789ABCDEF used in prod",
			masked: []string{"sk_live_abcdef0123456789ABCDEF"},
			keep:   []string{"[REDACTED]", "used in prod"},
		},
		{
			name:   "stripe rk_live key",
			in:     "restricted rk_live_ZYXW9876543210abcdEF here",
			masked: []string{"rk_live_ZYXW9876543210abcdEF"},
			keep:   []string{"[REDACTED]"},
		},
		{
			name:   "aws secret access key space and equals",
			in:     "aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY next",
			masked: []string{"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"},
			keep:   []string{"aws_secret_access_key", "[REDACTED]", "next"},
		},
		{
			name:   "bare jwt",
			in:     "saw eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0In0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U in logs",
			masked: []string{"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9", "dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"},
			keep:   []string{"saw", "in logs", "[REDACTED]"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := redactSecrets(tc.in)
			for _, m := range tc.masked {
				if strings.Contains(got, m) {
					t.Errorf("secret %q survived redaction: %q", m, got)
				}
			}
			for _, k := range tc.keep {
				if !strings.Contains(got, k) {
					t.Errorf("expected %q to remain, got %q", k, got)
				}
			}
		})
	}
}

func TestRedactSecretsLeavesBenignStringsUntouched(t *testing.T) {
	benign := []string{
		"see https://host.example.com/path?q=1 for details", // plain URL, no creds
		"trace id 550e8400-e29b-41d4-a716-446655440000",     // UUID
		"pin to v3.33.0 before upgrading",                   // version
		"the password rotates weekly, no value here",        // prose, no credential value
		"publishable key pk_live_abcdef0123456789ABCDEF",    // Stripe publishable key must NOT be masked
	}
	for _, s := range benign {
		if got := redactSecrets(s); got != s {
			t.Errorf("benign string was modified.\n--- want ---\n%s\n--- got ---\n%s", s, got)
		}
	}
}

func TestRedactSecretsLeavesCleanLessonUnchanged(t *testing.T) {
	clean := `---
id: retry-with-backoff
kind: procedure
status: active
confirmed: 3
---
# retry-with-backoff
- On a transient error, retry with exponential backoff and jitter.
- Cap total attempts at 5; surface the last error verbatim.
- Use context.WithTimeout to bound the whole operation.
`
	if got := redactSecrets(clean); got != clean {
		t.Errorf("clean lesson was modified.\n--- want ---\n%s\n--- got ---\n%s", clean, got)
	}
}

func TestGatherLessonDocsShipsAllAndRedacts(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())
	// A stale, unconfirmed lesson still ships (no eligibility filter).
	writeLesson(t, "stale-one", "---\nid: stale-one\nstatus: stale\nconfirmed: 0\n---\n# stale-one\n- some advice\n")
	writeLesson(t, "with-secret", "---\nid: with-secret\nstatus: active\n---\n# with-secret\n- set password: topsecretvalue123 in the config\n")

	docs, err := gatherLessonDocs("lessons")
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
	if strings.Contains(docs[1].Content, "topsecretvalue123") {
		t.Errorf("secret survived in shipped doc: %q", docs[1].Content)
	}
	if !strings.Contains(docs[1].Content, "[REDACTED]") {
		t.Errorf("expected redaction marker in shipped doc: %q", docs[1].Content)
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

	plan, err := buildContributionPlan(context.Background(), contributeOptions{Repo: "acme/kb", Dir: "lessons", DryRun: true}, t.TempDir())
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
	opts, err := parseContributeArgs([]string{"--repo", "o/n", "--dir", "kb", "--dry-run"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Repo != "o/n" || opts.Dir != "kb" || !opts.DryRun {
		t.Fatalf("bad parse: %+v", opts)
	}
	if _, err := parseContributeArgs([]string{"--bogus"}); err == nil {
		t.Fatal("expected error on unknown flag")
	}
	if _, err := parseContributeArgs([]string{"--repo"}); err == nil {
		t.Fatal("expected error on --repo without value")
	}
	// Default dir.
	def, _ := parseContributeArgs(nil)
	if def.Dir != "lessons" {
		t.Fatalf("default dir should be lessons, got %q", def.Dir)
	}
}
