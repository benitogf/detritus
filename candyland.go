package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// candyland is the sidecar app a prompt triggers (via its control MCP). detritus
// installs its binary beside its own and registers the MCP — all in this Go
// setup path (no per-platform install scripts to drift). The binary fetch mirrors
// the self-update in update.go; candyland releases are RAW per-platform binaries
// (candyland-<goos>-<goarch>[.exe]), so there is no archive to extract.

// candylandBinPath is where the candyland binary lives on this platform, whether
// or not it exists yet. Preferred location is beside detritus; but when that dir
// isn't writable by the current user (e.g. a sudo install to /usr/local/bin while
// --setup runs as the user), it falls back to ~/.local/bin so the fetch and the
// MCP registration — which use the absolute path — still work without sudo. The
// result is deterministic for a given environment, so fetch and registration
// always agree.
func candylandBinPath(detritusPath string) string {
	name := "candyland"
	if runtime.GOOS == "windows" {
		name = "candyland.exe"
	}
	dir := filepath.Dir(detritusPath)
	if !dirWritable(dir) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, ".local", "bin", name)
		}
	}
	return filepath.Join(dir, name)
}

// dirWritable reports whether the current user can create files in dir.
func dirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, ".w-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	_ = os.Remove(name)
	return true
}

// candylandBinFor returns the candyland binary beside detritus and whether it
// exists. When absent (no candyland release yet, or the fetch was skipped) the
// MCP registration is skipped rather than pointing a host at a missing binary.
func candylandBinFor(detritusPath string) (string, bool) {
	p := candylandBinPath(detritusPath)
	if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
		return p, true
	}
	return "", false
}

// fetchCandylandBinary downloads the latest candyland release binary for this
// platform and installs it beside detritus, so the MCP registration below can
// wire it up. Best-effort and cross-platform (pure Go, no shell): on any
// failure — no release yet, no asset for this os/arch, network error, a locked
// existing binary on Windows — it logs and returns so setup proceeds; detritus
// works without the sidecar. Run before the host registrations.
func fetchCandylandBinary(detritusPath string, dryRun bool) {
	ver, err := fetchLatestReleaseTag("benitogf/candyland")
	if err != nil || ver == "" {
		fmt.Println("candyland: no release found yet — skipping the sidecar binary (detritus works without it).")
		return
	}
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	asset := fmt.Sprintf("candyland-%s-%s%s", runtime.GOOS, runtime.GOARCH, ext)
	url := fmt.Sprintf("https://github.com/benitogf/candyland/releases/download/%s/%s", ver, asset)
	dest := candylandBinPath(detritusPath)

	if dryRun {
		fmt.Printf("[dry-run] Would download %s → %s\n", url, dest)
		return
	}

	fmt.Printf("Installing candyland %s (%s/%s)...\n", ver, runtime.GOOS, runtime.GOARCH)
	if err := downloadRawBinary(url, dest); err != nil {
		fmt.Printf("candyland: couldn't install %s (%v) — skipping the sidecar.\n", asset, err)
		return
	}
	fmt.Printf("Installed candyland %s to %s\n", ver, dest)
}

// downloadRawBinary fetches a raw binary and installs it at dest (0755). It
// writes to a temp file in dest's directory first, then renames it into place
// (same-filesystem, so the swap is atomic and never leaves a half-written
// binary). On Windows a running candyland may hold dest open; the rename then
// fails and the caller keeps the existing binary.
func downloadRawBinary(url, dest string) error {
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, url)
	}

	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".candyland-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	tmp.Close()
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if runtime.GOOS == "windows" {
		os.Remove(dest) // best-effort; if locked the rename below fails and is reported
	}
	if err := os.Rename(tmpPath, dest); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}

// previewCandyland prints the dry-run preview of the candyland MCP registration
// that the real run would add for a host (dest names the file + parent key). The
// binary fetch is skipped under dry-run, so when it isn't already installed the
// line says the registration is conditional on a real run fetching it — matching
// what actually happens, rather than implying it's always wired.
func previewCandyland(dest, detritusPath string) {
	if _, ok := candylandBinFor(detritusPath); ok {
		fmt.Printf("[dry-run] Would register the candyland control-mcp in %s\n", dest)
		return
	}
	fmt.Printf("[dry-run] Would register the candyland control-mcp in %s once its binary is installed\n", dest)
}

// registerCandylandJSON upserts the candyland control-mcp beside detritus in a
// JSON MCP host config (same file + parentKey), when the candyland binary is
// installed alongside detritus. A prompt can then call launch_run; the control
// MCP brings the sidecar up on demand.
func registerCandylandJSON(file, parentKey, detritusPath string) {
	if cbin, ok := candylandBinFor(detritusPath); ok {
		upsertMCPServer(file, parentKey, "candyland", cbin, []any{"control-mcp"})
	}
}
