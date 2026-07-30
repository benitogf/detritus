package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadSettingsDefaultsWhenAbsent verifies that with no settings file the
// effective settings are the per-role defaults and no warning is raised.
func TestLoadSettingsDefaultsWhenAbsent(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())

	eff, warnings := loadSettings()
	if len(warnings) != 0 {
		t.Fatalf("absent file must not warn; got %v", warnings)
	}
	if eff.Levels[roleReviewer].Model != "claude-fable-5" || eff.Levels[roleReviewer].Thinking != "high" {
		t.Fatalf("reviewer default wrong: %+v", eff.Levels[roleReviewer])
	}
	if eff.Levels[roleCoder].Model != "inherit" || eff.Levels[roleCoder].Thinking != "low" {
		t.Fatalf("coder default wrong: %+v", eff.Levels[roleCoder])
	}
}

// TestLoadSettingsPartialOverlay verifies a partial file overlays only the
// fields it sets, leaving the rest at their defaults.
func TestLoadSettingsPartialOverlay(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DETRITUS_HOME", home)
	writeSettingsFile(t, home, `{"levels":{"reviewer":{"model":"claude-opus-4-8"}}}`)

	eff, warnings := loadSettings()
	if len(warnings) != 0 {
		t.Fatalf("valid partial file must not warn; got %v", warnings)
	}
	if eff.Levels[roleReviewer].Model != "claude-opus-4-8" {
		t.Fatalf("reviewer model overlay failed: %+v", eff.Levels[roleReviewer])
	}
	if eff.Levels[roleReviewer].Thinking != "high" {
		t.Fatalf("reviewer thinking should stay default high: %+v", eff.Levels[roleReviewer])
	}
	if eff.Levels[roleCoder].Model != "inherit" {
		t.Fatalf("coder should stay default: %+v", eff.Levels[roleCoder])
	}
}

// TestLoadSettingsInvalidIgnoredWithWarning verifies invalid model, thinking,
// and unknown role are each ignored and surfaced as a warning.
func TestLoadSettingsInvalidIgnoredWithWarning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DETRITUS_HOME", home)
	writeSettingsFile(t, home, `{"levels":{
		"reviewer":{"model":"gpt-9","thinking":"ludicrous"},
		"stranger":{"model":"claude-opus-4-8"}
	}}`)

	eff, warnings := loadSettings()
	if eff.Levels[roleReviewer].Model != "claude-fable-5" || eff.Levels[roleReviewer].Thinking != "high" {
		t.Fatalf("invalid reviewer fields must fall back to default: %+v", eff.Levels[roleReviewer])
	}
	joined := strings.Join(warnings, "\n")
	for _, want := range []string{"gpt-9", "ludicrous", "stranger"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("warning must mention %q; got:\n%s", want, joined)
		}
	}
}

// TestLoadSettingsUnparseableJSON verifies a corrupt file yields defaults plus
// exactly one warning, never an error to the caller.
func TestLoadSettingsUnparseableJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DETRITUS_HOME", home)
	writeSettingsFile(t, home, `{not valid json`)

	eff, warnings := loadSettings()
	if len(warnings) != 1 {
		t.Fatalf("unparseable file must yield exactly one warning; got %v", warnings)
	}
	if eff.Levels[roleReviewer].Model != "claude-fable-5" {
		t.Fatalf("unparseable file must fall back to defaults: %+v", eff.Levels[roleReviewer])
	}
}

