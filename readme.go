package main

import (
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
)

// README command-table markers. The block between them is generated from
// docs/flows/ so the documented command surface can never drift from what
// ships. Regenerate with `detritus --readme`; TestReadmeCommandsMatchFlows
// guards it.
const (
	readmeCommandsStart = "<!-- COMMANDS:START -->"
	readmeCommandsEnd   = "<!-- COMMANDS:END -->"
)

// commandCategories maps each docs/flows/ subfolder to its README section
// title, in display order. The subfolder IS the category — adding a doc under
// flows/<dir> lists it under the matching title with no other bookkeeping.
var commandCategories = []struct{ dir, title string }{
	{"plan", "Plan"},
	{"build", "Build & maintain"},
	{"github", "GitHub"},
	{"project", "Project"},
	{"testing", "Testing"},
	{"principles", "Principles & style"},
	{"maintainer", "Maintainer"},
}

// firstSentence trims a frontmatter description to its first sentence so README
// table cells stay scannable.
func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, ". "); i >= 0 {
		return s[:i+1]
	}
	return s
}

// readmeCommandsSection renders the generated command table from docs/flows/,
// grouped by category subfolder. It is the single source the README block and
// the drift test both derive from.
func readmeCommandsSection() string {
	type cmd struct{ alias, desc string }
	byDir := map[string][]cmd{}
	_ = fs.WalkDir(docsFS, "docs/flows", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		name := strings.TrimSuffix(strings.TrimPrefix(path, "docs/"), ".md")
		dir := strings.SplitN(strings.TrimPrefix(name, "flows/"), "/", 2)[0]
		content, _ := fs.ReadFile(docsFS, path)
		byDir[dir] = append(byDir[dir], cmd{aliasForDoc(name), firstSentence(extractDescription(string(content)))})
		return nil
	})

	// Append any flows/ subfolder not in commandCategories (sorted) so a new
	// category can't be silently dropped from the README — both this and
	// TestReadmeCommandsMatchFlows derive from here, so the test alone can't
	// catch it. Mirrors the categoryOrder hardening in cmd/generate.
	cats := append([]struct{ dir, title string }{}, commandCategories...)
	known := map[string]bool{}
	for _, c := range commandCategories {
		known[c.dir] = true
	}
	var extra []string
	for dir := range byDir {
		if !known[dir] {
			extra = append(extra, dir)
		}
	}
	sort.Strings(extra)
	for _, dir := range extra {
		cats = append(cats, struct{ dir, title string }{dir, strings.ToUpper(dir[:1]) + dir[1:]})
	}

	var b strings.Builder
	b.WriteString(readmeCommandsStart + "\n")
	for _, cat := range cats {
		cmds := byDir[cat.dir]
		if len(cmds) == 0 {
			continue
		}
		sort.Slice(cmds, func(i, j int) bool { return cmds[i].alias < cmds[j].alias })
		fmt.Fprintf(&b, "\n### %s\n\n| Command | Use it for |\n| --- | --- |\n", cat.title)
		for _, c := range cmds {
			fmt.Fprintf(&b, "| `/%s` | %s |\n", c.alias, c.desc)
		}
	}
	b.WriteString("\n" + readmeCommandsEnd)
	return b.String()
}

// writeReadmeCommands rewrites the marked command block in README.md from
// docs/flows/. Invoked by `detritus --readme`.
func writeReadmeCommands(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text := string(raw)
	si := strings.Index(text, readmeCommandsStart)
	ei := strings.Index(text, readmeCommandsEnd)
	if si < 0 || ei < 0 || ei < si {
		return fmt.Errorf("README.md missing %s/%s markers", readmeCommandsStart, readmeCommandsEnd)
	}
	updated := text[:si] + readmeCommandsSection() + text[ei+len(readmeCommandsEnd):]
	if updated == text {
		return nil
	}
	return os.WriteFile(path, []byte(updated), 0o644)
}
