---
description: Shared review-rigor checklist used by gh-pr and gh-self-review — do not invoke directly. Same analysis applied to whichever diff is in scope.
category: principles
triggers:
  - review-rigor
when: Loaded by /gh-pr (Phase 5) and /gh-self-review (Phase 3) to apply a uniform diff analysis. Not a standalone workflow — invoke one of the wrapping skills instead.
related:
  - meta/gh-pr
  - meta/gh-self-review
  - meta/truthseeker
---

# Shared review rigor

The truthseeker-rigor analysis is identical whether you're reviewing your own pending changes (`/gh-self-review`) or another author's posted PR (`/gh-pr`). This file is the single source for that checklist; the two skills wrap it with their own scope-detection, output, and posting semantics.

## Principles

- **Prove before flagging.** A blocker without evidence is noise. Cite the line, the caller, the race, the missing test. If you can't cite, don't flag.
- **Prove before approving.** Approval / "looks clean" is a positive claim that the change is correct and safe. Back it with what you actually read and verified.
- **Reject fragility.** Hunt for setups that require multiple things to go right — package globals, hidden ordering dependencies, "works on my machine" patterns, vestigial code.
- **Make invisible visible.** "Fixes X" / "improves Y" claims need a test, benchmark, trace, or at minimum a plausible code-level explanation in the diff. Call out unverified claims explicitly.
- **Compare, don't absolve.** Every decision has a cost. If you can only name the benefit, you haven't thought hard enough.
- **Intellectual honesty.** Don't soften blockers to be polite. Don't inflate strengths to cushion criticism. Don't pad with review-theater bullets. "Nothing to flag" is a valid finding.

## Verify the change's claims

The body of a PR, an issue, a branch name, or a commit message is **a claim**, not evidence. Before accepting any of it:

- **"Fixes #N" / "Closes #N"** — read the linked issue's acceptance criteria. Does the diff actually satisfy them, or does it satisfy something adjacent? If the issue asked for X and the diff delivers "X-ish", flag it.
- **Performance claims** ("p99 1s → 200µs", "removes N dials") — benchmark, repro, telemetry link, or at minimum a plausible code-level explanation visible in the diff? Unverified perf claims are blockers until substantiated — even if the change looks reasonable, "trust me it's faster" is not evidence.
- **Bug-fix claims without a regression test** — "this fixes the crash" with no test means the next regression is invisible. Default position: blocker.
- **"Works on all supported platforms"** — unless the diff or CI shows it, this is a claim, not a fact.

## Classify scope

Set analysis depth based on the files touched. Only the analysis subsections whose scope class fired here run.

- **Docs-only** (`*.md`, frontmatter, KB): links, frontmatter schema, heading structure, convention match against sibling docs, attribution footer where required. Skip runtime analysis.
- **Deps-only** (`go.mod`/`go.sum`, `package.json`/lockfiles, etc.): verify new deps' publishers (supply-chain plausibility), licenses, version bumps vs. version jumps, removed deps that look load-bearing (grep for usages in the repo).
- **Generated-only**: verify the generated diff is consistent with the hand-edited changes it accompanies. Orphan regenerations are a yellow flag — they mean either the tool ran against a different input, or the hand-edit was abandoned.
- **Code**: full analysis (the subsections below). Within Code, classify the language(s) touched — `Go`, `JS/TS`, `Python`, `Godot` (any of `*.gd`, `*.tscn`, `*.tres`, `*.res`, `*.uid`, `*.import`, `*.gdshader`, `*.gdshaderinc`, `project.godot`, `export_presets.cfg`, `addons/**`), etc. — and run only the matching language subsections.
- **Mixed**: code rules apply; don't let docs/deps changes dilute scrutiny of the code changes.

## Don't stop at easy findings

Surface findings (dead code, doc typos, format) are cheap; the point is the expensive bugs the cheap pass misses. After your first pass, do a second pass — even when you already have enough findings — checking each of these:

- **State machines** — enumerate event orderings and ask what else now misfires; React refs vs state, deferred callbacks, handlers fired from multiple paths.
- **Schema migrations** — both fresh `CREATE TABLE` and upgrade `ALTER` paths; every new column in both; silent-skip masking real failures.
- **Struct↔JSON boundaries** — every field in the JSON sample present in the struct.
- **New shared state concurrency** — singletons/registries: written once or many times, read concurrently, safe by mutex or by construction (and what enforces that).
- **New error paths** — does each `err != nil` recover, degrade, or silently break?
- **Hidden coupling** — hard-coded paths/ports/filenames vs the deploy reality.
- **Tests as theatre** — would the test fail without the fix? Race tests cover only the schedules they sample.
- **Removed code blast radius** — grep the whole repo for stale references after a deletion.

