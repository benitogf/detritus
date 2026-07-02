package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/ulikunitz/xz"
)

// The /consult flow renders Typst documents and D2 diagrams, so detritus bundles
// both companion binaries beside its own during --setup (mirroring the candyland
// fetch). Releases ship as archives with the binary inside a subdirectory, so the
// extractors below match by basename rather than exact path.

// goosVar/goarchVar default to the build target and are overridable in tests so
// asset-name construction can be exercised across platforms.
var (
	goosVar   = runtime.GOOS
	goarchVar = runtime.GOARCH
)

// toolBinPath is where a companion tool binary lives on this platform, reusing
// candyland's sibling-then-~/.local/bin resolution so fetch and verification agree.
func toolBinPath(detritusPath, binName string) string {
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	dir := filepath.Dir(detritusPath)
	if !dirWritable(dir) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, ".local", "bin", binName)
		}
	}
	return filepath.Join(dir, binName)
}

// toolBinFor returns the tool binary beside detritus and whether it exists.
func toolBinFor(detritusPath, binName string) (string, bool) {
	p := toolBinPath(detritusPath, binName)
	if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
		return p, true
	}
	return "", false
}

// archiveKind selects the extractor for a downloaded asset.
type archiveKind int

const (
	archiveTarGz archiveKind = iota
	archiveTarXz
	archiveZip
)

// fetchToolBinary downloads the latest release of repo, extracts binName from the
// archive, and installs it beside detritus. Best-effort and cross-platform (pure
// Go, no shell): on any failure it logs and returns so setup proceeds. assetFor
// maps a tag to (asset name, archive kind) for this os/arch.
func fetchToolBinary(repo, label, binName string, assetFor func(tag string) (string, archiveKind), detritusPath string, dryRun bool) {
	ver, err := fetchLatestReleaseTag(repo)
	if err != nil || ver == "" {
		fmt.Printf("%s: no release found yet — skipping (detritus works without it).\n", label)
		return
	}
	asset, kind := assetFor(ver)
	url := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repo, ver, asset)

	inName := binName
	if runtime.GOOS == "windows" {
		inName += ".exe"
	}
	dest := toolBinPath(detritusPath, binName)

	if dryRun {
		fmt.Printf("[dry-run] Would download %s → %s\n", url, dest)
		return
	}

	fmt.Printf("Installing %s %s (%s/%s)...\n", label, ver, runtime.GOOS, runtime.GOARCH)
	if err := downloadAndExtract(url, kind, inName, dest); err != nil {
		fmt.Printf("%s: couldn't install %s (%v) — skipping.\n", label, asset, err)
		return
	}
	fmt.Printf("Installed %s %s to %s\n", label, ver, dest)
}

func fetchTypstBinary(detritusPath string, dryRun bool) {
	fetchToolBinary("typst/typst", "typst", "typst", typstAsset, detritusPath, dryRun)
}

func fetchD2Binary(detritusPath string, dryRun bool) {
	fetchToolBinary("terrastruct/d2", "d2", "d2", d2Asset, detritusPath, dryRun)
}

func typstAsset(string) (string, archiveKind) { return typstAssetFor(goosVar, goarchVar) }

// typstAssetFor names the Typst release asset for an os/arch. Typst uses Rust
// target triples (amd64→x86_64, arm64→aarch64): Windows ships a .zip, Linux a
// .tar.xz.
func typstAssetFor(goos, goarch string) (string, archiveKind) {
	triple := "x86_64"
	if goarch == "arm64" {
		triple = "aarch64"
	}
	if goos == "windows" {
		return fmt.Sprintf("typst-%s-pc-windows-msvc.zip", triple), archiveZip
	}
	return fmt.Sprintf("typst-%s-unknown-linux-musl.tar.xz", triple), archiveTarXz
}

func d2Asset(tag string) (string, archiveKind) { return d2AssetFor(tag, goosVar, goarchVar) }

// d2AssetFor names the D2 release asset for an os/arch. D2 embeds the tag in the
// asset name and uses GOOS/GOARCH directly; every platform ships a .tar.gz.
func d2AssetFor(tag, goos, goarch string) (string, archiveKind) {
	return fmt.Sprintf("d2-%s-%s-%s.tar.gz", tag, goos, goarch), archiveTarGz
}

// downloadAndExtract fetches an archive, extracts the entry whose basename equals
// binName, and installs it at dest (0755) via temp-write + atomic rename, mirroring
// downloadRawBinary. The extracted binary is written into dest's directory so the
// final rename is a same-filesystem atomic swap that never leaves a truncated
// binary; the archive itself goes to the OS temp dir. On Windows a running tool may
// hold dest open; the rename then fails and the existing binary is kept.
func downloadAndExtract(url string, kind archiveKind, binName, dest string) error {
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, url)
	}

	archive, err := os.CreateTemp("", "detritus-tool-*.archive")
	if err != nil {
		return err
	}
	archivePath := archive.Name()
	defer os.Remove(archivePath)
	if _, err := io.Copy(archive, resp.Body); err != nil {
		archive.Close()
		return err
	}
	archive.Close()

	destDir := filepath.Dir(dest)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}

	var binPath string
	switch kind {
	case archiveZip:
		binPath, err = extractByBaseFromZipFile(archivePath, binName, destDir)
	case archiveTarXz:
		binPath, err = extractByBaseFromTarXzFile(archivePath, binName, destDir)
	default:
		binPath, err = extractByBaseFromTarGzFile(archivePath, binName, destDir)
	}
	if err != nil {
		return err
	}

	if err := os.Chmod(binPath, 0o755); err != nil {
		os.Remove(binPath)
		return err
	}
	if runtime.GOOS == "windows" {
		os.Remove(dest) // best-effort; if locked the rename below fails and is reported
	}
	if err := os.Rename(binPath, dest); err != nil {
		os.Remove(binPath)
		return err
	}
	return nil
}

// extractTarByBase copies the first tar entry whose basename equals binName to a
// temp file in destDir (same filesystem as the eventual dest) and returns its path.
// Shared by the gz and xz paths.
func extractTarByBase(r io.Reader, binName, destDir string) (string, error) {
	out, err := os.CreateTemp(destDir, ".detritus-tool-bin-*")
	if err != nil {
		return "", err
	}
	outPath := out.Name()

	tr := tar.NewReader(r)
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
		if hdr.Typeflag != tar.TypeReg {
			continue
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

func extractByBaseFromTarGzFile(archivePath, binName, destDir string) (string, error) {
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
	return extractTarByBase(gz, binName, destDir)
}

// extractByBaseFromTarXzFile handles Typst's Linux .tar.xz. Pure-Go xz
// (ulikunitz/xz) keeps the installer self-contained, no system tar dependency.
func extractByBaseFromTarXzFile(archivePath, binName, destDir string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	xr, err := xz.NewReader(f)
	if err != nil {
		return "", err
	}
	return extractTarByBase(xr, binName, destDir)
}

func extractByBaseFromZipFile(archivePath, binName, destDir string) (string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer r.Close()

	out, err := os.CreateTemp(destDir, ".detritus-tool-bin-*")
	if err != nil {
		return "", err
	}
	outPath := out.Name()

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
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
