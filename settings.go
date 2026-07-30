package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/benitogf/detritus/internal/core"
)

// Per-user role settings: the model + reasoning effort each generated agent
// (the /forge coder, the review-loop reviewer) runs on. Persisted as
// settings.json under core.DataDir(); read on every setup/render so a user's
// choice is honored without editing the generated agent files by hand.

const (
	roleReviewer = "reviewer"
	roleCoder    = "coder"

	modelInherit = "inherit"
)

// LevelConfig is the per-role model + reasoning-effort selection.
type LevelConfig struct {
	Model    string `json:"model"`
	Thinking string `json:"thinking"`
}

// Settings is the persisted per-user configuration: one LevelConfig per role.
type Settings struct {
	Levels map[string]LevelConfig `json:"levels"`
}

// modelAliases maps short input aliases to canonical model ids. This allowlist
// mirrors candyland's internal/conductor/settings.go set — kept in sync
// manually (the two surfaces share no code).
var modelAliases = map[string]string{
	"fable":   "claude-fable-5",
	"opus":    "claude-opus-4-8",
	"sonnet":  "claude-sonnet-5",
	"haiku":   "claude-haiku-4-5-20251001",
	"session": modelInherit,
}

// canonicalModels is the curated allowlist of canonical ids (plus inherit) that
// a settings value may resolve to.
var canonicalModels = map[string]bool{
	"claude-fable-5":            true,
	"claude-opus-4-8":           true,
	"claude-sonnet-5":           true,
	"claude-haiku-4-5-20251001": true,
	modelInherit:                true,
}

// thinkingLevels is the reasoning-effort enum Claude Code honors.
var thinkingLevels = map[string]bool{
	"low":    true,
	"medium": true,
	"high":   true,
	"xhigh":  true,
	"max":    true,
}

// normalizeModel resolves an input (alias or canonical id, case-insensitive)
// to its canonical id. ok=false means the input is unresolvable.
func normalizeModel(input string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(input))
	if key == "" {
		return "", false
	}
	if canonical, ok := modelAliases[key]; ok {
		return canonical, true
	}
	if canonicalModels[key] {
		return key, true
	}
	return "", false
}

// normalizeThinking lowercases and validates an effort value against the enum.
func normalizeThinking(input string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(input))
	if thinkingLevels[key] {
		return key, true
	}
	return "", false
}

// defaultLevelConfig returns the built-in default for a role: reviewer runs on
// claude-fable-5/high, coder inherits the session model at low effort.
func defaultLevelConfig(role string) LevelConfig {
	if role == roleReviewer {
		return LevelConfig{Model: "claude-fable-5", Thinking: "high"}
	}
	return LevelConfig{Model: modelInherit, Thinking: "low"}
}

// defaultSettings is the pure-default effective configuration.
func defaultSettings() Settings {
	return Settings{Levels: map[string]LevelConfig{
		roleReviewer: defaultLevelConfig(roleReviewer),
		roleCoder:    defaultLevelConfig(roleCoder),
	}}
}

// knownRole reports whether role is one detritus renders an agent for.
func knownRole(role string) bool {
	return role == roleReviewer || role == roleCoder
}

// settingsPath is the per-user store location under core.DataDir().
func settingsPath() string {
	return filepath.Join(core.DataDir(), "settings.json")
}

