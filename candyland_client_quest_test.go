package main

import (
	"strings"
	"testing"
)

// TestQuestLaunchSummary verifies the rendered launch output carries the pieces
// an operator needs: the deliver mode + its honest effect line, the API URL on
// :8888, the UI URL on :8080, and the remote/WSL forward-BOTH-ports hint.
func TestQuestLaunchSummary(t *testing.T) {
	out := questLaunchSummary("q-123", "pr", 0, false)

	wants := []string{
		"candyland quest started: q-123",
		"Deliver: pr (opens a PR, never merges)",
		":8888",
		":8080",
		"forward BOTH ports",
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("summary missing %q\n---\n%s", w, out)
		}
	}
}

// TestQuestLaunchSummaryFeedbackEffect verifies the deliver-mode effect line
// reflects the feedback mode (updates the referenced PR in place, no new PR).
func TestQuestLaunchSummaryFeedbackEffect(t *testing.T) {
	out := questLaunchSummary("q-9", "feedback", 42, false)

	if !strings.Contains(out, "candyland quest started: q-9") {
		t.Errorf("summary missing quest kind/id line\n---\n%s", out)
	}
	if !strings.Contains(out, "Deliver: feedback (updates existing PR #42 in place — no new PR, never merges)") {
		t.Errorf("summary missing feedback effect line\n---\n%s", out)
	}
}

// TestExtractPerFindingFlag verifies the --per-finding flag is pulled out of the
// trailing quest args wherever it appears, leaving the folders intact. The
// flag→convergence mapping it feeds is covered by TestConvergenceMode.
func TestExtractPerFindingFlag(t *testing.T) {
	cases := []struct {
		name        string
		args        []string
		wantFlag    bool
		wantFolders []string
	}{
		{"absent", []string{"a", "b"}, false, []string{"a", "b"}},
		{"leading", []string{"--per-finding", "a", "b"}, true, []string{"a", "b"}},
		{"trailing", []string{"a", "b", "--per-finding"}, true, []string{"a", "b"}},
		{"middle", []string{"a", "--per-finding", "b"}, true, []string{"a", "b"}},
		{"only-flag", []string{"--per-finding"}, true, nil},
		{"none", nil, false, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotFlag, gotFolders := extractPerFindingFlag(tc.args)
			if gotFlag != tc.wantFlag {
				t.Errorf("perFinding = %v, want %v", gotFlag, tc.wantFlag)
			}
			if strings.Join(gotFolders, ",") != strings.Join(tc.wantFolders, ",") {
				t.Errorf("folders = %v, want %v", gotFolders, tc.wantFolders)
			}
		})
	}
}

// TestConvergenceMode covers the flag→convergence mapping runQuestCmd receives:
// the --per-finding flag selects perFinding, its absence selects converge.
func TestConvergenceMode(t *testing.T) {
	if got := convergenceMode(true); got != "perFinding" {
		t.Errorf("convergenceMode(true) = %q, want %q", got, "perFinding")
	}
	if got := convergenceMode(false); got != "converge" {
		t.Errorf("convergenceMode(false) = %q, want %q", got, "converge")
	}
}