Bar: if a bug surfaces in two weeks, would you be embarrassed you missed it? If yes, the second pass wasn't deep enough. Surface findings are not a stopping condition.

**Each finding is still one short sentence + file:line.** Depth lives in the audit, not the prose.

## Correctness

For each non-trivial change, ask:

- What's the invariant this code assumes? Is it preserved?
- Who calls this? Search for callers (`grep -rn "FuncName"` if the repo is local) — not every caller is in the diff.
- What's the failure mode if this goes wrong? Silent data corruption is worse than a loud crash; a loud crash worse than a returned error.
- Error handling: errors returned, logged with context, or silently swallowed? Are `err == nil` checks flipped to `err != nil` with early returns?
- Nil derefs, off-by-one, resource leaks (unclosed files/conns/tickers), context cancellation gaps, goroutine leaks.
- Concurrency: shared mutable state, data races, mutex discipline, ordering dependencies between goroutines.

## Fragility hunt

Actively look for patterns that require multiple things to go right:

- **Package-level mutable globals** — not keyed by instance, fine "as long as" X holds. Write down the X. If X is implicit, that's the footgun.
- **Ordering dependencies** — "this must run before that" that nothing enforces. Look for setup functions that wrap previous state, or initialization that assumes a specific call order.
- **Dead code / vestigial config** — is there anything added that removing wouldn't break? If yes, it shouldn't exist.
- **"Works on my machine"** — fixtures referenced but not committed, env vars with no defaults, hard-coded paths.
- **Silent fallbacks** — catch-all `except:`, `|| true`, swallowed errors, default values masking misconfiguration.

## Performance

- Hot-path allocations, N+1 queries, blocking calls on request paths.
- Unbounded growth (slices, maps, caches without eviction).
- Mutex contention (coarse locks on hot paths).
- Timer-driven polling that should be event-driven, or the reverse.
- If the change claims a perf win: was it measured? (See "Verify the change's claims" above.)

## Tests

Tests are evidence. Missing tests on a hot-path change, a caching change, or a concurrency change is usually a blocker, not a suggestion.

- Bug fix: is there a regression test? If not, the next regression is silent.
- New feature: happy path + at least one edge case covered?
- Caching / invalidation: hit, miss, and invalidation tested?
- Concurrency: race test (`go test -race`) or equivalent?
- Performance claim: benchmark?

## Security

- Auth bypass, credential exposure, injection (SQL / shell / path traversal).
- Unvalidated input at system boundaries (HTTP handlers, CLI args, file paths).
- Unsafe file operations (weak perms, `filepath.Join` with user input, symlink races).
- Secrets checked in, tokens in logs, debug endpoints exposed.
- Never quote secrets inline even when flagging — point at the line and describe the class.

## Scope discipline

In-scope cleanup the change didn't do is a **blocker**, not a non-blocker. If the change introduced or moved code that left dead, vestigial, or contradictory artifacts, those need to be cleaned up — fixing them is cheap, the author is right here, and "we'll get it next time" is how rot accumulates.

Treat as blockers when introduced or made visible by this change:

- Commented-out code, dead branches, `// removed`-style placeholders. If the intent was to remove something, the comment-out form is a half-finished implementation.
- Stale comments / log strings / docstrings referencing functions, flags, or paths that the change renamed or deleted.
- Vestigial config, callbacks, or interface methods left orphaned after the only caller was removed.
- Generated artifacts that don't match the hand-written edits (regenerate or revert — orphan generations indicate the tool ran on the wrong input).
- Debug prints, `TODO`/`XXX`/`FIXME` markers added in this change without a tracking link.
- Formatting-only noise diluting the real change in the same files.

