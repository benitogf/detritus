---
description: Shared review-rigor checklist used by gh-pr and gh-self-review — do not invoke directly. Same analysis applied to whichever diff is in scope.
triggers:
  - review-rigor
when: Loaded by /gh-pr (Phase 5) and /gh-self-review (Phase 3) to apply a uniform diff analysis. Not a standalone workflow — invoke one of the wrapping skills instead.
related:
  - flows/github/gh-pr
  - flows/github/gh-self-review
  - flows/principles/truthseeker
  - roles/reviewer
---

# Shared review rigor

The truthseeker-rigor analysis is identical whether you're reviewing your own pending changes (`/gh-self-review`) or another author's posted PR (`/gh-pr`). This file is the single source for that checklist; the two skills wrap it with their own scope-detection, output, and posting semantics.

## How a reviewer is spawned — load this rubric, never paraphrase it

Any caller that needs a review or audit — a loop's self-review (`/smith`, `/janitor`), a pre-PR check, an ad-hoc "audit this diff" — must spawn the reviewer to **load this doc (and `truthseeker`) via `kb_get`**, with the detritus MCP available to the review agent. In practice that means invoking the real skill: `/gh-self-review` for your own pending changes, `/gh-pr` for a posted PR. The review agent has MCP access — hand it the doc *name* to load, not a summary of it.

**Anti-pattern: the hand-rolled rubric.** Spawning a review sub-agent with an `Agent` prompt that *paraphrases* these criteria is a degraded review: it encodes only the checks the caller happened to remember, so it silently drops the classes this doc enumerates. A paraphrased review passes diffs the full rubric catches (e.g. in one case a hand-rolled self-review green-lit a change that the real `/gh-self-review` then found was shipping a broken plugin file with dead doc references). If you find yourself writing review criteria into an `Agent` prompt, stop and invoke the skill instead.

## Forbidden actions (RV-F#)

Checkable prohibitions for a reviewer. These give IDs to rules stated elsewhere in this doc; a single row fired = the review is degraded and must not post its verdict.

| ID | Forbidden action | Instead | Why (one line) |
|----|------------------|---------|----------------|
| RV-F1 | Improvising / paraphrasing a rubric into an `Agent` prompt instead of loading this doc + `truthseeker` via `kb_get` | Invoke the real skill (`/gh-self-review` or `/gh-pr`); hand the agent the doc *names* to load | A paraphrase drops the classes this doc enumerates (the hand-rolled-rubric anti-pattern above). |
| RV-F2 | Emitting a `clean`/`approve` verdict on unproven wiring (added symbol/route with no cited caller, consumer claimed "elsewhere") | Verdict `changes` with the reachability finding as a blocker (R1/R1b/V2) | Unwired = dead-in-prod; an unverified reach is a standing blocker, not a clear. |
| RV-F3 | Clearing a finding or approving with a hedge-word (*plausibly, likely, should be, probably, seems, elsewhere, …*) | Cite VERIFIED evidence to clear, or let the finding STAND as a blocker (V1) | A verdict that needs a hedge-word is not clear — there is no third state. |
| RV-F4 | Fixing the code yourself to make a finding go away | Write the finding (one sentence + file:line); the coder/author fixes it | The reviewer verifies; a self-fix erases the finding's evidence trail and the author's contract. |
| RV-F5 | Silently discarding a verified finding because it is "pre-existing" / "out of scope" / "not something I need to flag" | Exclude it from the posted verdict, but report it to the user alongside the verdict and route each real, actionable defect to a tracked issue (`/gh-issue-create`) | The scope filter governs the posted artifact, never the signal — a dropped finding is lost forever; a routed one costs one issue. |

