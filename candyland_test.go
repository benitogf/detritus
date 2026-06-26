package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// downloadRawBinary lands the fetched bytes at dest (creating the dir, 0755),
// leaves no temp behind, and writes nothing on a failed download.
func TestDownloadRawBinary(t *testing.T) {
	body := []byte("#!/bin/sh\necho candyland\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	// Happy path — dest dir doesn't exist yet, so MkdirAll must create it.
	dest := filepath.Join(t.TempDir(), "bin", "candyland")
	if err := downloadRawBinary(srv.URL+"/candyland", dest); err != nil {
		t.Fatalf("download: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != string(body) {
		t.Fatalf("dest content wrong: %q err=%v", got, err)
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(dest); fi.Mode().Perm()&0o100 == 0 {
			t.Error("downloaded binary should be executable")
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(dest))
	if len(entries) != 1 || entries[0].Name() != "candyland" {
		t.Errorf("dest dir should hold only the binary, no temp leftovers: %v", entries)
	}

	// Failure path — a 404 errors and writes no file.
	dest2 := filepath.Join(t.TempDir(), "candyland")
	if err := downloadRawBinary(srv.URL+"/missing", dest2); err == nil {
		t.Error("a 404 download should error")
	}
	if _, err := os.Stat(dest2); !os.IsNotExist(err) {
		t.Error("no binary should be written on a failed download")
	}
}

// dirWritable detects a writable dir; candylandBinPath prefers the sibling of
// detritus when that dir is writable.
func TestDirWritableAndBinPath(t *testing.T) {
	dir := t.TempDir()
	if !dirWritable(dir) {
		t.Fatal("a fresh temp dir should be writable")
	}
	name := "candyland"
	if runtime.GOOS == "windows" {
		name = "candyland.exe"
	}
	detr := filepath.Join(dir, "detritus")
	if got, want := candylandBinPath(detr), filepath.Join(dir, name); got != want {
		t.Errorf("candylandBinPath = %q, want sibling %q", got, want)
	}

	// When the sibling dir isn't writable, the path falls back to ~/.local/bin so
	// a sudo-installed detritus still wires up the sidecar without sudo. A missing
	// dir forces dirWritable=false deterministically — a chmod-based denial
	// wouldn't hold when the tests run as root (root bypasses the perm bits).
	home := t.TempDir()
	t.Setenv("HOME", home)
	sudoDetr := filepath.Join(dir, "no-such-subdir", "detritus")
	if dirWritable(filepath.Dir(sudoDetr)) {
		t.Fatal("a non-existent dir should not be writable")
	}
	if got, want := candylandBinPath(sudoDetr), filepath.Join(home, ".local", "bin", name); got != want {
		t.Errorf("candylandBinPath fallback = %q, want %q", got, want)
	}
}
