package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDeriveQuestAutonomy verifies the pure autonomy-derivation function: an
// execute-shaped objective yields the executing level (L2) while a report/audit
// objective stays report-only (L1). deliver:"pr" is the safety rail and is not
// affected by this classification.
func TestDeriveQuestAutonomy(t *testing.T) {
	cases := []struct {
		name      string
		objective string
		want      string
	}{
		{"fix verb", "Fix the broken login handler", "L2"},
		{"implement verb", "Implement pagination on the users list", "L2"},
		{"build verb", "Build a CSV export endpoint", "L2"},
		{"refactor verb", "Refactor the auth middleware for clarity", "L2"},
		{"wire verb", "Wire up the new metrics collector", "L2"},
		{"add verb", "Add a retry to the publisher", "L2"},
		{"update pr", "Update the PR per review feedback", "L2"},
		{"solve verb", "Solve the race in the bus filter", "L2"},
		{"report only", "Report on the test coverage gaps", "L1"},
		{"audit only", "Audit the repo for stale docs", "L1"},
		{"survey only", "Survey the dependencies for CVEs", "L1"},
		{"empty stays L1", "", "L1"},
		{"case insensitive", "FIX the thing", "L2"},
		{"verb mid-sentence", "Please go and fix the parser", "L2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := deriveQuestAutonomy(tc.objective)
			if got != tc.want {
				t.Fatalf("deriveQuestAutonomy(%q) = %q, want %q", tc.objective, got, tc.want)
			}
		})
	}
}

// TestQuestLaunchSummary verifies the enriched launch output: it names the
// autonomy and deliver mode, a what-will/won't-do line, and both the API and UI
// ports with a port-forwarding hint. For an L1 autonomy against an
// execute-shaped objective it must include a loud warning.
func TestQuestLaunchSummary(t *testing.T) {
	l2 := questLaunchSummary("q-123", "L2", "pr", 0, "Fix the parser")
	for _, want := range []string{"q-123", "L2", "pr", candylandBaseURL, candylandDashboardURL, "opens a PR", "never merges"} {
		if !strings.Contains(l2, want) {
			t.Fatalf("L2 summary missing %q in:\n%s", want, l2)
		}
	}
	if !strings.Contains(strings.ToLower(l2), "forward") {
		t.Fatalf("L2 summary missing port-forwarding hint in:\n%s", l2)
	}

	l1 := questLaunchSummary("q-9", "L1", "pr", 0, "Report on coverage")
	if !strings.Contains(l1, "no code changes") {
		t.Fatalf("L1 summary missing report-only description in:\n%s", l1)
	}
	if strings.Contains(strings.ToLower(l1), "warning") {
		t.Fatalf("L1 summary against a report objective should not warn:\n%s", l1)
	}

	// L1 against an execute-shaped objective is a misclassification: warn loudly.
	l1exec := questLaunchSummary("q-7", "L1", "pr", 0, "Fix the broken handler")
	if !strings.Contains(strings.ToLower(l1exec), "warning") {
		t.Fatalf("L1 summary against an execute-shaped objective must warn loudly:\n%s", l1exec)
	}
}

// TestDeriveQuestDelivery verifies the pure delivery-derivation function:
// feedback/fix intent on an existing PR selects ("feedback", N); review/check
// intent selects ("review", N); new work falls back to ("pr", 0). PR numbers
// parse from "#N" and "PR N"; an intent marker without a PR number degrades to
// the new-work default.
func TestDeriveQuestDelivery(t *testing.T) {
	cases := []struct {
		name       string
		objective  string
		wantMode   string
		wantTarget int
	}{
		{"feedback on PR #N", "Address feedback on PR #12", "feedback", 12},
		{"fix review comments on #N", "Fix the review comments on #34", "feedback", 34},
		{"update PR #N", "Update PR #7 per the latest comments", "feedback", 7},
		{"review PR #N", "Review PR #12 for correctness", "review", 12},
		{"check PR against reqs", "Check PR #88 against the requirements", "review", 88},
		{"review #N short form", "Please review #5", "review", 5},
		{"PR-space number form", "address feedback on pr 21", "feedback", 21},
		{"new work, no PR", "Fix the broken login handler", "pr", 0},
		{"report only, no PR", "Report on test coverage gaps", "pr", 0},
		{"feedback marker without number degrades", "Address the feedback please", "pr", 0},
		{"feedback wins over review", "Review and address the review comments on PR #9", "feedback", 9},
		{"empty", "", "pr", 0},
		{"case insensitive", "ADDRESS FEEDBACK ON PR #3", "feedback", 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotMode, gotTarget := deriveQuestDelivery(tc.objective)
			if gotMode != tc.wantMode || gotTarget != tc.wantTarget {
				t.Fatalf("deriveQuestDelivery(%q) = (%q, %d), want (%q, %d)",
					tc.objective, gotMode, gotTarget, tc.wantMode, tc.wantTarget)
			}
		})
	}
}