// loadSettings reads the store, overlays every VALID value onto the defaults,
// and returns the effective settings plus a human-readable warning for each
// ignored invalid entry. It tolerates-and-flags on read: an absent file yields
// pure defaults with no warning, an unparseable file yields defaults with one
// warning, and it never returns an error to the caller.
func loadSettings() (Settings, []string) {
	eff := defaultSettings()

	raw, err := os.ReadFile(settingsPath())
	if err != nil {
		return eff, nil
	}

	var stored Settings
	if err := json.Unmarshal(raw, &stored); err != nil {
		return eff, []string{fmt.Sprintf("settings file %s is not valid JSON; using defaults", settingsPath())}
	}

	var warnings []string
	for _, role := range sortedRoles(stored.Levels) {
		lc := stored.Levels[role]
		if !knownRole(role) {
			warnings = append(warnings, fmt.Sprintf("ignoring settings for unknown role %q", role))
			continue
		}
		cur := eff.Levels[role]
		if lc.Model != "" {
			if canonical, ok := normalizeModel(lc.Model); ok {
				cur.Model = canonical
			} else {
				warnings = append(warnings, fmt.Sprintf("ignoring invalid model %q for role %q", lc.Model, role))
			}
		}
		if lc.Thinking != "" {
			if canonical, ok := normalizeThinking(lc.Thinking); ok {
				cur.Thinking = canonical
			} else {
				warnings = append(warnings, fmt.Sprintf("ignoring invalid thinking %q for role %q", lc.Thinking, role))
			}
		}
		eff.Levels[role] = cur
	}
	return eff, warnings
}

// loadStoredSettings reads what the user explicitly persisted WITHOUT overlaying
// defaults, used for provenance (default vs set). Absent/unparseable → empty.
func loadStoredSettings() Settings {
	empty := Settings{Levels: map[string]LevelConfig{}}
	raw, err := os.ReadFile(settingsPath())
	if err != nil {
		return empty
	}
	var stored Settings
	if err := json.Unmarshal(raw, &stored); err != nil || stored.Levels == nil {
		return empty
	}
	return stored
}

