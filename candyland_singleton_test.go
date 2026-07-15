package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// healthServer starts an httptest server answering GET /api/health with body and
// returns the server plus its bound port, so a test can point discovery at it.
func healthServer(t *testing.T, status int, body string) (*httptest.Server, int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	port := srv.Listener.Addr().(*net.TCPAddr).Port
	return srv, port
}

// candylandIdentityAt (D1) verifies the health BODY, not just a 200: a valid
// ok:true+version body parses to an identity; a 200 with a non-candyland body is a
// foreign app; a refused connection is down.
func TestCandylandIdentityAt(t *testing.T) {
	t.Run("valid body parses to identity", func(t *testing.T) {
		srv, _ := healthServer(t, http.StatusOK, `{"ok":true,"version":"v1.2.3","pid":42,"startedAt":"2026-07-15T00:00:00Z","activeRuns":1,"activeQuests":2,"ui":"window"}`)
		id, err := candylandIdentityAt(srv.URL, time.Second)
		if err != nil {
			t.Fatalf("candylandIdentityAt: %v", err)
		}
		if !id.OK || id.Version != "v1.2.3" || id.PID != 42 || id.ActiveRuns != 1 || id.ActiveQuests != 2 || id.UI != "window" {
			t.Fatalf("unexpected identity: %+v", id)
		}
		if id.preUpgrade {
			t.Error("a full body must not be flagged preUpgrade")
		}
	})

	t.Run("200 with non-candyland body is a foreign app", func(t *testing.T) {
		srv, _ := healthServer(t, http.StatusOK, `{"service":"grafana"}`)
		_, err := candylandIdentityAt(srv.URL, time.Second)
		if err == nil || !isForeignApp(err) {
			t.Fatalf("want foreign-app error, got %v", err)
		}
	})

	t.Run("200 with ok:false is a foreign app", func(t *testing.T) {
		srv, _ := healthServer(t, http.StatusOK, `{"ok":false,"version":"v1"}`)
		_, err := candylandIdentityAt(srv.URL, time.Second)
		if !isForeignApp(err) {
			t.Fatalf("ok:false must be a foreign app, got %v", err)
		}
	})

	t.Run("pre-upgrade body missing identity fields", func(t *testing.T) {
		srv, _ := healthServer(t, http.StatusOK, `{"ok":true,"version":"v0.9"}`)
		id, err := candylandIdentityAt(srv.URL, time.Second)
		if err != nil {
			t.Fatalf("candylandIdentityAt: %v", err)
		}
		if !id.preUpgrade {
			t.Error("a body missing pid/activeRuns must be flagged preUpgrade")
		}
	})

	t.Run("refused connection is down (not foreign app)", func(t *testing.T) {
		// A port nothing listens on: grab a free one and immediately release it.
		port, err := pickFreePort()
		if err != nil {
			t.Fatal(err)
		}
		_, err = candylandIdentityAt(fmt.Sprintf("http://127.0.0.1:%d", port), 500*time.Millisecond)
		if err == nil || isForeignApp(err) {
			t.Fatalf("a refused connection must be down, not foreign, got %v", err)
		}
	})
}

// isForeignApp is the errors.Is check the launcher uses (D5), mirrored for assertions.
func isForeignApp(err error) bool { return errors.Is(err, errCandylandForeignApp) }