Pre-existing instances of these (the change didn't introduce them, didn't touch the relevant lines, isn't the natural place to clean them up) are out of scope — drop them entirely, don't downgrade them to non-blockers.

## Conventions

If the repo is locally available:

- Read its `CLAUDE.md` and any `.claude/rules/*.md`.
- Search the detritus KB with `kb_search` for conventions relevant to the change (e.g. ooo patterns, test patterns, state management).
- Grep sibling files for existing patterns before asserting something is non-conventional.

Do not flag "non-conventional" without having verified the convention. "I think Go usually does X" is not evidence; `grep -rn "X" repo/` is.

## Godot (gated — only when the diff touches Godot files)

Skip this entire subsection unless the diff actually touches Godot files. If only the binary export artifacts changed (`.so`, `.dll`, `.pck`) without source, note the unverifiable rebuild and stop here.

**Resource UIDs.** Godot 4.4+ uses `uid://` references for stable cross-scene linking; new resources must have unique UIDs.

- **Duplicate UIDs across the diff or against the existing tree** are a real bug — scene instancing by `uid://` resolves to one of the duplicates non-deterministically across imports, so behaviour can swap between deploys without any code change. Look for `uid="..."` headers in `.tscn` / `.tres` / `.res` and `uid://` references; flag any matches between newly-added files and existing files. Common causes: copy-paste of an asset between customer dirs without regenerating the UID, fork of an existing scene without `Make Local` / `New UID`.
- **Missing `.uid` sidecar files** for new `.gd` are noise on the first import after upgrade (4.4+ auto-generates them); only flag if the diff adds `.gd` files but the matching `.uid` is absent and the rest of the repo commits them.
- A trendboard / baccarat road / dashboard build emitting "UID duplicate detected" warnings during export is the pattern to watch for, even if it doesn't fail the build.

**Naming and structure.**

- Signals: `snake_case` (`emit_signal("hand_dealt")`, not `handDealt`).
- Class names: `class_name PascalCase` matching the file's primary type.
- Node names in `.tscn`: `PascalCase` for scene roots and named children; unique-name nodes (`%`) for nodes accessed across scenes.
- Scene-script coupling: a `.tscn` whose root script changed should still resolve at the scripted node path; a `.gd` with `class_name X` must not collide with an existing `class_name X` elsewhere in the project.

**Lifecycle and memory.**

- `queue_free()` after instancing in pooling code; check for orphaned instances (`get_tree().root.add_child` without later `queue_free`).
- `is_instance_valid(node)` before accessing nodes that may have been freed (especially in deferred callbacks or signal handlers fired after `queue_free`).
- `connect(callable, CONNECT_ONE_SHOT)` for signals that should fire once; otherwise stale connections accumulate when the receiver outlives the emitter.
- `WeakRef` for back-references to avoid cycles when both nodes hold strong refs to each other.
- `_exit_tree` cleanup for things `_ready` set up — timers, autoload subscriptions, file handles.

**Performance.**

- Per-frame allocations in `_process` / `_physics_process`: avoid `String + String`, repeated `get_node`, `Array.new()` calls. Cache node refs in `_ready`, accumulate strings via `PackedStringArray`.
- `signal` over polling for state changes — a `_process` that polls `if some_state_changed:` is almost always wrong vs. emitting a signal at the change site.
- `Resource.duplicate(true)` only when needed; deep duplication is expensive.
- Tween / Animation churn in tight loops — cap or pool.
- Shader uniform churn — set in `_ready` if static, else only on actual change.

**Autoload and project config.**

- New autoloads in `project.godot` need to be declared in `export_presets.cfg` / build pipeline if they're not auto-included; missing exports cause runtime "could not find autoload" only on the exported build, not in the editor.
- Autoload ordering: if A's `_ready` accesses B, B must be declared above A in `project.godot`'s `[autoload]` section.

**Version-specific (Godot 4.6+).**

- `class_exists()` / `is_class()` deprecation patterns.
- `RenderingServer.global_shader_parameter_set` instead of older variants.
- `@warning_ignore` annotations should target a specific code, not blanket-suppress.
- `@tool` scripts: any side-effect in `_ready` runs in the editor — flag editor-only state mutation.

**Tests (gated further — only when the project has a Godot test framework).**

- GUT (Godot Unit Test) is the most common: tests live under `test/` or `tests/`, files match `test_*.gd`, run via `godot --headless --script res://addons/gut/gut_cmdln.gd -gdir=res://test -gexit`.
- For a Godot bug fix, a regression test means a GUT test (or an in-engine `assert`-driven test) that fires the buggy code path and asserts the new expected behaviour.
- Missing fixtures in tests (referenced `.tres` / `.tscn` / textures not committed) are the same `t.Skip("requires fixture")` antipattern as in Go — flag fixtures-not-in-repo as fragile.

## Large diffs

For diffs >500 lines: prioritize files in the change's stated scope, then skim the rest for drift. **Say so in the report.** Pretending to have read every line of a 5000-line diff is worse than saying "I prioritized these files".
