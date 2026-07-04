package main

import (
	"fmt"
	"os/exec"
	"strings"
)

// windowsPathContains reports whether dir is already an entry in a Windows PATH
// string (";"-separated, case-insensitive, trailing-separator tolerant).
func windowsPathContains(pathVar, dir string) bool {
	want := strings.ToLower(strings.TrimRight(dir, `\`))
	for _, p := range strings.Split(pathVar, ";") {
		p = strings.ToLower(strings.TrimRight(strings.TrimSpace(p), `\`))
		if p != "" && p == want {
			return true
		}
	}
	return false
}

// computeWindowsUserPath returns the user PATH with dir appended and whether a
// change is needed. An empty existing PATH yields just dir.
func computeWindowsUserPath(existing, dir string) (string, bool) {
	if windowsPathContains(existing, dir) {
		return existing, false
	}
	if strings.TrimSpace(existing) == "" {
		return dir, true
	}
	return strings.TrimRight(existing, ";") + ";" + dir, true
}

// readWindowsUserPath returns the persistent user PATH from the registry via
// `reg query`, or "" when it is unset, together with its registry value type
// (REG_SZ or REG_EXPAND_SZ). Uses reg rather than the registry API so this file
// compiles on every platform. The type is preserved on write so an existing
// expandable PATH is not silently downgraded.
func readWindowsUserPath() (value, regType string) {
	out, err := exec.Command("reg", "query", `HKCU\Environment`, "/v", "PATH").Output()
	if err != nil {
		return "", ""
	}
	return parseWindowsUserPath(string(out))
}

// parseWindowsUserPath extracts the PATH value and its registry type from the raw
// `reg query HKCU\Environment /v PATH` output. It returns ("", "") when no PATH
// line is present. Split out from readWindowsUserPath so the parse — the highest-
// risk part — is unit-testable without invoking reg (which does not exist off
// Windows). A REG_EXPAND_SZ value keeps its %VAR% literal so the write preserves
// the type.
func parseWindowsUserPath(out string) (value, regType string) {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 3 && strings.EqualFold(fields[0], "PATH") {
			// reg output: "PATH  REG_SZ  <value>"; value is everything after the type.
			idx := strings.Index(line, fields[1])
			return strings.TrimSpace(line[idx+len(fields[1]):]), fields[1]
		}
	}
	return "", ""
}

// addDirToWindowsUserPath persists dir onto the user PATH when it is not already
// present. It uses `reg add` rather than `setx`: setx silently truncates the
// user PATH at 1024 chars and always writes REG_SZ, which would downgrade an
// existing REG_EXPAND_SZ value and break %VAR% expansion. `reg add` has no such
// length limit, and we preserve the existing value type (defaulting to
// REG_EXPAND_SZ so newly created PATHs stay expandable).
func addDirToWindowsUserPath(dir string) error {
	existing, regType := readWindowsUserPath()
	newPath, changed := computeWindowsUserPath(existing, dir)
	if !changed {
		return nil
	}
	if !strings.EqualFold(regType, "REG_SZ") {
		regType = "REG_EXPAND_SZ"
	}
	cmd := exec.Command("reg", "add", `HKCU\Environment`, "/v", "PATH", "/t", regType, "/d", newPath, "/f")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("reg add PATH: %w", err)
	}
	return nil
}
