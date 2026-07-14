package main

import "testing"

// #127: a full PR/issue URL handed as the whole objective must classify as a
// hand-off even when it carries a path/query/fragment suffix.
func TestIsLaunchHandoffDeepLinkURL(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"bare pr url", "https://github.com/o/r/pull/5", true},
		{"deep link /files", "https://github.com/o/r/pull/5/files", true},
		{"deep link ?query", "https://github.com/o/r/pull/5?tab=comments", true},
		{"deep link #fragment", "https://github.com/o/r/pull/5#issuecomment-123", true},
		{"verb phrase + deep link", "review https://github.com/o/r/pull/5/files", true},
		{"trailing period", "https://github.com/o/r/pull/5/files.", true},
		{"issue deep link", "https://github.com/o/r/issues/9#issue-1", true},
		{"prose citation", "see https://github.com/o/r/pull/5/files for the diff", false},
		{"multiline", "line one\nhttps://github.com/o/r/pull/5/files", false},
		{"paren-wrapped citation", "the fix (https://github.com/o/r/pull/5/files) landed", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ref, ok := parseLaunchReference(tc.input)
			if !ok {
				t.Fatalf("no reference parsed from %q", tc.input)
			}
			if got := isLaunchHandoff(tc.input, ref); got != tc.want {
				t.Errorf("isLaunchHandoff(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// The owner/repo/num are still extracted correctly when a suffix is present, and
// the recorded match offsets span the suffix so the hand-off check sees no
// trailing text.
func TestParseLaunchReferenceDeepLink(t *testing.T) {
	input := "https://github.com/benitogf/detritus/pull/5/files"
	ref, ok := parseLaunchReference(input)
	if !ok {
		t.Fatal("expected a reference")
	}
	if ref.owner != "benitogf" || ref.repo != "detritus" || ref.num != 5 {
		t.Errorf("parsed %q owner/repo/num = %q/%q/%d, want benitogf/detritus/5",
			input, ref.owner, ref.repo, ref.num)
	}
	if ref.start != 0 || ref.end != len(input) {
		t.Errorf("match offsets = [%d,%d), want [0,%d) covering the suffix",
			ref.start, ref.end, len(input))
	}
}
