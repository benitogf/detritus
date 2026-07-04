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

func TestParseWindowsUserPath(t *testing.T) {
	cases := []struct {
		name        string
		out         string
		wantValue   string
		wantRegType string
	}{
		{
			name:        "REG_SZ value",
			out:         "\r\nHKEY_CURRENT_USER\\Environment\r\n    PATH    REG_SZ    C:\\Windows;C:\\Users\\x\\AppData\\Local\\detritus\r\n",
			wantValue:   `C:\Windows;C:\Users\x\AppData\Local\detritus`,
			wantRegType: "REG_SZ",
		},
		{
			name:        "REG_EXPAND_SZ preserves %VAR% literal",
			out:         "\r\nHKEY_CURRENT_USER\\Environment\r\n    PATH    REG_EXPAND_SZ    %USERPROFILE%\\bin;C:\\tools\r\n",
			wantValue:   `%USERPROFILE%\bin;C:\tools`,
			wantRegType: "REG_EXPAND_SZ",
		},
		{
			name:        "value containing spaces is kept whole",
			out:         "    PATH    REG_SZ    C:\\Program Files\\x;C:\\other\r\n",
			wantValue:   `C:\Program Files\x;C:\other`,
			wantRegType: "REG_SZ",
		},
		{
			name:        "value containing the type-token substring is not truncated",
			out:         "    PATH    REG_SZ    C:\\REG_SZ_tools;C:\\other\r\n",
			wantValue:   `C:\REG_SZ_tools;C:\other`,
			wantRegType: "REG_SZ",
		},
		{
			name:        "no PATH line",
			out:         "\r\nHKEY_CURRENT_USER\\Environment\r\n    TEMP    REG_SZ    C:\\Temp\r\n",
			wantValue:   "",
			wantRegType: "",
		},
		{
			name:        "empty output",
			out:         "",
			wantValue:   "",
			wantRegType: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			value, regType := parseWindowsUserPath(c.out)
			if value != c.wantValue || regType != c.wantRegType {
				t.Errorf("parseWindowsUserPath() = (%q, %q), want (%q, %q)", value, regType, c.wantValue, c.wantRegType)
			}
		})
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