// decideTakeover (D3) is the smart-takeover ladder: match→drive, skew+idle→restart,
// skew+busy→fail, headless+display+idle→restart, headless+display+busy→warn,
// pre-upgrade→warn, dev/unknown skew→warn. Running work is never killed.
func TestDecideTakeover(t *testing.T) {
	full := func(version, ui string, runs, quests int) candylandIdentity {
		return candylandIdentity{OK: true, Version: version, PID: 1, UI: ui, ActiveRuns: runs, ActiveQuests: quests}
	}
	cases := []struct {
		name           string
		id             candylandIdentity
		installed      string
		displayMarkers bool
		want           takeoverAction
	}{
		{"match + UI healthy → drive", full("v1", "window", 0, 0), "v1", true, takeoverDrive},
		{"skew + idle → restart", full("v1", "window", 0, 0), "v2", true, takeoverRestart},
		{"skew + busy run → fail", full("v1", "window", 1, 0), "v2", true, takeoverFail},
		{"skew + busy quest → fail", full("v1", "window", 0, 1), "v2", true, takeoverFail},
		{"headless + display + idle → restart", full("v1", "headless", 0, 0), "v1", true, takeoverRestart},
		{"headless + display + busy → warn+proceed", full("v1", "headless", 1, 0), "v1", true, takeoverWarnProceed},
		{"headless + NO display → drive", full("v1", "headless", 0, 0), "v1", false, takeoverDrive},
		{"pre-upgrade → warn+proceed", candylandIdentity{OK: true, Version: "v0.9", preUpgrade: true}, "v1", true, takeoverWarnProceed},
		{"dev sidecar, enforcement skipped → drive", full("dev", "window", 0, 0), "v2", true, takeoverDrive},
		{"unknown installed, enforcement skipped → drive", full("v1", "window", 0, 0), "", true, takeoverDrive},
		{"dev sidecar headless + display + idle → restart (UI still heals)", full("dev", "headless", 0, 0), "v2", true, takeoverRestart},
	}
	const dashURL = "http://localhost:53999"
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := decideTakeover(tc.id, tc.installed, tc.displayMarkers, dashURL)
			if got.action != tc.want {
				t.Errorf("decideTakeover = %v (%q), want %v", got.action, got.reason, tc.want)
			}
			if got.action == takeoverFail && !strings.Contains(got.reason, "busy") {
				t.Errorf("a fail verdict must name the running counts: %q", got.reason)
			}
			// The busy headless warn tells the user where the dashboard is — it must
			// name the RESOLVED URL passed in, not a default-port guess.
			if got.action == takeoverWarnProceed && !tc.id.preUpgrade && !strings.Contains(got.reason, dashURL) {
				t.Errorf("busy headless warn must name the resolved dashboard URL: %q", got.reason)
			}
		})
	}
}

// ensureCandylandUp must never shut a working sidecar down when no installed binary
// exists to replace it (the restart would strand the user with nothing) — it errors
// out first, leaving the sidecar untouched.
func TestEnsureCandylandUpNoBinaryNeverKillsSidecar(t *testing.T) {
	origMarkers := displayMarkersDetected
	displayMarkersDetected = func() bool { return true }
	defer func() { displayMarkersDetected = origMarkers }()

	shutdownHit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/health":
			w.WriteHeader(http.StatusOK)
			// Idle + headless + display markers → the ladder wants a restart.
			_, _ = w.Write([]byte(`{"ok":true,"version":"v1","pid":9,"activeRuns":0,"activeQuests":0,"ui":"headless"}`))
		case "/api/shutdown":
			shutdownHit = true
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	port := srv.Listener.Addr().(*net.TCPAddr).Port

	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".candyland"), 0o700); err != nil {
		t.Fatal(err)
	}
	epPath := filepath.Join(home, ".candyland", "endpoint.json")
	if err := os.WriteFile(epPath, []byte(fmt.Sprintf(`{"apiPort":%d,"spaPort":9999,"pid":9,"version":"v1"}`, port)), 0o600); err != nil {
		t.Fatal(err)
	}

	// detritusPath in an empty temp dir → no candyland binary beside it.
	err := ensureCandylandUp(filepath.Join(t.TempDir(), "detritus"))
	if !errors.Is(err, errCandylandNotInstalled) {
		t.Fatalf("ensureCandylandUp = %v, want errCandylandNotInstalled", err)
	}
	if shutdownHit {
		t.Error("the running sidecar was shut down despite no installed binary to replace it")
	}
}

// versionEnforcement (D4): equal enforces (no skew), differing enforces skew, and a
// dev/unknown on EITHER side skips enforcement.
func TestVersionEnforcement(t *testing.T) {
	cases := []struct {
		sidecar, installed string
		wantSkew, wantEnf  bool
	}{
		{"v1", "v1", false, true},
		{"v1", "v2", true, true},
		{"dev", "v2", false, false},
		{"v1", "dev", false, false},
		{"", "v2", false, false},
		{"v1", "", false, false},
	}
	for _, tc := range cases {
		skew, enf := versionEnforcement(tc.sidecar, tc.installed)
		if skew != tc.wantSkew || enf != tc.wantEnf {
			t.Errorf("versionEnforcement(%q,%q) = skew:%v enforce:%v, want skew:%v enforce:%v", tc.sidecar, tc.installed, skew, enf, tc.wantSkew, tc.wantEnf)
		}
	}
}