// saveSettings normalizes and strictly validates every non-empty field, then
// writes the store (indented, 0o644). Unlike the read path this rejects any
// unresolvable role/model/thinking with an error and persists nothing.
func saveSettings(s Settings) error {
	out := Settings{Levels: map[string]LevelConfig{}}
	for _, role := range sortedRoles(s.Levels) {
		if !knownRole(role) {
			return fmt.Errorf("unknown role %q (allowed: %s, %s)", role, roleReviewer, roleCoder)
		}
		lc := s.Levels[role]
		norm := LevelConfig{}
		if lc.Model != "" {
			canonical, ok := normalizeModel(lc.Model)
			if !ok {
				return fmt.Errorf("invalid model %q (allowed: %s)", lc.Model, strings.Join(allowedModelInputs(), ", "))
			}
			norm.Model = canonical
		}
		if lc.Thinking != "" {
			canonical, ok := normalizeThinking(lc.Thinking)
			if !ok {
				return fmt.Errorf("invalid thinking %q (allowed: %s)", lc.Thinking, strings.Join(allowedThinking(), ", "))
			}
			norm.Thinking = canonical
		}
		out.Levels[role] = norm
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(core.DataDir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(settingsPath(), append(data, '\n'), 0o644)
}

// effectiveLevel returns the effective (model, thinking) a role renders with.
func effectiveLevel(role string) (string, string) {
	eff, _ := loadSettings()
	lc, ok := eff.Levels[role]
	if !ok {
		lc = defaultLevelConfig(role)
	}
	return lc.Model, lc.Thinking
}

// allowedModelInputs lists every accepted model input: canonical ids first,
// then the aliases that resolve to them.
func allowedModelInputs() []string {
	return []string{
		"claude-fable-5", "claude-opus-4-8", "claude-sonnet-5", "claude-haiku-4-5-20251001", modelInherit,
		"fable", "opus", "sonnet", "haiku", "session",
	}
}

// allowedThinking lists the reasoning-effort enum in ascending order.
func allowedThinking() []string {
	return []string{"low", "medium", "high", "xhigh", "max"}
}

// fieldProvenance reports whether a role's field is "set" (a valid explicit
// value in the store) or "default".
func fieldProvenance(stored Settings, role, field string) string {
	lc, ok := stored.Levels[role]
	if !ok {
		return "default"
	}
	val, validate := lc.Model, normalizeModel
	if field == "thinking" {
		val, validate = lc.Thinking, normalizeThinking
	}
	if val == "" {
		return "default"
	}
	if _, valid := validate(val); valid {
		return "set"
	}
	return "default"
}

// renderSettingsReport produces the human-readable effective-settings report
// settings_get returns: per-role values with provenance, the allowed inputs,
// the store path, and any load warnings.
func renderSettingsReport() string {
	eff, warnings := loadSettings()
	stored := loadStoredSettings()

	var b strings.Builder
	b.WriteString("Detritus role settings (effective)\n")
	fmt.Fprintf(&b, "Store: %s\n\n", settingsPath())
	for _, role := range []string{roleReviewer, roleCoder} {
		lc := eff.Levels[role]
		fmt.Fprintf(&b, "%s:\n", role)
		fmt.Fprintf(&b, "  model:    %s (%s)\n", lc.Model, fieldProvenance(stored, role, "model"))
		fmt.Fprintf(&b, "  thinking: %s (%s)\n", lc.Thinking, fieldProvenance(stored, role, "thinking"))
	}
	fmt.Fprintf(&b, "\nallowed model inputs: %s\n", strings.Join(allowedModelInputs(), ", "))
	fmt.Fprintf(&b, "allowed thinking: %s\n", strings.Join(allowedThinking(), ", "))
	if len(warnings) > 0 {
		b.WriteString("\nwarnings:\n")
		for _, w := range warnings {
			fmt.Fprintf(&b, "  - %s\n", w)
		}
	}
	return b.String()
}

// applySettingsSet mutates the persisted store for one settings_set call:
// reset (per-role or global) or a validated model/thinking update. It returns a
// human-readable summary of what changed plus the effective role touched, or an
// error (allowed-values message) when the input is unresolvable — in which case
// nothing is persisted.
func applySettingsSet(role, model, thinking string, reset bool) (string, error) {
	if reset && role == "" {
		if err := saveSettings(Settings{Levels: map[string]LevelConfig{}}); err != nil {
			return "", err
		}
		return "reset all roles to defaults", nil
	}
	if role != "" && !knownRole(role) {
		return "", fmt.Errorf("unknown role %q (allowed: %s, %s)", role, roleReviewer, roleCoder)
	}

	stored := loadStoredSettings()
	oldModel, oldThinking := effectiveLevel(role)

	if reset {
		delete(stored.Levels, role)
	} else {
		if model == "" && thinking == "" {
			return "", fmt.Errorf("nothing to set: provide model, thinking, or reset")
		}
		lc := stored.Levels[role]
		if model != "" {
			canonical, ok := normalizeModel(model)
			if !ok {
				return "", fmt.Errorf("invalid model %q (allowed: %s)", model, strings.Join(allowedModelInputs(), ", "))
			}
			lc.Model = canonical
		}
		if thinking != "" {
			canonical, ok := normalizeThinking(thinking)
			if !ok {
				return "", fmt.Errorf("invalid thinking %q (allowed: %s)", thinking, strings.Join(allowedThinking(), ", "))
			}
			lc.Thinking = canonical
		}
		stored.Levels[role] = lc
	}

	if err := saveSettings(stored); err != nil {
		return "", err
	}

	newModel, newThinking := effectiveLevel(role)
	var b strings.Builder
	if reset {
		fmt.Fprintf(&b, "reset %s to defaults\n", role)
	} else {
		b.WriteString("updated " + role + "\n")
	}
	fmt.Fprintf(&b, "  model:    %s -> %s\n", oldModel, newModel)
	fmt.Fprintf(&b, "  thinking: %s -> %s\n", oldThinking, newThinking)
	fmt.Fprintf(&b, "applies to the next %s spawn in this session.\n", role)
	return b.String(), nil
}

// sortedRoles returns the map's role keys in deterministic order so warnings
// and persisted output are stable.
func sortedRoles(levels map[string]LevelConfig) []string {
	roles := make([]string, 0, len(levels))
	for role := range levels {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	return roles
}
