package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchChecksumsParses(t *testing.T) {
	body := "abc123  detritus_linux_amd64.tar.gz\n" +
		"def456  detritus_windows_amd64.zip\n" +
		"\n" + // blank line ignored
		"garbage-line-with-one-field\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	sums, err := fetchChecksums(&http.Client{Timeout: time.Second}, srv.URL)
	if err != nil {
		t.Fatalf("fetchChecksums: %v", err)
	}
	if got := sums["detritus_linux_amd64.tar.gz"]; got != "abc123" {
		t.Errorf("linux sum = %q, want abc123", got)
	}
	if got := sums["detritus_windows_amd64.zip"]; got != "def456" {
		t.Errorf("windows sum = %q, want def456", got)
	}
	if len(sums) != 2 {
		t.Errorf("parsed %d entries, want 2 (blank + malformed lines skipped)", len(sums))
	}
}

func TestFetchChecksumsErrors(t *testing.T) {
	// 404 is an error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	if _, err := fetchChecksums(&http.Client{Timeout: time.Second}, srv.URL); err == nil {
		t.Error("expected error on 404, got nil")
	}

	// Empty/all-malformed body yields no entries -> error.
	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("\n\n"))
	}))
	defer empty.Close()
	if _, err := fetchChecksums(&http.Client{Timeout: time.Second}, empty.URL); err == nil {
		t.Error("expected error on empty checksums, got nil")
	}
}

func TestVerifyChecksum(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("DEADBEEF  detritus_linux_amd64.tar.gz\n"))
	}))
	defer srv.Close()
	client := &http.Client{Timeout: time.Second}

	// Match is case-insensitive hex.
	if err := verifyChecksum(client, srv.URL, "detritus_linux_amd64.tar.gz", "deadbeef"); err != nil {
		t.Errorf("expected match, got %v", err)
	}

	// Mismatch is a hard error.
	err := verifyChecksum(client, srv.URL, "detritus_linux_amd64.tar.gz", "00000000")
	if err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Errorf("expected mismatch error, got %v", err)
	}

	// Missing entry is a hard error (never run unverified).
	err = verifyChecksum(client, srv.URL, "detritus_darwin_arm64.tar.gz", "deadbeef")
	if err == nil || !strings.Contains(err.Error(), "no checksum") {
		t.Errorf("expected missing-entry error, got %v", err)
	}
}

func TestArchiveFileNameShape(t *testing.T) {
	name := archiveFileName()
	if !strings.HasPrefix(name, "detritus_") {
		t.Errorf("archive name %q missing detritus_ prefix", name)
	}
	if !strings.HasSuffix(name, ".tar.gz") && !strings.HasSuffix(name, ".zip") {
		t.Errorf("archive name %q has unexpected extension", name)
	}
	// The checksums.txt lookup key must equal the downloaded asset's base name.
	if got := releaseDownloadURL("v1.2.3"); !strings.HasSuffix(got, name) {
		t.Errorf("download URL %q does not end with archive name %q", got, name)
	}
}
