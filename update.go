package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// releaseSigningPubKeyB64 is the base64-encoded 32-byte ed25519 public key whose
// private half signs checksums.txt at release time (via cmd/sign in CI). The
// updater refuses any release whose checksums.txt is unsigned or fails this
// verification — this is deliberate downgrade protection, not verify-if-present.
const releaseSigningPubKeyB64 = "AuShDfRa7XX8gmzIMcPQFWbFECpbjyNai8lmKRsG/zU="

// sameRelease reports whether two release identifiers name the same release,
// ignoring a leading "v". Release tags are "v3.43.0" but the embedded
// main.version is "3.43.0" (goreleaser's {{.Version}} strips the "v").
func sameRelease(a, b string) bool {
	return strings.TrimPrefix(a, "v") == strings.TrimPrefix(b, "v")
}

// canonicalTag returns the v-prefixed form of a release tag. An empty tag (latest)
// is returned unchanged. Accepts both "3.43.0" and "v3.43.0".
func canonicalTag(tag string) string {
	if tag == "" || strings.HasPrefix(tag, "v") {
		return tag
	}
	return "v" + tag
}

// parseUpdateArgs parses the arguments after "--update". A self-mutating command
// must not guess intent from a typo, so unknown flags and a second positional tag
// are hard errors rather than silently ignored.
func parseUpdateArgs(args []string) (pinTag string, dryRun bool, err error) {
	for _, a := range args {
		if a == "--dry-run" {
			dryRun = true
			continue
		}
		if strings.HasPrefix(a, "-") {
			return "", false, fmt.Errorf("unknown flag %q for --update (usage: detritus --update [<tag>] [--dry-run])", a)
		}
		if pinTag != "" {
			return "", false, fmt.Errorf("multiple tags given (%q and %q)", pinTag, a)
		}
		pinTag = a
	}
	return pinTag, dryRun, nil
}

// RunUpdate downloads and installs a release, then runs --setup. When pinTag is
// empty it targets the latest release; otherwise it pins to that exact tag.
// The release archive's SHA-256 is verified against the release's checksums.txt
// before the binary is extracted and swapped in. If dryRun is true, it only
// prints what would happen.
func RunUpdate(currentBinary, pinTag string, dryRun bool) error {
	fmt.Println("Checking for updates...")

	pinTag = canonicalTag(pinTag)
	target := pinTag
	if target == "" {
		latest, err := fetchLatestVersion()
		if err != nil {
			return fmt.Errorf("check latest version: %w", err)
		}
		target = latest
	}

	if version != "dev" && sameRelease(version, target) {
		fmt.Printf("Already up to date (%s)\n", version)
		return nil
	}

	switch {
	case pinTag != "":
		fmt.Printf("Updating %s → %s (pinned)\n", version, target)
	case version == "dev":
		fmt.Printf("Current: dev build — latest release: %s\n", target)
	default:
		fmt.Printf("Updating %s → %s\n", version, target)
	}

	url := releaseDownloadURL(target)
	sumsURL := checksumsURL(target)
	if dryRun {
		fmt.Printf("[dry-run] Would download %s\n", url)
		fmt.Printf("[dry-run] Would verify signature against %s.sig\n", sumsURL)
		fmt.Printf("[dry-run] Would verify SHA-256 against %s\n", sumsURL)
		fmt.Printf("[dry-run] Would replace %s\n", currentBinary)
		fmt.Printf("[dry-run] Would run --setup\n")
		return nil
	}

	pub, err := decodeSigningPubKey()
	if err != nil {
		return fmt.Errorf("load signing key: %w", err)
	}

	fmt.Printf("Downloading %s...\n", url)
	newBin, err := downloadVerifiedBinary(url, sumsURL, archiveFileName(), pub)
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

	fmt.Printf("Updated to %s\n", target)
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
	return fetchLatestReleaseTag("benitogf/detritus")
}