✅ Added `ExportHandler` has no non-test caller after a whole-repo grep → verdict `changes`, blocker "ExportHandler unreachable from any entrypoint (R1b) — export_reports.go:40".
❌ Added `ExportHandler` "is probably wired in the router elsewhere" → verdict `clean` (RV-F2 + RV-F3).

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
- **"Works on all supported platforms"** — unless the diff or CI shows it, this is a claim, not a fact. And a green run shows it only for the platforms it actually executed on (the next bullet) — Linux-only CI does not clear a Windows claim.
- **A runnable done-signal is proof only for the environment it ran in.** When acceptance is a runnable command ("the checker exits 0", "the suite is green", "the command succeeds"), executing it and observing success clears the claim **only for the machine/environment it ran on** — the reviewer's box, CI's runner. That is not the same as "it works for the operator" whenever the artifact's declared runtime differs from the build/review environment. Fires when the change ships an artifact an operator or another environment runs elsewhere: an operator-run CLI, a script, a checker, anything whose real execution environment is not the one the done-signal was observed in. Enumerate the environment axes the code is sensitive to and confirm each — or cite the code property that makes it agnostic (axes include, but are not limited to, shell dialect, environment/PATH, CPU arch, and libc alongside the below):
  - **line endings** — a parser matching `\n`/`\r\n` literally breaks on a CRLF checkout (`core.autocrlf=true`, the common Windows default); agnostic ⇔ it normalizes line endings before matching.
  - **path separators** — an OS-native path function (`filepath.Dir`/`Join`) applied to already-slash-form data re-introduces backslashes on Windows and desyncs comparisons against committed slash-form data; agnostic ⇔ it uses slash-only path ops (`path.*`) on slash-form data.
  - **timezone / locale / filesystem case-sensitivity** — a date/collation/case check green in the reviewer's tz/locale/FS can fail in the operator's; agnostic ⇔ it pins or normalizes the axis.

  A done-signal that passed only on the build box, for an artifact whose operator runtime differs, is an **unverified claim** — resolve it like any R-check: `pass` (cite the run under the target environment, OR the code property that makes it environment-independent) · `blocker` (an environment axis the code is sensitive to is unconfirmed) · `n/a` (the artifact runs only in the environment the done-signal was observed in). Detection cue: a PR body asserting "`<command>` exits 0 / passes on this branch" as the contract's done-signal for a tool operators run on a platform the reviewer didn't execute it on.

## Intent fidelity

> When the review brief carries the **driving intent** — the ask that produced this change (a plan contract, an issue body, a feedback spec, the stated request) — the intent is part of the review subject. Verify the diff **satisfies it**: walk each commitment the intent makes and locate where the diff delivers it. A commitment that is missing, only partially delivered, or contradicted by the diff is a **blocker** (`intent unmet: <commitment>`), exactly like a correctness defect. Do not substitute "the code is sound" for "the code is what was asked for" — a defect-free diff that quietly delivers less than, or something adjacent to, the intent is the false-clean this section exists to catch. When no intent is supplied, note its absence in the output and review mechanics only — never invent one.

## Two briefing layers

A reviewer at any gate — run delivery, quest branch delivery — receives intent in **two layers**, and the rule is **visibility global, authority local**:

