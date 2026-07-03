package main

import "testing"

func TestWindowsPathContains(t *testing.T) {
	cases := []struct {
		pathVar string
		dir     string
		want    bool
	}{
		{`C:\Windows;C:\Users\x\AppData\Local\detritus`, `C:\Users\x\AppData\Local\detritus`, true},
		{`C:\Windows;C:\Users\x\AppData\Local\detritus\`, `C:\Users\x\AppData\Local\detritus`, true},
		{`C:\WINDOWS;C:\USERS\X\APPDATA\LOCAL\DETRITUS`, `C:\Users\x\AppData\Local\detritus`, true},
		{`C:\Windows;C:\Other`, `C:\Users\x\AppData\Local\detritus`, false},
		{``, `C:\Users\x\AppData\Local\detritus`, false},
	}
	for _, c := range cases {
		if got := windowsPathContains(c.pathVar, c.dir); got != c.want {
			t.Errorf("windowsPathContains(%q, %q) = %v, want %v", c.pathVar, c.dir, got, c.want)
		}
	}
}

func TestComputeWindowsUserPath(t *testing.T) {
	dir := `C:\Users\x\AppData\Local\detritus`

	if got, changed := computeWindowsUserPath("", dir); !changed || got != dir {
		t.Errorf("empty PATH: got (%q, %v), want (%q, true)", got, changed, dir)
	}

	existing := `C:\Windows`
	got, changed := computeWindowsUserPath(existing, dir)
	if !changed || got != existing+";"+dir {
		t.Errorf("append: got (%q, %v), want (%q, true)", got, changed, existing+";"+dir)
	}

	already := `C:\Windows;` + dir
	if got, changed := computeWindowsUserPath(already, dir); changed || got != already {
		t.Errorf("already present must be a no-op: got (%q, %v)", got, changed)
	}
}
