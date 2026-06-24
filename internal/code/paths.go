package code

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var dataDirWarnOnce sync.Once

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
	home, err := os.UserHomeDir()
	if err != nil {
		fallback := filepath.Join(os.TempDir(), "detritus")
		dataDirWarnOnce.Do(func() {
			fmt.Fprintf(os.Stderr, "detritus: could not resolve home directory (%v); falling back to %s. Set DETRITUS_HOME to configure.\n", err, fallback)
		})
		return fallback
	}
	return filepath.Join(home, ".detritus")
}
