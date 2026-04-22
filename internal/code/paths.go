package code

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var validPackName = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// ValidatePackName rejects names that could escape the packs directory or
// collide with special filesystem entries.
func ValidatePackName(name string) error {
	if name == "" {
		return fmt.Errorf("pack name required")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("invalid pack name %q", name)
	}
	if !validPackName.MatchString(name) {
		return fmt.Errorf("invalid pack name %q: allowed characters are [A-Za-z0-9._-]", name)
	}
	return nil
}

// DataDir returns the detritus data directory.
// Resolution order: $DETRITUS_HOME, $XDG_DATA_HOME/detritus, ~/.detritus.
// If none of those resolve, falls back to $TMPDIR/detritus with a warning
// — silently writing to CWD would hide the problem and pollute the repo.
func DataDir() string {
	if p := os.Getenv("DETRITUS_HOME"); p != "" {
		return p
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "detritus")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".detritus")
	} else {
		fallback := filepath.Join(os.TempDir(), "detritus")
		fmt.Fprintf(os.Stderr, "detritus: could not resolve home directory (%v); falling back to %s. Set DETRITUS_HOME to configure.\n", err, fallback)
		return fallback
	}
}

// PacksDir returns the directory under which all packs live.
func PacksDir() string {
	return filepath.Join(DataDir(), "packs")
}

// PackDir returns the directory for a single named pack.
func PackDir(name string) string {
	return filepath.Join(PacksDir(), name)
}

// ManifestPath returns the path of a pack's manifest.json.
func ManifestPath(name string) string {
	return filepath.Join(PackDir(name), "manifest.json")
}

// IndexPath returns the directory of a pack's Bleve index.
func IndexPath(name string) string {
	return filepath.Join(PackDir(name), "index.bleve")
}

// EnsurePacksDir creates the packs parent dir if it doesn't exist.
func EnsurePacksDir() error {
	return os.MkdirAll(PacksDir(), 0o755)
}
