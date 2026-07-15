package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSameRelease(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"v3.43.0", "3.43.0", true},
		{"3.43.0", "v3.43.0", true},
		{"v3.43.0", "v3.43.0", true},
		{"3.43.0", "3.44.0", false},
		{"dev", "v3.43.0", false},
	}
	for _, c := range cases {
		if got := sameRelease(c.a, c.b); got != c.want {
			t.Errorf("sameRelease(%q,%q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestCanonicalTag(t *testing.T) {
	cases := []struct{ in, want string }{
		{"3.43.0", "v3.43.0"},
		{"v3.43.0", "v3.43.0"},
		{"", ""},
	}
	for _, c := range cases {
		if got := canonicalTag(c.in); got != c.want {
			t.Errorf("canonicalTag(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseUpdateArgs(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		pin     string
		dry     bool
		wantErr string
	}{
		{"empty", nil, "", false, ""},
		{"tag then dry-run", []string{"v3.0.0", "--dry-run"}, "v3.0.0", true, ""},
		{"dry-run then tag", []string{"--dry-run", "v3.0.0"}, "v3.0.0", true, ""},
		{"unknown flag", []string{"--dry-rum"}, "", false, "--dry-rum"},
		{"two tags", []string{"v1.0.0", "v2.0.0"}, "", false, "multiple tags"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pin, dry, err := parseUpdateArgs(c.args)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("expected error containing %q, got %v", c.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pin != c.pin || dry != c.dry {
				t.Errorf("got pin=%q dry=%v, want pin=%q dry=%v", pin, dry, c.pin, c.dry)
			}
		})
	}
}

func TestParseChecksums(t *testing.T) {
	body := []byte("abc123  detritus_linux_amd64.tar.gz\n" +
		"def456  detritus_windows_amd64.zip\n" +
		"\n" +
		"garbage-line-with-one-field\n")
	sums, err := parseChecksums(body)
	if err != nil {
		t.Fatalf("parseChecksums: %v", err)
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

	if _, err := parseChecksums([]byte("\n\n")); err == nil {
		t.Error("expected error on empty checksums, got nil")
	}
}

// signedChecksumsServer serves checksums.txt and (optionally) its .sig.
func signedChecksumsServer(t *testing.T, priv ed25519.PrivateKey, body []byte, serveSig, tamperSig bool) *httptest.Server {
	t.Helper()
	sig := ed25519.Sign(priv, body)
	if tamperSig {
		sig[0] ^= 0xff
	}
	sigB64 := base64.StdEncoding.EncodeToString(sig) + "\n"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".sig") {
			if !serveSig {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(sigB64))
			return
		}
		_, _ = w.Write(body)
	}))
}

func TestVerifiedChecksums(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("abc123  detritus_linux_amd64.tar.gz\n")
	client := &http.Client{Timeout: time.Second}

	t.Run("valid signature passes", func(t *testing.T) {
		srv := signedChecksumsServer(t, priv, body, true, false)
		defer srv.Close()
		sums, err := verifiedChecksums(client, srv.URL+"/checksums.txt", pub)
		if err != nil {
			t.Fatalf("expected pass, got %v", err)
		}
		if sums["detritus_linux_amd64.tar.gz"] != "abc123" {
			t.Errorf("unexpected sums: %v", sums)
		}
	})

	t.Run("tampered signature fails", func(t *testing.T) {
		srv := signedChecksumsServer(t, priv, body, true, true)
		defer srv.Close()
		_, err := verifiedChecksums(client, srv.URL+"/checksums.txt", pub)
		if err == nil || !strings.Contains(err.Error(), "signature verification failed") {
			t.Errorf("expected verification failure, got %v", err)
		}
	})

	t.Run("missing sig refuses with predates message", func(t *testing.T) {
		srv := signedChecksumsServer(t, priv, body, false, false)
		defer srv.Close()
		_, err := verifiedChecksums(client, srv.URL+"/checksums.txt", pub)
		if err == nil || !strings.Contains(err.Error(), "predates signed checksums") {
			t.Errorf("expected predates-signing error, got %v", err)
		}
	})
}

func TestArchiveFileNameShape(t *testing.T) {
	name := archiveFileName()
	if !strings.HasPrefix(name, "detritus_") {
		t.Errorf("archive name %q missing detritus_ prefix", name)
	}
	if !strings.HasSuffix(name, ".tar.gz") && !strings.HasSuffix(name, ".zip") {
		t.Errorf("archive name %q has unexpected extension", name)
	}
	if got := releaseDownloadURL("v1.2.3"); !strings.HasSuffix(got, name) {
		t.Errorf("download URL %q does not end with archive name %q", got, name)
	}
}

// buildTarGz builds a gzipped tar containing a single "detritus" file.
func buildTarGz(t *testing.T, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: "detritus", Mode: 0o755, Size: int64(len(content))}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestDownloadVerifiedBinary(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	archiveName := archiveFileName()
	archive := buildTarGz(t, []byte("fake-detritus-binary"))
	h := sha256.Sum256(archive)
	sum := hex.EncodeToString(h[:])

	serve := func(tamper bool) *httptest.Server {
		body := []byte(sum + "  " + archiveName + "\n")
		sig := ed25519.Sign(priv, body)
		sigB64 := base64.StdEncoding.EncodeToString(sig) + "\n"
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "checksums.txt.sig"):
				_, _ = w.Write([]byte(sigB64))
			case strings.HasSuffix(r.URL.Path, "checksums.txt"):
				_, _ = w.Write(body)
			default:
				out := archive
				if tamper {
					out = append([]byte("X"), archive...)
				}
				_, _ = w.Write(out)
			}
		}))
	}

	t.Run("match extracts", func(t *testing.T) {
		srv := serve(false)
		defer srv.Close()
		path, err := downloadVerifiedBinary(srv.URL+"/"+archiveName, srv.URL+"/checksums.txt", archiveName, pub)
		if err != nil {
			t.Fatalf("expected extraction, got %v", err)
		}
		if path == "" {
			t.Error("expected non-empty binary path")
		}
	})

	t.Run("tampered archive hard-fails", func(t *testing.T) {
		srv := serve(true)
		defer srv.Close()
		_, err := downloadVerifiedBinary(srv.URL+"/"+archiveName, srv.URL+"/checksums.txt", archiveName, pub)
		if err == nil || !strings.Contains(err.Error(), "mismatch") {
			t.Errorf("expected checksum mismatch, got %v", err)
		}
	})
}
