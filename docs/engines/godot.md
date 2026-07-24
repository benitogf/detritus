---
description: Godot engine review reference — the Godot-specific review mechanics (uid:// resources, project.godot / export_presets.cfg / autoloads, version-gated notes, GUT tests) that don't generalize to other engines. Loaded via kb_get when a diff touches Godot files; not a command.
triggers:
  - godot
  - gdscript
  - tscn
  - uid
  - gut
  - export presets
when: A diff touches Godot files (.gd, .tscn, .tres, .godot, .import, .uid) — kb_get this from core/review-rigor's engine/language review-reference hook and apply it.
related:
  - core/review-rigor
---

# Godot Engine Review Reference

Loaded via `kb_get` from `core/review-rigor`'s engine/language review-reference hook when a diff's classified engine is Godot. This is the last-resort residue: the truly Godot-specific mechanics that don't generalize to other engines. The generic review disciplines (naming, lifecycle/memory, reference-aliasing change-detection, performance) live in `core/review-rigor` itself and apply across engines — do not duplicate them here.

## File extensions that trigger this doc

A diff touches Godot files when it changes any of: `.gd` (GDScript), `.tscn` (packed scenes), `.tres` / `.res` (resources), `.godot` (`project.godot`), `.import` (import metadata), `.uid` (UID sidecars), `export_presets.cfg`. If only binary export artifacts changed (`.so`, `.dll`, `.pck`) without source, note the unverifiable rebuild and stop.

## Resource UIDs (Godot 4.4+)

Godot 4.4+ uses `uid://` references for stable cross-scene linking. Every resource carries a unique identifier; new resources must have unique UIDs.

- **Duplicate UIDs across the diff or against the existing tree** are a real bug. Scene instancing by `uid://` resolves to one of the duplicates non-deterministically across imports, so behaviour can swap between deploys with no code change at all. Look for `uid="..."` headers in `.tscn` / `.tres` / `.res` and `uid://` references; flag any collisions between newly-added files and existing files. Common causes: copy-paste of an asset between directories without regenerating the UID, or forking an existing scene without `Make Local` / `New UID`.
- A build emitting **"UID duplicate detected"** warnings during export is the pattern to watch for, even when it doesn't fail the build — it signals a duplicate that will resolve non-deterministically.
- **Missing `.uid` sidecar files** for new `.gd` are noise on the first import after upgrade (4.4+ auto-generates them). Only flag if the diff adds `.gd` files, the matching `.uid` is absent, and the rest of the repo commits them.

## Autoload and project config

- New autoloads declared in `project.godot`'s `[autoload]` section need to be included by `export_presets.cfg` / the build pipeline if they aren't auto-included; a missing export surfaces as a runtime "could not find autoload" **only on the exported build**, not in the editor.
- **Autoload init ordering:** autoloads initialize top-to-bottom in the `[autoload]` section. If A's `_ready` accesses B, B must be declared above A in `project.godot` — otherwise A initializes against an absent singleton.

Example (generic — order-sensitive):

```ini
[autoload]

; ConfigStore must precede any autoload whose _ready reads config
ConfigStore="*res://autoload/config_store.gd"
SessionManager="*res://autoload/session_manager.gd"  ; _ready reads ConfigStore
```

## Version-gated notes (Godot 4.6+)

- `class_exists()` / `is_class()` deprecation patterns.
- `RenderingServer.global_shader_parameter_set` instead of older variants.
- `@warning_ignore` annotations should target a specific code, not blanket-suppress.
- `@tool` scripts: any side-effect in `_ready` runs in the editor — flag editor-only state mutation.

## GUT tests and regression expectations

- **GUT (Godot Unit Test)** is the most common framework: tests live under `test/` or `tests/`, files match `test_*.gd`, and run headless via:

```
godot --headless --script res://addons/gut/gut_cmdln.gd -gdir=res://test -gexit
```

- For a Godot bug fix, a **regression test** means a GUT test (or an in-engine `assert`-driven test) that fires the buggy code path and asserts the new expected behaviour.
- **Missing fixtures** — referenced `.tres` / `.tscn` / textures not committed — are the same `t.Skip("requires fixture")` antipattern as in Go: flag fixtures-not-in-repo as fragile, since the test silently degrades to a no-op when the asset is absent.
