package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ulikunitz/xz"
)

// makeTarGz builds a gzip-tar holding files keyed by path → content.
func makeTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	writeTar(t, tw, files)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// makeTarXz builds an xz-tar holding files keyed by path → content.
func makeTarXz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	xw, err := xz.NewWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(xw)
	writeTar(t, tw, files)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := xw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeTar(t *testing.T, tw *tar.Writer, files map[string]string) {
	t.Helper()
	for name, content := range files {
		hdr := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}
		if content == "" {
			hdr.Typeflag = tar.TypeDir
			hdr.Size = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if hdr.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(content)); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestExtractByBase confirms each extractor pulls the binary out by basename even
// when it sits in a subdirectory, and errors when the binary is absent.
func TestExtractByBase(t *testing.T) {
	want := "BINARY-CONTENT"

	tarGzFiles := map[string]string{
		"d2-v9/":          "",
		"d2-v9/README.md": "readme",
		"d2-v9/bin/":      "",
		"d2-v9/bin/._d2":  "appledouble", // basename ._d2, must NOT match
		"d2-v9/bin/d2":    want,
	}
	xzFiles := map[string]string{
		"typst-x/":        "",
		"typst-x/LICENSE": "lic",
		"typst-x/typst":   want,
	}
	// Windows D2 ships bin/._d2.exe alongside bin/d2.exe (and arrives in a tar.gz),
	// so cover the .exe AppleDouble skip on both the tar and zip paths.
	winTarGzFiles := map[string]string{
		"d2-v9/":             "",
		"d2-v9/bin/":         "",
		"d2-v9/bin/._d2.exe": "appledouble", // basename ._d2.exe, must NOT match
		"d2-v9/bin/d2.exe":   want,
	}
	zipFiles := map[string]string{
		"typst-x/":            "",
		"typst-x/._typst.exe": "appledouble", // basename ._typst.exe, must NOT match
		"typst-x/typst.exe":   want,
	}

	cases := []struct {
		name    string
		write   func(string) error
		extract func(string, string, string) (string, error)
		binName string
	}{
		{"targz", func(p string) error { return os.WriteFile(p, makeTarGz(t, tarGzFiles), 0o644) }, extractByBaseFromTarGzFile, "d2"},
		{"targz-win-exe", func(p string) error { return os.WriteFile(p, makeTarGz(t, winTarGzFiles), 0o644) }, extractByBaseFromTarGzFile, "d2.exe"},
		{"tarxz", func(p string) error { return os.WriteFile(p, makeTarXz(t, xzFiles), 0o644) }, extractByBaseFromTarXzFile, "typst"},
		{"zip", func(p string) error { return os.WriteFile(p, makeZip(t, zipFiles), 0o644) }, extractByBaseFromZipFile, "typst.exe"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			arc := filepath.Join(t.TempDir(), "a")
			if err := tc.write(arc); err != nil {
				t.Fatal(err)
			}
			destDir := t.TempDir()
			got, err := tc.extract(arc, tc.binName, destDir)
			if err != nil {
				t.Fatalf("extract: %v", err)
			}
			defer os.Remove(got)
			if filepath.Dir(got) != destDir {
				t.Errorf("temp binary should land in destDir %q, got %q", destDir, got)
			}
			b, err := os.ReadFile(got)
			if err != nil || string(b) != want {
				t.Fatalf("content = %q err=%v, want %q", b, err, want)
			}

			if _, err := tc.extract(arc, "nope", destDir); err == nil {
				t.Error("missing binary should error")
			}
		})
	}
}

// TestDownloadAndExtract serves a fake archive over httptest and asserts the
// binary lands at dest, with the executable bit set and a 404 erroring cleanly.
func TestDownloadAndExtract(t *testing.T) {
	want := "TOOLBIN"
	arc := makeTarGz(t, map[string]string{
		"d2-v9/":       "",
		"d2-v9/bin/":   "",
		"d2-v9/bin/d2": want,
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(arc)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "bin", "d2")
	if err := downloadAndExtract(srv.URL+"/d2.tar.gz", archiveTarGz, "d2", dest); err != nil {
		t.Fatalf("downloadAndExtract: %v", err)
	}
	b, err := os.ReadFile(dest)
	if err != nil || string(b) != want {
		t.Fatalf("dest = %q err=%v, want %q", b, err, want)
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(dest); fi.Mode().Perm()&0o100 == 0 {
			t.Error("installed binary should be executable")
		}
	}

	if err := downloadAndExtract(srv.URL+"/missing", archiveTarGz, "d2", filepath.Join(t.TempDir(), "d2")); err == nil {
		t.Error("a 404 should error")
	}
}

// TestToolAssetNames pins the per-os/arch asset names, including Typst's Rust
// triple mapping (amd64→x86_64, arm64→aarch64) and D2's tag-in-name + GOARCH use.
func TestToolAssetNames(t *testing.T) {
	cases := []struct {
		goos, goarch string
		typst        string
		typstKind    archiveKind
		d2           string
	}{
		{"linux", "amd64", "typst-x86_64-unknown-linux-musl.tar.xz", archiveTarXz, "d2-v0.7.1-linux-amd64.tar.gz"},
		{"linux", "arm64", "typst-aarch64-unknown-linux-musl.tar.xz", archiveTarXz, "d2-v0.7.1-linux-arm64.tar.gz"},
		{"windows", "amd64", "typst-x86_64-pc-windows-msvc.zip", archiveZip, "d2-v0.7.1-windows-amd64.tar.gz"},
		{"windows", "arm64", "typst-aarch64-pc-windows-msvc.zip", archiveZip, "d2-v0.7.1-windows-arm64.tar.gz"},
	}

	origOS, origArch := goosVar, goarchVar
	t.Cleanup(func() { goosVar, goarchVar = origOS, origArch })

	for _, tc := range cases {
		t.Run(tc.goos+"-"+tc.goarch, func(t *testing.T) {
			goosVar, goarchVar = tc.goos, tc.goarch
			a, k := typstAssetFor(tc.goos, tc.goarch)
			if a != tc.typst || k != tc.typstKind {
				t.Errorf("typst = %q,%d want %q,%d", a, k, tc.typst, tc.typstKind)
			}
			d, dk := d2AssetFor("v0.7.1", tc.goos, tc.goarch)
			if d != tc.d2 || dk != archiveTarGz {
				t.Errorf("d2 = %q,%d want %q,targz", d, dk, tc.d2)
			}
		})
	}
}
