---
description: Godot engine review reference — the Godot-specific review mechanics (uid:// resources, project.godot / export_presets.cfg / autoloads, node lifecycle & reference semantics, GDScript performance, GUT tests) that don't generalize to other engines. Loaded via kb_get when a diff touches Godot files; not a command.
triggers:
  - godot
  - gdscript
  - tscn
  - uid
  - gut
  - export presets
when: A diff touches Godot files (.gd, .gdshader, .gdshaderinc, .tscn, .tres, .godot, .import, .uid, export_presets.cfg, or anything under addons/) — kb_get this from core/review-rigor's engine/language review-reference hook and apply it.
related:
  - core/review-rigor
---

# Godot Engine Review Reference

Loaded via `kb_get` from `core/review-rigor`'s engine/language review-reference hook when a diff's classified engine is Godot. This is the last-resort residue: the Godot-specific review mechanics that don't generalize to other engines. The **generic, language-agnostic** build/packaging disciplines — unique generated/serialized identifiers, init/dependency ordering, packaging/export-manifest completeness, and deferred-flag ordering — live in `core/review-rigor` (its *Build and packaging integrity* section and *Trace the lived path*) and apply across engines. Apply those generic rules first, then the Godot specifics below on top; don't restate the generic rules here.

## File extensions that trigger this doc

A diff touches Godot files when it changes any of: `.gd` (GDScript), `.gdshader` / `.gdshaderinc` (shaders), `.tscn` (packed scenes), `.tres` / `.res` (resources), `.godot` (`project.godot`), `.import` (import metadata), `.uid` (UID sidecars), `export_presets.cfg`, or anything under `addons/`. If only binary export artifacts changed (`.so`, `.dll`, `.pck`) without source, note the unverifiable rebuild and stop.

## Resource UIDs (Godot 4.4+)

Godot 4.4+ uses `uid://` references for stable cross-scene linking. Every resource carries a unique identifier; new resources must have unique UIDs.

- **Duplicate UIDs across the diff or against the existing tree** are a real bug. Scene instancing by `uid://` resolves to one of the duplicates non-deterministically across imports, so behaviour can swap between deploys with no code change at all. Look for `uid="..."` headers in `.tscn` / `.tres` / `.res` and `uid://` references; flag any collisions between newly-added files and existing files. Common causes: copy-paste of an asset between directories without regenerating the UID, or forking an existing scene without `Make Local` / `New UID`.
- A build emitting **"UID duplicate detected"** warnings during export is the pattern to watch for, even when it doesn't fail the build — it signals a duplicate that will resolve non-deterministically.
- **Missing `.uid` sidecar files** for new `.gd` are noise on the first import after upgrade (4.4+ auto-generates them). Only flag if the diff adds `.gd` files, the matching `.uid` is absent, and the rest of the repo commits them.

## Naming and structure

- Signals: `snake_case` (`emit_signal("item_added")`, not `itemAdded`).
- Class names: `class_name PascalCase` matching the file's primary type.
- Node names in `.tscn`: `PascalCase` for scene roots and named children; unique-name nodes (`%`) for nodes accessed across scenes.
- Scene-script coupling: a `.tscn` whose root script changed should still resolve at the scripted node path; a `.gd` with `class_name X` must not collide with an existing `class_name X` elsewhere in the project.

## Lifecycle and memory

- `queue_free()` after instancing in pooling code; check for orphaned instances (`get_tree().root.add_child` without a later `queue_free`).
- `is_instance_valid(node)` before accessing nodes that may have been freed (especially in deferred callbacks or signal handlers fired after `queue_free`).
- `connect(callable, CONNECT_ONE_SHOT)` for signals that should fire once; otherwise stale connections accumulate when the receiver outlives the emitter.
- `WeakRef` for back-references to avoid cycles when both nodes hold strong refs to each other.
- `_exit_tree` cleanup for things `_ready` set up — timers, autoload subscriptions, file handles.

## Reference semantics

- A `Dictionary` / `Array` returned by an accessor is a **reference**, not a copy. When the source mutates it **in place** — a live record a backend or streaming layer patches field-by-field — storing that reference and later comparing it against a fresh read of the same source is **always equal**: both sides are the same object mutating together. Any change-detection built on that comparison (a de-dup guard, an "only react when it changed" cache, `if new == _last: return`) **silently never fires**, so the UI keeps the first value forever. The tell: code does `_last = accessor()` (or `_current`, `_prev`) where `accessor()` reaches into a live-patched store, then guards on `x == _last`. Fix: **snapshot before storing** — `_last = value.duplicate()` (deep `duplicate(true)` only if the payload nests containers; shallow suffices for a flat dict). A test that fabricates its own input dicts cannot reproduce this — it only surfaces against the real, in-place-mutated source, so "verified with an injected payload" is not verification of the aliasing path.

## Performance

- Per-frame allocations in `_process` / `_physics_process`: avoid `String + String`, repeated `get_node`, `Array.new()` calls. Cache node refs in `_ready`, accumulate strings via `PackedStringArray`.
- `signal` over polling for state changes — a `_process` that polls `if some_state_changed:` is almost always wrong vs. emitting a signal at the change site.
- `Resource.duplicate(true)` only when needed; deep duplication is expensive.
- Tween / Animation churn in tight loops — cap or pool.
- Shader uniform churn — set in `_ready` if static, else only on actual change.

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