// TestQuestLaunchSummaryDeliveryModes verifies the launch summary states each
// delivery mode honestly: feedback updates PR #N in place with no new PR, review
// reports on PR #N and may produce no PR, and a review delivery does not trip the
// L1 execute-shaped warning. Every mode states it never merges.
func TestQuestLaunchSummaryDeliveryModes(t *testing.T) {
	fb := questLaunchSummary("q-1", "L2", "feedback", 12, "Address feedback on PR #12")
	for _, want := range []string{"feedback", "PR #12", "in place", "no new PR", "never merges"} {
		if !strings.Contains(fb, want) {
			t.Fatalf("feedback summary missing %q in:\n%s", want, fb)
		}
	}

	rv := questLaunchSummary("q-2", "L1", "review", 12, "Review PR #12")
	for _, want := range []string{"review", "PR #12", "may produce no PR", "never merges"} {
		if !strings.Contains(rv, want) {
			t.Fatalf("review summary missing %q in:\n%s", want, rv)
		}
	}
	// A review delivery is legitimately L1, so it must NOT warn even though the
	// objective contains the execute-shaped "review" marker family.
	if strings.Contains(strings.ToLower(rv), "warning") {
		t.Fatalf("review delivery at L1 should not warn:\n%s", rv)
	}
}

// TestCampaignLaunchSummaryDeliveryModes verifies the campaign launch output
// states autonomy (fixed L2) and the derived delivery mode honestly, reusing the
// shared quest mode lines: feedback updates PR #N in place with no new PR, review
// reports on PR #N, pr opens a new PR. Every mode states it never merges.
func TestCampaignLaunchSummaryDeliveryModes(t *testing.T) {
	fb := campaignLaunchSummary("c-1", "L2", "feedback", 7)
	for _, want := range []string{"c-1", "L2", "feedback", "PR #7", "in place", "no new PR", "never merges", candylandDashboardURL} {
		if !strings.Contains(fb, want) {
			t.Fatalf("feedback campaign summary missing %q in:\n%s", want, fb)
		}
	}

	pr := campaignLaunchSummary("c-2", "L2", "pr", 0)
	for _, want := range []string{"L2", "pr", "opens a PR", "never merges"} {
		if !strings.Contains(pr, want) {
			t.Fatalf("pr campaign summary missing %q in:\n%s", want, pr)
		}
	}
}

// TestStartCandylandCampaignSendsDeliver verifies the campaign request carries
// the derived deliver/targetPr on the wire (exact JSON keys, matching the quest
// contract), so candyland can propagate them. Fails-without-change: before the
// struct gained the fields the body would not include them.
func TestStartCandylandCampaignSendsDeliver(t *testing.T) {
	var got candylandCampaignRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/campaigns" && r.Method == http.MethodPost:
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Errorf("decode campaign body: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"c-42"}`))
		case strings.HasSuffix(r.URL.Path, "/begin") && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	id, err := startCandylandCampaignAt(srv.URL, "Address feedback on PR #7", []string{"."}, "L2", "feedback", 7)
	if err != nil {
		t.Fatalf("startCandylandCampaignAt: %v", err)
	}
	if id != "c-42" {
		t.Fatalf("campaign id = %q, want c-42", id)
	}
	if got.Deliver != "feedback" || got.TargetPR != 7 {
		t.Fatalf("campaign request deliver/targetPr = (%q, %d), want (feedback, 7)", got.Deliver, got.TargetPR)
	}
	if got.AutonomyLevel != "L2" {
		t.Fatalf("campaign autonomy = %q, want L2 (fixed, never derived)", got.AutonomyLevel)
	}
}

// TestCampaignDeliveryDerivedFromInput verifies the campaign launcher derives the
// delivery mode from the input via the SAME function the quest path uses
// (deriveQuestDelivery, reused not duplicated): a feedback-on-PR input selects
// feedback+target, a review-on-PR input selects review+target, plain new work
// selects pr+0.
func TestCampaignDeliveryDerivedFromInput(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		wantMode   string
		wantTarget int
	}{
		{"feedback on pr", "Address feedback on PR #21 across the services", "feedback", 21},
		{"review pr", "Review PR #5 for the new gateway", "review", 5},
		{"plain goal", "Build a billing dashboard across web and api", "pr", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotMode, gotTarget := deriveQuestDelivery(tc.input)
			if gotMode != tc.wantMode || gotTarget != tc.wantTarget {
				t.Fatalf("deriveQuestDelivery(%q) = (%q, %d), want (%q, %d)",
					tc.input, gotMode, gotTarget, tc.wantMode, tc.wantTarget)
			}
		})
	}
}