- **Task layer** — the brief for *this* unit (the run's task intent, the quest objective). This layer is the reviewer's **verdict scope**: it arms blockers. A commitment in the task layer that the diff misses, half-delivers, or contradicts is a blocker, exactly as *Intent fidelity* states.
- **Root layer** — the verbatim, immutable top-level intent this unit serves (the quest's original objective, or, for a standalone run, its own intent — layers coincide there). The reviewer sees it in full for **context only**. It arms **contradiction detection, never completeness**: the reviewer may flag that the diff *contradicts* the root intent, but may never demand that the root intent is fully *delivered*.

**Sibling-absence is never a finding.** A root-intent commitment not present in this diff is owned by sibling work the reviewer cannot see; its absence here is expected, not a defect.

**A contradiction is an `INTENT_CONFLICT`, not a blocker.** When the diff genuinely contradicts the root intent, the reviewer reports it as an `INTENT_CONFLICT` (one line + JSON `{"issue":"…"}`) *in addition to* its task-scoped verdict — never folded into the local fix loop. The conflict routes to a **ruling one level up** (run→quest): a `proceed` ruling resumes delivery, a `fix` ruling converts the conflict into a work item at the level that owns decomposition. The reviewer never fixes a conflict itself.

**Decidability rationale.** A contradiction with the root is **locally decidable** — a single diff either violates a stated commitment or it does not, monotone in what the reviewer can see. Completeness is decidable **only over the union** of all sibling work, which no single reviewer holds — so a local "missing commitment" verdict against the root is unsound by construction.

**Route findings by remaining decision content.** A **citable** finding — one whose `file` exists in the reviewed branch — carries its own fix and goes to the **fix identity** directly. A **non-citable** finding needs decomposition and goes to the **decomposition owner** (quest lead) as a work item. The reviewer never spawns coders and never designs the fix.

## Classify scope

Set analysis depth based on the files touched. Only the analysis subsections whose scope class fired here run.

- **Docs-only** (`*.md`, frontmatter, KB): links, frontmatter schema, heading structure, convention match against sibling docs, attribution footer where required. Skip runtime analysis of code — but docs-only does NOT exempt the diff from R2b (any prose claim about how code behaves is verified against that code, same repo or sibling, not only against other docs) nor from R1's whole-repo reference sweep when the diff deletes or renames an entity (a doc/role deletion's surviving references live in code strings — see R1).
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
- **Removed code blast radius** — grep the whole repo for stale references after a deletion (R1), and the comments/docs/runbooks that still describe it (R2).

Bar: if a bug surfaces in two weeks, would you be embarrassed you missed it? If yes, the second pass wasn't deep enough. Surface findings are not a stopping condition.

**Each finding is still one short sentence + file:line.** Depth lives in the audit, not the prose.

## Trace the lived path (not just the diff)

Diff-correctness is necessary, not sufficient. A change can be internally correct — clean code, passing lint, safe config, consistent docs — and still leave the feature **non-functional for a real user**. For any change to authentication, transport (TLS / HTTP / WebSocket), serving, cross-origin behavior, redirects, sessions / cookies, caching, or deployment topology, walk the **actual runtime sequence end to end** — not just whether the diff reads correctly.

- **Who is the realistic actor, on what realistic (worst-case) machine/state?** Trace the literal steps: first request → response → each subsequent call the client makes → does each one actually succeed? Pick the state that exposes the assumption — a first-time user on a clean machine, a cold cache, an unauthenticated browser, a node that just rebooted.
- **Check the change against the rules the running system obeys.** Many failures live not in the code but in the platform / protocol / security model it runs inside — and those rules are knowable from a spec. Identify the boundaries this change crosses and verify it against each boundary's *real* semantics, not its happy-path intent. Boundary rules to consider: TLS chain/SAN validation and OS trust stores; the browser security model (origins by scheme+host+**port**, mixed content, per-origin certificate trust, CORS, cookie `SameSite`/`Secure` scope); auth and redirect flows; cache coherence and invalidation; filesystem permissions/ownership; process and network reachability. The recurring trap is a change that's locally correct but violates one boundary rule, so the feature **silently** doesn't work end to end (e.g. a multi-origin web app whose cross-origin calls fail because a private CA isn't trusted — and a per-origin warning click-through never covers them).
- **"No e2e environment" is not an excuse.** When the behavior is derivable from a published spec or security model (TLS validation, same-origin policy, mixed-content rules, OAuth redirect flow, cookie scoping, cache semantics), reason it through analytically and state the conclusion. Deferring to "couldn't test against a live stack" when the answer is knowable from the spec is precisely the failure this section exists to prevent.

**Bar:** can a first-time user on a clean machine complete the primary task this change touches? If you can't answer that from the diff plus the relevant spec, that uncertainty is itself a finding — flag it; don't bury it under "looks correct" or "not e2e-testable here."

## Reason over the diff↔repo relationship (mandatory checks R1–R9)

