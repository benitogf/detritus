package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// installGHStub puts a fake `gh` on PATH that serves canned JSON fixtures keyed
// by the `gh api <path>` argument (slashes → underscores), plus an optional
// "nwo" fixture answered to `gh repo view`. Anything without a fixture exits 1,
// so an empty fixture map models a failing/absent gh. No network is touched.
func installGHStub(t *testing.T, fixtures map[string]string) {
	t.Helper()
	binDir := t.TempDir()
	script := `#!/bin/sh
if [ "$1" = "repo" ]; then
  if [ -f "$GH_STUB_DIR/nwo" ]; then cat "$GH_STUB_DIR/nwo"; exit 0; fi
  exit 1
fi
if [ "$1" != "api" ]; then exit 1; fi
key=$(printf '%s' "$2" | tr '/' '_')
if [ -f "$GH_STUB_DIR/$key" ]; then cat "$GH_STUB_DIR/$key"; exit 0; fi
exit 1
`
	if err := os.WriteFile(filepath.Join(binDir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	fixDir := t.TempDir()
	for name, content := range fixtures {
		if err := os.WriteFile(filepath.Join(fixDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("GH_STUB_DIR", fixDir)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

const (
	openPRIssue  = `{"state":"open","pull_request":{"url":"x"}}`
	openPull     = `{"state":"open","merged_at":null}`
	mergedPull   = `{"state":"closed","merged_at":"2026-01-03T00:00:00Z"}`
	closedPull   = `{"state":"closed","merged_at":null}`
	openIssue    = `{"state":"open","pull_request":null}`
	closedIssue  = `{"state":"closed","pull_request":null}`
	oneCommit    = `[{"commit":{"committer":{"date":"2026-01-02T00:00:00Z"}}}]`
	noComments   = `[]`
	lateComment  = `[{"created_at":"2026-01-05T00:00:00Z"}]`
	earlyComment = `[{"created_at":"2026-01-01T00:00:00Z"}]`
	noReviews    = `[]`
	crReview     = `[{"user":{"login":"bob"},"state":"CHANGES_REQUESTED","submitted_at":"2026-01-01T00:00:00Z"}]`
	approvedAll  = `[{"user":{"login":"bob"},"state":"APPROVED","submitted_at":"2026-01-01T00:00:00Z"}]`
	crThenOK     = `[{"user":{"login":"bob"},"state":"CHANGES_REQUESTED","submitted_at":"2026-01-01T00:00:00Z"},{"user":{"login":"bob"},"state":"COMMENTED","submitted_at":"2026-01-01T01:00:00Z"},{"user":{"login":"bob"},"state":"APPROVED","submitted_at":"2026-01-02T00:00:00Z"}]`
	crDismissed  = `[{"user":{"login":"bob"},"state":"CHANGES_REQUESTED","submitted_at":"2026-01-01T00:00:00Z"},{"user":{"login":"bob"},"state":"DISMISSED","submitted_at":"2026-01-02T00:00:00Z"}]`
)

func prFixtures(reviews, comments string) map[string]string {
	return map[string]string{
		"repos_acme_widget_issues_5":          openPRIssue,
		"repos_acme_widget_pulls_5":           openPull,
		"repos_acme_widget_pulls_5_reviews":   reviews,
		"repos_acme_widget_pulls_5_commits":   oneCommit,
		"repos_acme_widget_issues_5_comments": comments,
	}
}

// classifyLaunchInput mirrors the flows/github/gh Phase 0/1 outcome table
// against live (stubbed) gh state. The load-bearing rows: unaddressed
// CHANGES_REQUESTED forces feedback even when the text asks for a review;
// merged/closed refs error out with the verbatim state; a gh failure degrades
// to the old marker-only derivation instead of failing the launch.
func TestClassifyLaunchInput(t *testing.T) {
	prURL := "https://github.com/acme/widget/pull/5"
	cases := []struct {
		name      string
		input     string
		fixtures  map[string]string
		want      string
		wantPR    int
		ambiguous bool
		degraded  bool
		wantErr   string
	}{
		{
			name:     "changes-requested beats review-intent text",
			input:    "review " + prURL,
			fixtures: prFixtures(crReview, noComments),
			want:     "feedback", wantPR: 5,
		},
		{
			name:     "review-intent text on a clean open PR",
			input:    "review " + prURL,
			fixtures: prFixtures(approvedAll, noComments),
			want:     "review", wantPR: 5,
		},
		{
			name:     "changes-requested superseded by later approval",
			input:    "review " + prURL,
			fixtures: prFixtures(crThenOK, noComments),
			want:     "review", wantPR: 5,
		},
		{
			name:     "post-commit comments without review text",
			input:    "handle " + prURL,
			fixtures: prFixtures(noReviews, lateComment),
			want:     "feedback", wantPR: 5,
		},
		{
			name:      "pre-commit comments do not count",
			input:     "handle " + prURL,
			fixtures:  prFixtures(crDismissed, earlyComment),
			ambiguous: true, wantPR: 5,
		},
		{
			name:     "feedback-intent text on a clean open PR",
			input:    "address feedback on " + prURL,
			fixtures: prFixtures(approvedAll, noComments),
			want:     "feedback", wantPR: 5,
		},
		{
			name:      "clean open PR with no signal is ambiguous",
			input:     "handle " + prURL,
			fixtures:  prFixtures(noReviews, noComments),
			ambiguous: true, wantPR: 5,
		},
		{
			name:     "open issue is new work",
			input:    "solve https://github.com/acme/widget/issues/5",
			fixtures: map[string]string{"repos_acme_widget_issues_5": openIssue},
			want:     "pr", wantPR: 0,
		},
		{
			name:     "merged PR aborts with verbatim state",
			input:    "review " + prURL,
			fixtures: map[string]string{"repos_acme_widget_issues_5": openPRIssue, "repos_acme_widget_pulls_5": mergedPull},
			wantErr:  "merged",
		},
		{
			name:     "closed PR aborts with verbatim state",
			input:    "review " + prURL,
			fixtures: map[string]string{"repos_acme_widget_issues_5": openPRIssue, "repos_acme_widget_pulls_5": closedPull},
			wantErr:  "closed",
		},
		{
			name:     "closed issue aborts",
			input:    "solve https://github.com/acme/widget/issues/5",
			fixtures: map[string]string{"repos_acme_widget_issues_5": closedIssue},
			wantErr:  "closed",
		},
		{
			name:     "owner/repo#N shorthand",
			input:    "review acme/widget#5",
			fixtures: prFixtures(approvedAll, noComments),
			want:     "review", wantPR: 5,
		},
		{
			name:     "bare #N resolves owner/repo from the cwd repo",
			input:    "address feedback on PR #5",
			fixtures: merge(prFixtures(crReview, noComments), map[string]string{"nwo": "acme/widget\n"}),
			want:     "feedback", wantPR: 5,
		},
		{
			name:     "gh failure degrades to marker derivation",
			input:    "address feedback on PR #7",
			fixtures: map[string]string{},
			want:     "feedback", wantPR: 7, degraded: true,
		},
		{
			name:     "no reference is new work without touching gh",
			input:    "build a csv exporter",
			fixtures: map[string]string{},
			want:     "pr", wantPR: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			installGHStub(t, tc.fixtures)
			got, err := classifyLaunchInput(tc.input, t.TempDir())
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("classifyLaunchInput: %v", err)
			}
			if got.Ambiguous != tc.ambiguous {
				t.Fatalf("ambiguous = %v, want %v (%+v)", got.Ambiguous, tc.ambiguous, got)
			}
			if got.Degraded != tc.degraded {
				t.Fatalf("degraded = %v, want %v (%+v)", got.Degraded, tc.degraded, got)
			}
			if !tc.ambiguous && got.Deliver != tc.want {
				t.Fatalf("deliver = %q, want %q (%+v)", got.Deliver, tc.want, got)
			}
			if got.TargetPR != tc.wantPR {
				t.Fatalf("targetPR = %d, want %d (%+v)", got.TargetPR, tc.wantPR, got)
			}
		})
	}
}

// An objective whose leading heading names a feedback verb next to a bare PR
// number ("address … feedback on … PR #749") STILL classifies as feedback: the
// bare "#749" is gated on an intent marker, the feedback phrase supplies it, and
// the explicit feedback verb wins over the bare "review" word in the same line.
func TestClassifyLaunchInputHeadingFeedback(t *testing.T) {
	fixtures := map[string]string{
		"nwo":                                 "acme/widget\n",
		"repos_acme_widget_issues_749":        openPRIssue,
		"repos_acme_widget_pulls_749":         openPull,
		"repos_acme_widget_pulls_749_reviews": approvedAll, // clean PR: no CR forces the intent path
	}
	installGHStub(t, fixtures)
	got, err := classifyLaunchInput("# Objective: address review feedback on bulk PR #749", t.TempDir())
	if err != nil {
		t.Fatalf("classifyLaunchInput: %v", err)
	}
	if got.Deliver != "feedback" || got.TargetPR != 749 || got.Ambiguous || got.Degraded {
		t.Fatalf("got %+v, want feedback/749, not ambiguous/degraded", got)
	}
}

// A multi-paragraph plan whose prose merely mentions a bare "#97" with NO
// feedback/review verb classifies as new work (pr, targetPR 0) and never touches
// gh — the regression the old marker-only path guarded against. (Empty fixtures
// model an absent gh; a fetch would exit 1, so reaching pr/0 proves gh is untouched.)
func TestClassifyLaunchInputProseCitationIsNewWork(t *testing.T) {
	installGHStub(t, map[string]string{})
	plan := "# Implementation Plan\n\n" +
		"This continues the parser work begun in #97 and adds a CSV exporter.\n\n" +
		"## Steps\n\n1. Extract the tokenizer.\n2. Wire the exporter.\n"
	got, err := classifyLaunchInput(plan, t.TempDir())
	if err != nil {
		t.Fatalf("classifyLaunchInput: %v", err)
	}
	if got.Deliver != "pr" || got.TargetPR != 0 || got.Ambiguous || got.Degraded {
		t.Fatalf("got %+v, want pr/0, not ambiguous/degraded", got)
	}
}

func merge(a, b map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

// An ambiguous open PR triggers exactly one prompt: review and feedback answers
// launch with that mode; cancel aborts the launch with an error.
func TestResolveLaunchDeliveryAmbiguousPrompt(t *testing.T) {
	installGHStub(t, prFixtures(noReviews, noComments))
	orig := ambiguousDeliveryPrompt
	defer func() { ambiguousDeliveryPrompt = orig }()

	for _, choice := range []string{"review", "feedback"} {
		calls := 0
		ambiguousDeliveryPrompt = func(pr int) string { calls++; return choice }
		deliver, pr, _, err := resolveLaunchDelivery("handle https://github.com/acme/widget/pull/5", t.TempDir())
		if err != nil {
			t.Fatalf("resolveLaunchDelivery: %v", err)
		}
		if deliver != choice || pr != 5 || calls != 1 {
			t.Fatalf("deliver/pr/calls = %q/%d/%d, want %s/5/1", deliver, pr, calls, choice)
		}
	}

	ambiguousDeliveryPrompt = func(pr int) string { return "cancel" }
	if _, _, _, err := resolveLaunchDelivery("handle https://github.com/acme/widget/pull/5", t.TempDir()); err == nil {
		t.Fatal("cancel must abort the launch with an error")
	}
}

// deriveShortTitle yields a compact display label: leading markdown heading and
// Objective:/Goal: prefixes stripped, first line only, capped at seven words.
func TestDeriveShortTitle(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"heading stripped", "# Fix the parser\n\nlong body", "Fix the parser"},
		{"objective prefix stripped", "Objective: keep the suite green\nbody", "keep the suite green"},
		{"goal prefix stripped", "## Goal: ship the exporter", "ship the exporter"},
		{"first line only", "Add retries\nand a lot more text here", "Add retries"},
		{"seven word cap", "one two three four five six seven eight nine", "one two three four five six seven…"},
		{"leading blanks skipped", "\n\n# Plan: do the thing\nbody", "do the thing"},
		{"empty input", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := deriveShortTitle(tc.in); got != tc.want {
				t.Fatalf("deriveShortTitle(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// The quest request carries title + convergence on the wire and no autonomy
// field at all: --quest-run sends converge, --adventure-run sends perFinding.
func TestStartCandylandQuestSendsConvergenceAndTitle(t *testing.T) {
	for _, convergence := range []string{"converge", "perFinding"} {
		var raw []byte
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodPost && r.URL.Path == "/api/quests":
				defer r.Body.Close()
				raw, _ = json.Marshal(decodeBody(t, r))
				_, _ = w.Write([]byte(`{"id":"q-1"}`))
			case strings.HasSuffix(r.URL.Path, "/begin"):
				w.WriteHeader(http.StatusNoContent)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		if _, err := startCandylandQuestAt(srv.URL, "ship it", "Ship it", []string{"/repo"}, "pr", 0, convergence); err != nil {
			t.Fatalf("startCandylandQuestAt: %v", err)
		}
		srv.Close()
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		if body["convergence"] != convergence {
			t.Fatalf("convergence = %v, want %q", body["convergence"], convergence)
		}
		if body["title"] != "Ship it" {
			t.Fatalf("title = %v, want Ship it", body["title"])
		}
		if strings.Contains(strings.ToLower(string(raw)), "autonomy") {
			t.Fatalf("quest request must carry no autonomy field: %s", raw)
		}
	}
}

func decodeBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	body := map[string]any{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Errorf("decode body: %v", err)
	}
	return body
}

// The campaign request carries a title and no autonomy field.
func TestStartCandylandCampaignSendsTitleNoAutonomy(t *testing.T) {
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/campaigns":
			defer r.Body.Close()
			raw, _ = json.Marshal(decodeBody(t, r))
			_, _ = w.Write([]byte(`{"id":"c-1"}`))
		case strings.HasSuffix(r.URL.Path, "/begin"):
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	if _, err := startCandylandCampaignAt(srv.URL, "ship the program", "Ship the program", []string{"/repo"}, "pr", 0); err != nil {
		t.Fatalf("startCandylandCampaignAt: %v", err)
	}
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	if body["title"] != "Ship the program" {
		t.Fatalf("title = %v, want Ship the program", body["title"])
	}
	if strings.Contains(strings.ToLower(string(raw)), "autonomy") {
		t.Fatalf("campaign request must carry no autonomy field: %s", raw)
	}
}