// readCandylandEndpoint (D2/C4) parses a well-formed endpoint file and rejects every
// degrade case (absent, malformed, no API port).
func TestReadCandylandEndpoint(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "endpoint.json")
	if err := os.WriteFile(good, []byte(`{"apiPort":9001,"spaPort":9002,"pid":7,"version":"v1","startedAt":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ep, ok := readCandylandEndpoint(good)
	if !ok || ep.APIPort != 9001 || ep.SPAPort != 9002 {
		t.Fatalf("readCandylandEndpoint good = %+v ok:%v", ep, ok)
	}

	bad := filepath.Join(dir, "bad.json")
	_ = os.WriteFile(bad, []byte(`not json`), 0o600)
	if _, ok := readCandylandEndpoint(bad); ok {
		t.Error("malformed endpoint file must not verify")
	}
	noPort := filepath.Join(dir, "noport.json")
	_ = os.WriteFile(noPort, []byte(`{"spaPort":9002}`), 0o600)
	if _, ok := readCandylandEndpoint(noPort); ok {
		t.Error("endpoint file without an API port must not verify")
	}
	if _, ok := readCandylandEndpoint(filepath.Join(dir, "absent.json")); ok {
		t.Error("absent endpoint file must not verify")
	}
	if _, ok := readCandylandEndpoint(""); ok {
		t.Error("empty path must not verify")
	}
}

// resolveExistingCandyland (D2) prefers a VERIFIED endpoint-file port over the
// defaults, and a stale file (dead port) falls through.
func TestResolveExistingCandylandPrefersEndpointFile(t *testing.T) {
	srv, port := healthServer(t, http.StatusOK, `{"ok":true,"version":"v1","pid":9,"activeRuns":0,"activeQuests":0,"ui":"window"}`)
	_ = srv

	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".candyland"), 0o700); err != nil {
		t.Fatal(err)
	}
	epPath := filepath.Join(home, ".candyland", "endpoint.json")
	if err := os.WriteFile(epPath, []byte(fmt.Sprintf(`{"apiPort":%d,"spaPort":9999,"pid":9,"version":"v1"}`, port)), 0o600); err != nil {
		t.Fatal(err)
	}

	res, found := resolveExistingCandyland(time.Second)
	if !found {
		t.Fatal("expected the verified endpoint-file sidecar to resolve")
	}
	if res.apiPort != port {
		t.Errorf("resolved apiPort = %d, want the endpoint-file port %d", res.apiPort, port)
	}
	if res.id.Version != "v1" {
		t.Errorf("resolved identity version = %q, want v1", res.id.Version)
	}
}

func TestResolveExistingCandylandStaleFileFallsThrough(t *testing.T) {
	deadPort, err := pickFreePort() // nothing listens here
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	_ = os.MkdirAll(filepath.Join(home, ".candyland"), 0o700)
	epPath := filepath.Join(home, ".candyland", "endpoint.json")
	_ = os.WriteFile(epPath, []byte(fmt.Sprintf(`{"apiPort":%d,"spaPort":9999}`, deadPort)), 0o600)

	// A stale file's dead port must never resolve as the live endpoint. Resolution
	// may still find a real sidecar on the default port (dev machines run one), but
	// it must never return the stale, dead port.
	if res, found := resolveExistingCandyland(300 * time.Millisecond); found && res.apiPort == deadPort {
		t.Errorf("a stale endpoint file (dead port %d) must not resolve as a live sidecar", deadPort)
	}
}

// candylandLaunchArgs (D5) always carries explicit ports and --openBrowser.
func TestCandylandLaunchArgs(t *testing.T) {
	args := candylandLaunchArgs(51000, 51001)
	joined := strings.Join(args, " ")
	for _, want := range []string{"--port 51000", "--spaPort 51001", "--openBrowser"} {
		if !strings.Contains(joined, want) {
			t.Errorf("launch args %q missing %q", joined, want)
		}
	}
}

// buildCandylandLaunchCmd (D5) builds a cmd carrying the picked free ports,
// --openBrowser, and the DETRITUS_BIN env seam.
func TestBuildCandylandLaunchCmd(t *testing.T) {
	cmd := buildCandylandLaunchCmd("/path/to/detritus", "/path/to/candyland", 52000, 52001)
	joined := strings.Join(cmd.Args, " ")
	for _, want := range []string{"/path/to/candyland", "--port 52000", "--spaPort 52001", "--openBrowser"} {
		if !strings.Contains(joined, want) {
			t.Errorf("cmd.Args %q missing %q", joined, want)
		}
	}
	hasDetritusBin := false
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "DETRITUS_BIN=") {
			hasDetritusBin = true
		}
	}
	if !hasDetritusBin {
		t.Error("launch cmd must carry a DETRITUS_BIN env value")
	}
}

// launchCandyland (D5) hands the built cmd — carrying the picked ports and
// --openBrowser — to the spawn seam. The seam captures it and returns an error,
// so launchCandyland returns before any health wait — the launch can be asserted
// without a real process or a poll.
func TestLaunchCandylandSpawnSeamCarriesPorts(t *testing.T) {
	origSpawn := startDetachedProcess
	defer func() { startDetachedProcess = origSpawn }()

	var captured *exec.Cmd
	startDetachedProcess = func(cmd *exec.Cmd) error {
		captured = cmd
		return fmt.Errorf("stub: not really spawned") // short-circuit the health wait
	}

	_ = launchCandyland("/path/to/detritus", "/path/to/candyland", 53000, 53001)
	if captured == nil {
		t.Fatal("spawn seam was never called")
	}
	joined := strings.Join(captured.Args, " ")
	for _, want := range []string{"--port 53000", "--spaPort 53001", "--openBrowser"} {
		if !strings.Contains(joined, want) {
			t.Errorf("spawned cmd.Args %q missing %q", joined, want)
		}
	}
}

// candylandUIOutcomeLine (D6) prints the three distinct UI-outcome messages.
func TestCandylandUIOutcomeLine(t *testing.T) {
	cases := map[string]string{
		"window":   "candyland window opened",
		"browser":  "dashboard opened in your browser",
		"headless": "no display available — dashboard: http://localhost:8080",
	}
	for ui, want := range cases {
		got := candylandUIOutcomeLine(candylandIdentity{UI: ui}, "http://localhost:8080")
		if got != want {
			t.Errorf("candylandUIOutcomeLine(ui=%q) = %q, want %q", ui, got, want)
		}
	}
	// An empty/unknown mode (pre-upgrade sidecar) prints the URL without claiming
	// anything about the display — its state is not known.
	if got := candylandUIOutcomeLine(candylandIdentity{}, "http://x"); got != "dashboard: http://x" {
		t.Errorf("unknown UI mode = %q, want the plain dashboard URL", got)
	}
}

// shutdownCandyland (D3) surfaces a 409 (active work) as an error so the launcher
// never kills running work, and returns nil once the sidecar stops answering.
func TestShutdownCandyland(t *testing.T) {
	t.Run("409 active work → error, no kill", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/shutdown" {
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte(`{"activeRuns":1,"activeQuests":0}`))
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()
		err := shutdownCandyland(srv.URL, time.Second)
		if err == nil || !strings.Contains(err.Error(), "409") {
			t.Fatalf("want a 409 refusal error, got %v", err)
		}
	})

	t.Run("200 then health goes down → nil", func(t *testing.T) {
		var down bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/shutdown":
				down = true
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"ok":true}`))
			case "/api/health":
				if down {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"ok":true,"version":"v1"}`))
			}
		}))
		defer srv.Close()
		if err := shutdownCandyland(srv.URL, 2*time.Second); err != nil {
			t.Fatalf("shutdownCandyland: %v", err)
		}
	})
}

// #149: the sidecar endpoints must be overridable so a non-default deployment or
// a second sidecar on other ports can be targeted without a rebuild.
func TestEnvURLOr(t *testing.T) {
	const key = "CANDYLAND_TEST_URL"
	t.Setenv(key, "")
	if got := envURLOr(key, "http://default:1"); got != "http://default:1" {
		t.Errorf("blank env should yield default, got %q", got)
	}
	t.Setenv(key, "  http://override:9  ")
	if got := envURLOr(key, "http://default:1"); got != "http://override:9" {
		t.Errorf("set env should win (trimmed), got %q", got)
	}
}

// #149: the inherited-MCP file carries secret env values, so it must live in a
// per-user cache dir (fixed name, no accumulation) — never a predictable path in
// the world-shared TempDir where concurrent users collide.
func TestOriginMCPConfigPathPerUser(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache) // platformCacheDir honors this on linux

	path, err := originMCPConfigPath()
	if err != nil {
		t.Fatalf("originMCPConfigPath: %v", err)
	}
	if !strings.HasPrefix(path, cache+string(os.PathSeparator)) {
		t.Errorf("path %q is not under the per-user cache dir %q", path, cache)
	}
	if filepath.Base(path) != "origin-mcp.json" {
		t.Errorf("path %q should end in the fixed name origin-mcp.json", path)
	}
	// The parent dir is created 0700 (owner-only) so the secret file is not
	// exposed to other users.
	info, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat parent dir: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("cache dir perm = %v, want 0700", info.Mode().Perm())
	}
}

// portOfURL feeds a takeover relaunch concrete ports even when the env-override
// seam supplied the URL: explicit port wins; a missing port or unparseable URL
// falls back to the default.
func TestPortOfURL(t *testing.T) {
	if got := portOfURL("http://127.0.0.1:9123", 8888); got != 9123 {
		t.Errorf("explicit port should win, got %d", got)
	}
	if got := portOfURL("http://127.0.0.1", 8888); got != 8888 {
		t.Errorf("missing port should yield default, got %d", got)
	}
	if got := portOfURL("://bad", 8888); got != 8888 {
		t.Errorf("unparseable URL should yield default, got %d", got)
	}
}