The diff is never the whole system. Most expensive misses live in the *relationship* between the changed lines and the rest of the repo or the assembled running program: code that's correct in the hunk but dead in prod, a test that wires a dependency the entrypoint never builds, a doc that still describes the old behavior, a number that silently shifts on a dashboard. A green suite over a correct-looking diff does not clear any of these. Run each check below that applies; a failure here is a **blocker**, not a "verify later" note.

**Per-check outcome — closed enum** `(do NOT invent others)`: each applicable check resolves to exactly one of `pass` (verified clean, cite what proved it) · `blocker` (the check's failure condition is met — cite line/caller/consumer) · `n/a` (the check's trigger did not fire — the diff has no add/delete, no security gate, no upload, etc.). There is no "verify later", no "probably fine", no "minor". A check whose trigger fired but whose result you could not verify is `blocker`, not a fourth state.

- **R1 — Whole-repo reachability + build-at-HEAD + migration sweep.** On any add/delete/rename, cross out of the diff into the whole tree:
  - **Added** exported symbol / route / flag / prop / config key → `grep` the WHOLE repo for a non-test caller or a registered route. Called only by its own test, or by nothing = dead-in-prod = blocker. Don't soften it to a nit or defer it.
  - **Deleted / renamed** entity — a symbol, route, flag, config key, doc, role, or agent/prompt definition → `grep` the WHOLE repo for surviving references at the *real merge HEAD* (the state the branch actually merges into), and rebuild at that HEAD — not at the diff in isolation. The reference surface is **every file kind, not the kind the diff edited**: string literals and embedded templates in source, docs, configs, CI. Sweeping only the changed entity's own surface class is the canonical miss — a deleted doc's live references survive in code strings (an installer's embedded agent definition still instructing agents to load it), and a deleted symbol's survive in docs. A surviving reference OR a surviving statement of the entity's retired model is a blocker. (Boundary: R1 = entity-lifecycle reference/model survival on delete/rename; R2 = behavior-change prose staleness.)
  - **Removed default / enum value** → check that already-persisted data isn't left carrying the retired value. If it can be, a migration is needed and its absence is the finding.
- **R1b — Trace from the entrypoint, not the hunk — and from EVERY entrypoint for shared code.** For any new route/feature, start at `main.go` / the router / the composition root and confirm the path is reachable end to end. "The in-diff logic is correct" is not "the feature is reachable" — the wiring that connects the hunk to the entrypoint lives outside the hunk, and that's exactly where it gets dropped. A **shared library / exported symbol has more than one entrypoint**: `grep` the whole repo for **every** caller (across other services/packages, not just the file you're reading) and trace each — the credential/session/gate helper you changed is very likely invoked by an in-process function in *another* service, not only by this service's HTTP routes. Reviewing one service's mounted routes while a sibling service calls the same exported function on a different path is the canonical miss.
- **R1b-sec — A security/credential/authorization gate must fire on EVERY entry variant.** When a change adds, splits, moves, or renames an auth gate (account-status, lockout, method-toggle, permission check, session control), enumerate **all** authentication entry points for that credential — the HTTP handler(s) **and** every exported in-process `Authorize*`/verify function other services call — and confirm the gate fires on each. A gate split (e.g. moving deactivation out of a lockout check into a new helper) silently drops the gate on any caller that still calls only the old function. Detection cue: two functions authenticating the same credential (an HTTP handler and an exported in-process variant) with different gate sequences → factor one shared gate helper both call, or flag the drift as a blocker.
- **R1c — Test-wiring must match entrypoint-wiring.** When a test injects a dependency through a mock or helper — a denylist, a fake sender, a hand-built client/RP, an in-test registration — confirm the **production entrypoint constructs the same dependency**. A divergence yields a green suite over a broken production path: the tests mask the integration gap. Detection cue: a test helper named `mock*With*` / `*WithX` supplying wiring the entrypoint lacks.
- **R2 — Comment/doc/runbook co-change sweep.** A behavior / path / flag / name change must update every comment, docstring, `--help` string, README row, and runbook step that describes the old behavior — almost all of which live OUTSIDE the diff. Stale prose that contradicts the new code is a blocker, not cosmetic. **Fenced blocks are sweep subject, not opaque literals**: a prompt/command template embedded in a doc is the literal text an agent or shell receives — the most load-bearing prose in the file — so when a section, flag, or term it references changes, sweep the fence's contents like any prose. Detection cue: a diff that renames or reshapes a section/contract in a doc that also embeds a fenced template referring to it.
- **R2b — Prose-claims-about-code ground-truth check (R2's reverse direction).** When the change's *prose* — a doc, a code comment, a runbook, the PR body — asserts how code behaves ("X is never deleted", "children share branch Y", "the driver retries Z"), READ that code and verify the claim against it; do not clear it by internal consistency alone. A docs diff can be fully self-consistent — every cross-reference resolves, every cited doc agrees — and still be wrong against the code it describes; consistency among documents is not evidence about behavior. The described code is in scope for the review even when it lives outside the diff, **including in a sibling repo** (doctrine describing another service/driver: open that repo's source). Each behavioral claim resolves like any R-check: `pass` (cite the function/lines that exhibit the claimed behavior) · `blocker` (the code contradicts the prose — the prose is the bug to flag) · `n/a` (the diff's prose makes no behavioral claims). A behavioral claim whose ground truth you could not read is `blocker`, not "probably accurate". Detection cues: a docs/comment diff naming concrete mechanisms (branch names, retry/teardown semantics, survival guarantees, ordering); a review note like "consistent with the docs it cites" standing in for verification.
- **R3 — Claim-vs-diff reconciliation.** For "Closes #N", "no behavior change", or an enumerated list of deliverables: verify each claim is actually present in the diff, and that nothing beyond the declared scope changed. Sharper case: a claimed *guarantee* delivered by a **weaker mechanism** than stated, with no test exercising it, is a claim-vs-impl gap — e.g. the body claims "corrupt/truncated input is rejected" but the code only magic-byte-sniffs and then detect-and-corrects, never rejecting. Require the stronger mechanism, or drop the claim.
- **R4 — Failure-path honesty.** Error branches must not overwrite a failure with success, swallow the error into `_`, or fall through to a permissive default (secret/auth paths must fail closed). Whole-object writes built from a partial form must not drop sibling fields. Any change feeding a dashboard, tally, or aggregate must quantify the number it moves.
- **R5 — Test drives the defect path.** For every blocker, ask: would the suite have caught this? Require a test on the *actual* failure / retry / concurrent / boundary path — not an adjacent happy-path test that merely touches the same file.
- **R6 — Validator ordering.** Where validators or filters are evaluated first-match-wins, check that an earlier entry doesn't shadow a later one (the later rule becomes unreachable).
- **R7 — Concurrency / security primitives.** Watch for: re-entrant lock self-deadlock; a lock released across a remote call and then a stale write-back; forgeable or default keys; client-supplied privilege (trusting a field the client controls); a missing signing-method assertion on token verification. **Client-supplied privilege has a decisive cue — sibling divergence:** when a new write path sets an authorization-carrying field (level, role, permission, group/ownership, tenant) that a sibling creation/mutation path for the same entity forces, validates, or derives server-side, the new path must apply the same discipline. A path that copies such a field from client input verbatim — even behind an admin/permission gate — widens the authority surface versus its siblings and is a blocker, not a normal CRUD field. Detection cue: two paths setting the same privilege field with different server-owned discipline (one forces/validates, the newer one honors the client value); e.g. a bulk/import path honoring a role or level that the single-create path forces.
- **R8 — Untrusted/active content on upload *and* serve.** If a change accepts or stores a user-supplied content type that can carry active/executable content (`image/svg+xml`, `text/html`, …), trace the **serve/render** path and confirm execution is neutralized — CSP, `Content-Disposition: attachment` (non-inline), or sanitization. Accept-without-neutralize is stored-XSS / content-injection. This check fires on the **consume side**, not only the accept side. An automated FIX that newly enables such a type (e.g. adding SVG detection to an allowlist or sniffer) can itself open this surface — re-run R8 after any allowlist/detector change. Detection cue: an allowlist or validator admitting svg/html with no non-executable serve guard.
- **R9 — Doctrine-delta generalization.** Fires when the diff adds or modifies **codified guidance** — a KB/doctrine rule, a review-rubric entry, an embedded agent/rule definition, a lint or CI policy. A lesson ships as the **class** it represents, never the incident that surfaced it (`flows/maintainer/grow` → *Generalize past the trigger* is the authoring bar, and it **outranks the review brief's framing** — an author who scoped the delta to the incident will have briefed the reviewer with the same narrow intent; judge against the doctrine, not the brief). Three sub-checks, each `pass`/`blocker`/`n/a`:
  - **Substitution test** — construct a *different* instance of the same failure class (another entity kind, another surface, another flow) and check the new rule still matches it. A rule that fires only for the triggering situation is overfit = blocker.
  - **Placement test** — the rule must live in the doc the class's agents actually load (the shared rubric, check, or core doc those flows compose); the flow doc nearest the incident gets at most a pointer. A general rule buried where only the incident's flow reads it = blocker.
  - **Example demotion** — the triggering incident appears as at most a parenthetical `e.g.`; when the example carries the load, the rule above it is too narrow = blocker.
  Detection cues: a rule whose conditions restate the incident's specifics (same entity kind + same surface + same flow); a delta to exactly one flow doc for a mechanism that is flow-independent; a 📚 learning-loop footer on a PR whose rule names a single repo/file/tool where the mechanism is generic.

## Verdict integrity (V1–V2)

A blocker-class finding once DETECTED does not evaporate because clearing it is convenient. `truthseeker` §1 ("Prove Before Acting") is the foundation here: an assertion is not a fact, "it probably works" is not evidence, and a convenient explanation is rejected until proven. These rules apply that principle to the APPROVE/CLEAN decision itself and add the review-verdict-specific teeth truthseeker doesn't name.

**Verdict — closed enum** `(do NOT invent others)`. A review resolves to exactly one:

- `approve` / `clean` — every applicable R1–R9 check is `pass` and every finding is either resolved or a cited non-blocker. A positive claim the change is correct and safe (`Prove before approving`) — back it with what you read.
- `changes` — at least one standing blocker (any R-check `blocker`, or a V1/V2 finding that could not be cleared by cited evidence). List each blocker as one sentence + file:line.

Forbidden middle states: no `approve-with-reservations`, no `mostly-clean`, no `LGTM-but`. A finding that needs a hedge-word to clear (RV-F3 / V1) makes the verdict `changes`, not a softened `approve`.

✅ Verdict-emit: three R-checks `pass`, one `blocker` (R1b unreachable symbol) → verdict `changes`, one blocker line cited.
❌ Verdict-emit: same state, but "the symbol is likely wired elsewhere" → verdict `approve` (banned: RV-F2, RV-F3, V1).

- **V1 — No speculative clears.** A reviewer that has DETECTED a blocker-class finding — unwired/dead code, a missing consumer, an unenforced constraint, an unverified claim — may clear it ONLY by VERIFIED evidence: cite the actual caller, route, consumer, sibling PR, or test that resolves it. If the mitigating fact cannot be verified, the finding STANDS as a blocker. This is `truthseeker` §1 applied to the verdict: "it's probably fine" is not a clear.
  - **Hedge-words are a verdict smell and are BANNED from a CLEAN/APPROVE verdict:** *plausibly, presumably, likely, should be, probably, seems, I assume, elsewhere, "in a sibling/other branch", "not a genuine blocker in this diff"*. If clearing a finding needs one of these words, it is NOT clear — it is either a standing blocker or a clear-with-cited-evidence. There is no third state.
  - **Detection cue:** a CLEAN verdict whose own prose admits a gap ("not yet wired…, but…"). A verdict that contradicts its own narration is not clean.
  - **Out-of-scope code is not proof.** Speculation that a consumer or caller "lives elsewhere" never clears a reachability gap; only FINDING and CITING it does.
- **V2 — Resolve reachability across the change's FULL shipping scope.** "Unwired in this diff" is checked against the whole scope the change ships in — the integrated branch AND the sibling PRs of a coordinated multi-PR feature (one feature → one PR per repo, or a validator-PR + wiring-PR split). When a consumer is CLAIMED to live in a sibling PR or branch, LOCATE that PR/branch, confirm it wires the symbol, and CITE it; otherwise the symbol is unwired = blocker. This is the verify-don't-assume half of R1/R1b sharpened for the cross-PR clear — do not duplicate the whole-repo reachability sweep of R1, the entrypoint trace of R1b, or the test-vs-entrypoint check of R1c; this extends them to the multi-PR boundary.

## Re-review continuity

The **first** review pass runs in a fresh, uncontaminated context — independence from the authoring conversation is what makes it a real gate (the spawner owns that handoff). Every **re-review after a fix pass** is a different job: verifying that cited findings were resolved. It **continues the same reviewer context** — the diff understanding, the brief, and the evidence trail it already established — with a delta instruction; it never re-derives context it already holds.

- **Fresh evidence, held context.** The continued reviewer re-verifies each cited finding against the live tree: diff the fix commits, re-run the checks it already established, confirm nothing regressed. Held *context* is reused; held *conclusions* are not — the verdict-integrity rules (V1–V2) apply to a re-review verdict unchanged.
- **Fallback.** When the prior reviewer context is unavailable (session gone, agent not continuable), spawn fresh with the full brief — continuity is an optimization, never a gate bypass.
- **Realizations.** The candyland conductor continues the reviewer by resuming its session in the next round; in-session flows (`/gh-self-review`, `/forge` convergence) continue the same review sub-agent instead of spawning a new one per iteration. One contract, two transports.

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

Pre-existing instances of these (the change didn't introduce them, didn't touch the relevant lines, isn't the natural place to clean them up) are out of scope for the **verdict** — exclude them from the posted findings, don't downgrade them to non-blockers. **Excluded ≠ discarded (RV-F5).** A verified finding never silently vanishes: report every excluded finding to the user alongside the verdict (one line — what, where, why it's out of scope), and route each real, actionable defect to its own tracked issue (`/gh-issue-create`). This applies to ANY out-of-scope discovery the review turns up (e.g. a leaky test harness, doc drift, a missing ignore entry), not only the cleanup classes above — "pre-existing" and "out of scope" decide *where* a finding goes, never *whether* it is surfaced.

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

## Consume the diff from live git

Applies to reviewers with a local checkout of the change (a repo, worktree, or branch on disk) — the normal case for self-review, delivery gates, and the candyland reviewer. A flow reviewing a **remote** PR with no local clone (`/gh-pr` over the API) uses its API transport instead; the no-snapshot and tree-pinning principles still apply to how it re-reads the change.

The reviewer has the repo; git is the database. Pull the change on demand — never through a materialized copy:

- **Map first:** `git diff --stat <base>...HEAD` (or the brief's diff command) to see shape and size. Then **per-file diffs** (`git diff <base>...HEAD -- <file>`) for the files under examination, and `Read` for surrounding source. Untracked files are read directly from the tree.
- **Never materialize a full-diff snapshot** — no dumping the diff to a scratch file to re-read, and (for spawning flows) no pasting the full diff into the reviewer's brief. A snapshot pays the diff's tokens once per copy and goes **stale** the moment a fix commit lands — a re-review consulting it reviews history, not the branch. The brief carries *pointers* (repo path, base, head SHA, in-scope file list, the driving intent); the diff itself lives in exactly one context: the reviewer's, pulled live.
- **Pin the tree.** The brief names the head SHA the verdict must bind to; verify `git rev-parse HEAD` matches before reviewing, and re-check after — a tree that moved mid-review invalidates the pass.

## Large diffs

For diffs >500 lines: prioritize files in the change's stated scope, then skim the rest for drift. **Say so in the report.** Pretending to have read every line of a 5000-line diff is worse than saying "I prioritized these files".
