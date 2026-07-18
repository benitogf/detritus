package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// pluginCommandsDir is the Claude/Codex plugin's slash-command directory. Its
// shims are generated from docs/flows/ so they can never drift to a deleted
// doc name. Regenerate with `detritus --plugin-commands`;
// TestPluginCommandsMatchFlows guards it.
const pluginCommandsDir = "plugins/detritus/commands"

// pluginCommandGeneratedMarker identifies a shim this generator owns, so the
// prune step never deletes a hand-authored command file.
const pluginCommandGeneratedMarker = "Call the detritus MCP tool `kb_get`"

// extractFrontmatterField returns the first `field: value` from a doc's YAML
// frontmatter, or "" if absent.
func extractFrontmatterField(content, field string) string {
	prefix := field + ":"
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
		}
	}
	return ""
}

// pluginCommandContent renders one plugin command shim for a flows/ doc. It
// carries the doc's description and (when present) its argument-hint, and
// routes to the canonical doc name via kb_get.
func pluginCommandContent(name, desc, hint string) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "description: %s\n", desc)
	if hint != "" {
		fmt.Fprintf(&b, "argument-hint: %s\n", hint)
	}
	b.WriteString("---\n\n")
	b.WriteString("The user invoked this command with: $ARGUMENTS\n\n")
	fmt.Fprintf(&b, "Call the detritus MCP tool `kb_get` with `name=\"%s\"` and follow the returned guidance.\n", name)
	return b.String()
}

// pluginCommandFiles returns the generated shim filename → content for every
// doc under docs/flows/. The single source the writer and the drift test share.
func pluginCommandFiles() map[string]string {
	out := map[string]string{}
	_ = fs.WalkDir(docsFS, "docs/flows", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		name := strings.TrimSuffix(strings.TrimPrefix(path, "docs/"), ".md")
		content, _ := fs.ReadFile(docsFS, path)
		desc := extractDescription(string(content))
		hint := extractFrontmatterField(string(content), "argument-hint")
		out[aliasForDoc(name)+".md"] = pluginCommandContent(name, desc, hint)
		return nil
	})
	return out
}

// writePluginCommands regenerates the plugin command shims from docs/flows/,
// pruning generated shims whose backing doc was removed. Invoked by
// `detritus --plugin-commands`.
func writePluginCommands(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	want := pluginCommandFiles()
	for fname, content := range want {
		path := filepath.Join(dir, fname)
		if existing, err := os.ReadFile(path); err == nil && !strings.Contains(string(existing), pluginCommandGeneratedMarker) {
			continue
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || want[e.Name()] != "" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err == nil && strings.Contains(string(data), pluginCommandGeneratedMarker) {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	return nil
}