// fetchLatestReleaseTag returns the latest release tag for a GitHub repo
// (owner/name). Shared by the detritus self-update and the candyland fetch.
func fetchLatestReleaseTag(repo string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", "https://api.github.com/repos/"+repo+"/releases/latest", nil)
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

// archiveFileName is the release asset name for the running platform,
// e.g. "detritus_linux_amd64.tar.gz". It is also the key used to look the
// archive up in the release's checksums.txt.
func archiveFileName() string {
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("detritus_%s_%s.%s", runtime.GOOS, runtime.GOARCH, ext)
}

func releaseDownloadURL(ver string) string {
	return fmt.Sprintf(
		"https://github.com/benitogf/detritus/releases/download/%s/%s",
		ver, archiveFileName(),
	)
}

// checksumsURL is the release's goreleaser-published checksums.txt.
func checksumsURL(ver string) string {
	return fmt.Sprintf(
		"https://github.com/benitogf/detritus/releases/download/%s/checksums.txt",
		ver,
	)
}

// decodeSigningPubKey decodes the embedded base64 ed25519 public key.
func decodeSigningPubKey() (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(releaseSigningPubKeyB64)
	if err != nil {
		return nil, fmt.Errorf("decode embedded public key: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("embedded public key is %d bytes, want %d", len(raw), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(raw), nil
}

// downloadVerifiedBinary downloads the release archive, verifies the release's
// signed checksums.txt, verifies the archive's SHA-256 against its entry, and
// only then extracts the binary. Nothing is extracted until both the signature
// and the checksum pass — the binary is never extracted or executed unverified.
func downloadVerifiedBinary(archiveURL, sumsURL, archiveName string, pub ed25519.PublicKey) (string, error) {
	client := &http.Client{Timeout: 2 * time.Minute}

	// Verify the signed checksums BEFORE downloading/extracting anything.
	sums, err := verifiedChecksums(client, sumsURL, pub)
	if err != nil {
		return "", err
	}
	want, ok := sums[archiveName]
	if !ok {
		return "", fmt.Errorf("no checksum for %q in %s (refusing to run unverified binary)", archiveName, sumsURL)
	}

	resp, err := client.Get(archiveURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, archiveURL)
	}

	// Write archive to temp file, hashing the stream as it lands.
	archive, err := os.CreateTemp("", "detritus-update-*.archive")
	if err != nil {
		return "", err
	}
	archivePath := archive.Name()
	defer os.Remove(archivePath)

	hasher := sha256.New()
	if _, err := io.Copy(archive, io.TeeReader(resp.Body, hasher)); err != nil {
		archive.Close()
		return "", err
	}
	archive.Close()
	gotSum := hex.EncodeToString(hasher.Sum(nil))

	if !strings.EqualFold(want, gotSum) {
		return "", fmt.Errorf("checksum mismatch for %s: got %s, want %s (refusing to run tampered binary)", archiveName, gotSum, want)
	}
	fmt.Printf("Verified SHA-256 %s\n", gotSum)

	// Extract the binary from the (now verified) archive.
	binName := "detritus"
	if runtime.GOOS == "windows" {
		binName = "detritus.exe"
		return extractFromZipFile(archivePath, binName)
	}
	return extractFromTarGzFile(archivePath, binName)
}

// verifiedChecksums fetches checksums.txt and its detached .sig, verifies the
// ed25519 signature against pub, and returns the parsed checksum map. A failed
// signature or a missing .sig (404) is a hard error — pre-signing releases and
// signature-stripping tamper are both refused (downgrade protection).
func verifiedChecksums(client *http.Client, sumsURL string, pub ed25519.PublicKey) (map[string]string, error) {
	sumsBytes, err := fetchBytes(client, sumsURL)
	if err != nil {
		return nil, fmt.Errorf("fetch checksums: %w", err)
	}

	sigURL := sumsURL + ".sig"
	sigB64, err := fetchBytes(client, sigURL)
	if err != nil {
		return nil, fmt.Errorf("fetch signature: %w (this release predates signed checksums or its signature is missing; refusing to install)", err)
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sigB64)))
	if err != nil {
		return nil, fmt.Errorf("decode signature from %s: %w", sigURL, err)
	}
	if !ed25519.Verify(pub, sumsBytes, sig) {
		return nil, fmt.Errorf("checksums signature verification failed for %s (refusing to install)", sumsURL)
	}

	return parseChecksums(sumsBytes)
}

// fetchBytes fetches a URL fully into memory. A non-200 response is an error.
func fetchBytes(client *http.Client, url string) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

// parseChecksums parses a goreleaser checksums.txt into a map of asset filename
// -> lowercase hex SHA-256. Each line is "<hex>  <filename>".
func parseChecksums(data []byte) (map[string]string, error) {
	sums := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		// goreleaser prefixes the path-stripped asset name; use the base name.
		sums[filepath.Base(fields[1])] = fields[0]
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(sums) == 0 {
		return nil, fmt.Errorf("no checksum entries parsed")
	}
	return sums, nil
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
