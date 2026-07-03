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
// `reg query`, or "" when it is unset. Uses reg/setx rather than the registry
// API so this file compiles on every platform.
func readWindowsUserPath() string {
	out, err := exec.Command("reg", "query", `HKCU\Environment`, "/v", "PATH").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 3 && strings.EqualFold(fields[0], "PATH") {
			// reg output: "PATH  REG_SZ  <value>"; value is everything after the type.
			idx := strings.Index(line, fields[1])
			return strings.TrimSpace(line[idx+len(fields[1]):])
		}
	}
	return ""
}

// addDirToWindowsUserPath persists dir onto the user PATH via setx when it is
// not already present.
func addDirToWindowsUserPath(dir string) error {
	newPath, changed := computeWindowsUserPath(readWindowsUserPath(), dir)
	if !changed {
		return nil
	}
	if err := exec.Command("setx", "PATH", newPath).Run(); err != nil {
		return fmt.Errorf("setx PATH: %w", err)
	}
	return nil
}
