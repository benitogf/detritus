package main

import (
	"strings"
	"testing"
)

// TestQuestLaunchSummary verifies the rendered launch output carries the pieces
// an operator needs: the deliver mode + its honest effect line, the API URL on
// :8888, the UI URL on :8080, and the remote/WSL forward-BOTH-ports hint.
func TestQuestLaunchSummary(t *testing.T) {
	out := questLaunchSummary("quest", "q-123", "pr", 0, false)

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
	out := questLaunchSummary("adventure", "a-9", "feedback", 42, false)

	if !strings.Contains(out, "candyland adventure started: a-9") {
		t.Errorf("summary missing adventure kind/id line\n---\n%s", out)
	}
	if !strings.Contains(out, "Deliver: feedback (updates existing PR #42 in place — no new PR, never merges)") {
		t.Errorf("summary missing feedback effect line\n---\n%s", out)
	}
}