// TestNormalizeModel exercises the full alias table, canonical passthrough,
// mixed case, and an unresolvable input.
func TestNormalizeModel(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"fable", "claude-fable-5", true},
		{"opus", "claude-opus-4-8", true},
		{"sonnet", "claude-sonnet-5", true},
		{"haiku", "claude-haiku-4-5-20251001", true},
		{"session", "inherit", true},
		{"OPUS", "claude-opus-4-8", true},
		{"  Sonnet  ", "claude-sonnet-5", true},
		{"claude-opus-4-8", "claude-opus-4-8", true},
		{"inherit", "inherit", true},
		{"gpt-4", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := normalizeModel(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("normalizeModel(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

// TestNormalizeThinking validates the enum and rejects anything else.
func TestNormalizeThinking(t *testing.T) {
	for _, ok := range []string{"low", "medium", "high", "xhigh", "max", "HIGH", " max "} {
		if _, valid := normalizeThinking(ok); !valid {
			t.Errorf("normalizeThinking(%q) should be valid", ok)
		}
	}
	for _, bad := range []string{"", "extreme", "none"} {
		if _, valid := normalizeThinking(bad); valid {
			t.Errorf("normalizeThinking(%q) should be invalid", bad)
		}
	}
}

// TestSaveLoadRoundtrip verifies a saved selection reloads canonicalized.
func TestSaveLoadRoundtrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DETRITUS_HOME", home)

	if err := saveSettings(Settings{Levels: map[string]LevelConfig{
		roleReviewer: {Model: "opus", Thinking: "medium"},
	}}); err != nil {
		t.Fatalf("saveSettings: %v", err)
	}

	eff, warnings := loadSettings()
	if len(warnings) != 0 {
		t.Fatalf("roundtrip must not warn; got %v", warnings)
	}
	if eff.Levels[roleReviewer].Model != "claude-opus-4-8" || eff.Levels[roleReviewer].Thinking != "medium" {
		t.Fatalf("roundtrip lost data: %+v", eff.Levels[roleReviewer])
	}

	// Persisted file must carry the canonical id, not the alias.
	raw, _ := os.ReadFile(filepath.Join(home, "settings.json"))
	if !strings.Contains(string(raw), "claude-opus-4-8") || strings.Contains(string(raw), `"opus"`) {
		t.Fatalf("persisted file must store canonical id, not alias:\n%s", raw)
	}
}

// TestSaveSettingsRejectsInvalid verifies the write path is strict.
func TestSaveSettingsRejectsInvalid(t *testing.T) {
	t.Setenv("DETRITUS_HOME", t.TempDir())

	if err := saveSettings(Settings{Levels: map[string]LevelConfig{
		roleCoder: {Model: "gpt-9"},
	}}); err == nil {
		t.Fatal("saveSettings must reject an unresolvable model")
	}
	if err := saveSettings(Settings{Levels: map[string]LevelConfig{
		"stranger": {Model: "opus"},
	}}); err == nil {
		t.Fatal("saveSettings must reject an unknown role")
	}
}

// TestSettingsSetToleratesPreexistingJunk proves a valid settings_set (and a
// per-role reset) succeeds even when the store already holds hand-edited junk
// (an invalid field and an unknown role): rejection-on-write is scoped to the
// caller's input, and the persisted store comes out sanitized.
func TestSettingsSetToleratesPreexistingJunk(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DETRITUS_HOME", home)
	writeSettingsFile(t, home, `{"levels":{"reviewer":{"thinking":"hihg"},"helper":{"model":"opus"}}}`)

	if _, err := applySettingsSet("reviewer", "opus", "", false); err != nil {
		t.Fatalf("valid set must succeed over a junk store: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(home, "settings.json"))
	if strings.Contains(string(raw), "helper") || strings.Contains(string(raw), "hihg") {
		t.Fatalf("persisted store must be sanitized of prior junk:\n%s", raw)
	}
	if !strings.Contains(string(raw), "claude-opus-4-8") {
		t.Fatalf("valid set must persist:\n%s", raw)
	}

	// A per-role reset must also succeed regardless of prior junk (an unknown
	// role in the store previously made saveSettings reject the reset).
	writeSettingsFile(t, home, `{"levels":{"helper":{"model":"opus"},"reviewer":{"thinking":"hihg"}}}`)
	if _, err := applySettingsSet("reviewer", "", "", true); err != nil {
		t.Fatalf("reset must succeed over a junk store: %v", err)
	}
}

func writeSettingsFile(t *testing.T, home, body string) {
	t.Helper()
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "settings.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
