package code

import (
	"os"
	"path/filepath"
)

// projectMarkers are non-Go, non-git files that mark a project root. go.mod and
// .git are handled separately (their own precedence tiers in ResolveProjectRoot).
var projectMarkers = []string{
	"package.json",   // JS / TS
	"Cargo.toml",     // Rust
	"pyproject.toml", // Python
	"setup.py",       // Python
	"pom.xml",        // Java (maven)
	"build.gradle",   // Java / Kotlin (gradle)
	"Gemfile",        // Ruby
	"composer.json",  // PHP
}

// ResolveProjectRoot walks up from dir to the nearest project boundary and
// returns its absolute path. Precedence: the nearest ancestor containing
// go.mod (the Go module root), else the nearest containing .git, else the
// nearest containing any language marker, else dir itself.
//
// Identity is directory-based, never git — so multi-repo workspaces and
// not-yet-initialized folders are first-class: a folder with no marker at all
// resolves to itself, and a non-git project with go.mod resolves to its module
// root. If dir names a file, resolution starts from its containing directory.
func ResolveProjectRoot(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	if fi, err := os.Stat(abs); err == nil && !fi.IsDir() {
		abs = filepath.Dir(abs)
	}

	var gitRoot, markerRoot string
	for d := abs; ; {
		if hasEntry(d, "go.mod") {
			return d
		}
		if gitRoot == "" && hasEntry(d, ".git") {
			gitRoot = d
		}
		if markerRoot == "" && hasAnyMarker(d) {
			markerRoot = d
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}

	if gitRoot != "" {
		return gitRoot
	}
	if markerRoot != "" {
		return markerRoot
	}
	return abs
}

// hasEntry reports whether dir contains an entry with the given name.
func hasEntry(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

// hasAnyMarker reports whether dir contains any language project marker.
func hasAnyMarker(dir string) bool {
	for _, m := range projectMarkers {
		if hasEntry(dir, m) {
			return true
		}
	}
	return false
}
