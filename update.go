package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
)

// RunUpdate downloads and installs the latest release, then runs --setup.
// If dryRun is true, it only prints what would happen.
func RunUpdate(currentBinary string, dryRun bool) error {
	fmt.Println("Checking for updates...")

	latest, err := fetchLatestVersion()
	if err != nil {
		return fmt.Errorf("check latest version: %w", err)
	}

	if version != "dev" && version == latest {
		fmt.Printf("Already up to date (%s)\n", version)
		return nil
	}

	if version == "dev" {
		fmt.Printf("Current: dev build — latest release: %s\n", latest)
	} else {
		fmt.Printf("Updating %s → %s\n", version, latest)
	}

	url := releaseDownloadURL(latest)
	if dryRun {
		fmt.Printf("[dry-run] Would download %s\n", url)
		fmt.Printf("[dry-run] Would replace %s\n", currentBinary)
		fmt.Printf("[dry-run] Would run --setup\n")
		return nil
	}

	fmt.Printf("Downloading %s...\n", url)
	newBin, err := downloadBinaryFromURL(url)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer os.Remove(newBin)

	if err := os.Chmod(newBin, 0o755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}

	if err := replaceSelf(currentBinary, newBin); err != nil {
		return fmt.Errorf("replace binary: %w (close your editor and retry)", err)
	}

	fmt.Printf("Updated to %s\n", latest)
	fmt.Println("Running --setup...")
	// Exec the NEW binary so --setup uses the new embedded docs
	// (the running process still holds the old binary's docsFS in memory).
	return execNewBinarySetup(currentBinary)
}

// execNewBinarySetup runs `<binary> --setup` using the new binary on disk.
// On Unix it replaces the current process via syscall.Exec so the user sees
// the new binary's output directly. On Windows it spawns a child and exits.
func execNewBinarySetup(binary string) error {
	if runtime.GOOS == "windows" {
		cmd := exec.Command(binary, "--setup")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		return cmd.Run()
	}
	return syscall.Exec(binary, []string{binary, "--setup"}, os.Environ())
}

type githubRelease struct {
	TagName string `json:"tag_name"`
}

func fetchLatestVersion() (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", "https://api.github.com/repos/benitogf/detritus/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}
	if rel.TagName == "" {
		return "", fmt.Errorf("no tag_name in GitHub response")
	}
	return rel.TagName, nil
}

func releaseDownloadURL(ver string) string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf(
		"https://github.com/benitogf/detritus/releases/download/%s/detritus_%s_%s.%s",
		ver, goos, goarch, ext,
	)
}

func downloadBinaryFromURL(url string) (string, error) {
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, url)
	}

	// Write archive to temp file
	archive, err := os.CreateTemp("", "detritus-update-*.archive")
	if err != nil {
		return "", err
	}
	archivePath := archive.Name()
	defer os.Remove(archivePath)

	if _, err := io.Copy(archive, resp.Body); err != nil {
		archive.Close()
		return "", err
	}
	archive.Close()

	// Extract the binary from the archive
	binName := "detritus"
	if runtime.GOOS == "windows" {
		binName = "detritus.exe"
	}
	if runtime.GOOS == "windows" {
		return extractFromZipFile(archivePath, binName)
	}
	return extractFromTarGzFile(archivePath, binName)
}

func extractFromTarGzFile(archivePath, binName string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()

	out, err := os.CreateTemp("", "detritus-new-*")
	if err != nil {
		return "", err
	}
	outPath := out.Name()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			out.Close()
			os.Remove(outPath)
			return "", err
		}
		if filepath.Base(hdr.Name) == binName {
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				os.Remove(outPath)
				return "", err
			}
			out.Close()
			return outPath, nil
		}
	}
	out.Close()
	os.Remove(outPath)
	return "", fmt.Errorf("binary %q not found in archive", binName)
}

func extractFromZipFile(archivePath, binName string) (string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer r.Close()

	out, err := os.CreateTemp("", "detritus-new-*")
	if err != nil {
		return "", err
	}
	outPath := out.Name()

	for _, f := range r.File {
		if filepath.Base(f.Name) == binName {
			rc, err := f.Open()
			if err != nil {
				out.Close()
				os.Remove(outPath)
				return "", err
			}
			_, err = io.Copy(out, rc)
			rc.Close()
			out.Close()
			if err != nil {
				os.Remove(outPath)
				return "", err
			}
			return outPath, nil
		}
	}
	out.Close()
	os.Remove(outPath)
	return "", fmt.Errorf("binary %q not found in zip", binName)
}

func replaceSelf(currentPath, newPath string) error {
	if runtime.GOOS == "windows" {
		// Windows: rename current → .old, rename new → current
		// May fail if the MCP server process has the file open.
		oldPath := currentPath + ".old"
		os.Remove(oldPath)
		if err := os.Rename(currentPath, oldPath); err != nil {
			return err
		}
		if err := os.Rename(newPath, currentPath); err != nil {
			// Restore on failure
			os.Rename(oldPath, currentPath)
			return err
		}
		os.Remove(oldPath)
		return nil
	}
	// Unix: atomic rename
	return os.Rename(newPath, currentPath)
}
