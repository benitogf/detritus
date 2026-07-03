---
description: How to read and act on Aikido Security findings — the finding taxonomy mapped onto the review-rigor classes, the diff-scoped rule that keeps a scan from becoming a whole-repo cleanup mandate, the game-fairness carve-out that stops math/rand in simulation code from being triaged as a weak-crypto bug, and the offline CSV/JSON export normaliser that feeds those findings into the janitor triage loop without any network calls. Do not invoke directly.
triggers:
  - aikido
  - aikido security
  - aikido finding
  - aikido scan
  - sast
  - sca
  - weak random
  - insecure randomness
  - math/rand
  - crypto/rand
  - offline export
  - findings normaliser
  - security findings
when: Loaded via kb_get by any flow that ingests an Aikido (or comparable SAST/SCA/secrets) report — a /janitor security-audit tick, a /gh-pr review over a branch that tripped a scan, or a triage of a dashboard finding — to classify the finding, scope the fix to the diff, and apply the game-fairness randomness carve-out before flagging.
related:
  - core/review-rigor
  - core/completion
  - flows/github/gh-pr
  - flows/github/gh-self-review
  - flows/build/janitor
---

# Acting on Aikido findings

[Aikido Security](https://aikido.dev) is the SAST / SCA / secrets / IaC scanner wired into these repos. It emits findings against the whole tree on every push. This doc is how a reviewer or a loop turns that raw feed into correct, scoped action — it does **not** replace the review analysis in `core/review-rigor`; it maps Aikido's categories onto that rigor and adds the two rules a scanner can't know: scope the fix to the diff, and don't flag legitimate game-fairness randomness.

Load this doc by name via `kb_get` when a flow needs it; never paraphrase the taxonomy or the carve-out into an inline `Agent` prompt — the same anti-pattern `core/review-rigor` names for the review rubric applies here (a paraphrase silently drops the classes and the carve-out, so it re-flags what this doc exempts).

## Finding taxonomy — Aikido category → review-rigor class

Every Aikido finding maps onto a class the reviewer already knows how to reason about. Triage by mapping first, then apply the cited review-rigor check — don't invent a parallel rubric.

- **SAST · injection** (SQL / shell / path traversal) → `review-rigor` Security ("Unvalidated input at system boundaries") + R8 on any serve path. Prove the tainted source reaches the sink in the *assembled* program, not just in the flagged line.
- **SAST · insecure randomness** (`math/rand` where `crypto/rand` is expected) → Security, **but** run the game-fairness carve-out below FIRST. Most hits in these repos are simulation/animation code, not token/secret generation.
- **SAST · weak crypto / hardcoded key** → `review-rigor` R7 (forgeable or default keys) + Security. A default or checked-in key is a blocker with evidence: cite the constant and the verify path.
- **SAST · XSS / active content** → R8 on the serve/render side. Accept-without-neutralize (`image/svg+xml`, `text/html`) is stored-XSS.
- **Secrets · token/credential in source or logs** → Security ("Secrets checked in, tokens in logs"). Never quote the secret when flagging — point at the line, describe the class, and treat rotation as part of the fix.
- **SCA · vulnerable dependency** → `review-rigor` "Classify scope · Deps-only": confirm the vulnerable path is actually reached (grep for usages), check whether a bump is a version *jump*, and verify the advisory applies to the used API surface, not just the package.
- **IaC / config · exposed surface** → R1b/R1c wiring + "Hidden coupling": a permissive default, an open port, a debug endpoint. Trace it to the deploy reality, not the manifest in isolation.

An Aikido finding is a **claim**, exactly like a PR body (`review-rigor` "Verify the change's claims"). A scanner reports the pattern, not the reachability. Prove the tainted path is live before flagging, and prove it's dead before dismissing — a dismissal is itself a positive claim (`review-rigor` V1: no speculative clears).

## Diff-scope the fix (don't let a scan become a repo-wide mandate)

Aikido scans the **whole tree**; a single push's report includes findings in code the diff never touched. Scope discipline (`core/completion`'s three dispositions) governs which ones you act on:

- **In the diff, or made reachable by the diff** → fix now. This is in-scope work; deferring handle-able in-scope findings is the silent-deferral failure `core/completion` forbids.
- **Pre-existing, untouched by the diff, and this isn't the natural place to fix it** → out of scope. Don't fold an unrelated legacy finding into the PR (it dilutes the diff and stalls the review). File it per `core/completion` / `/gh-issue-create` if it's real, and move on — mirror `review-rigor`'s "Pre-existing instances … are out of scope — drop them, don't downgrade them to non-blockers."
- **Introduced OR made visible by the diff** (the change touched adjacent lines, moved the code, or newly routes to it) → in-scope blocker, fix now.

The bar is the same as `review-rigor` "Scope discipline": in-scope cleanup the change skipped is a blocker; out-of-scope legacy findings are not this PR's job. A scan's whole-tree noise floor is not a license to spray unrelated fixes across the repo, nor an excuse to skip the ones the diff owns.

## Game-fairness carve-out — `math/rand` is not always a bug

Aikido flags `math/rand` as insecure randomness because it's cryptographically weak. That verdict is correct for tokens, session IDs, nonces, password/salt generation, and any secret — those **must** use `crypto/rand`, and a `math/rand` hit there is a real blocker.

It is **wrong** for game-fairness and simulation code, where weak-but-fast, **seedable/reproducible** randomness is the requirement, not a defect:

- **Provable fairness needs a reproducible seed.** Game outcomes (a wheel spin, a shuffle, a deal) are audited by replaying a recorded seed and confirming the same result. `crypto/rand` is unseedable by design, so it *cannot* produce the deterministic, verifiable stream a fairness proof depends on. `math/rand` with an explicit `rand.New(rand.NewSource(seed))` is the correct tool.
- **Simulation and Monte-Carlo runs** (`simulate`-style helpers that estimate house edge / RTP over millions of rounds) want speed and reproducibility, never cryptographic strength.
- **Animation, jitter, non-security sampling** — cosmetic randomness that gates nothing.

Files that fall in this carve-out include the game engines and their simulators — e.g. `wheel.go` (wheel-spin outcome), shuffle/deal logic, and any `simulate`/`simulation` harness. When Aikido flags `math/rand` in one of these:

1. Confirm the value never becomes a secret, token, key, or authorization decision — trace the sink (`review-rigor` "Trace the lived path"). If it feeds auth or a credential, the carve-out does **not** apply — it's a real blocker.
2. Confirm the stream is explicitly seeded for reproducibility (a per-round `crypto/rand` re-seed of a `math/rand` source is a legitimate hybrid: unpredictable seed, replayable stream).
3. If both hold, **dismiss the finding as a fairness/simulation carve-out** and record why in the triage note. Suppress it in Aikido (or annotate the line) so the same false positive doesn't re-open every push.

Detection cue for the *real* bug this carve-out is careful not to hide: `math/rand` whose output lands in a token, cookie, key, OTP, or any comparison that grants access. That is never a fairness case — it's the weak-randomness blocker, and the carve-out must not shelter it.

## Offline export normaliser — feeding findings into the loop without network calls

Aikido Security is scanned from its dashboard, not wired into CI here. To feed
its findings into the janitor triage loop without any network calls, we pull
the findings list as a manual **offline export** and normalise it locally.

### Flow

1. In the Aikido dashboard, export the findings list as **CSV** or **JSON**.
2. Drop the file into `scripts/aikido/scratch/` — this directory is
   **gitignored** (raw exports may carry finding details, so they never land in
   git).
3. Normalise it:

   ```sh
   python3 scripts/aikido/normalise_export.py scripts/aikido/scratch/export.csv
   # or pipe: some-cmd | python3 scripts/aikido/normalise_export.py -
   ```

The normaliser prints a JSON array of canonical findings.

### Canonical finding shape

Every record — regardless of the export format or the dashboard's current
column/key names — is mapped onto:

| field      | type          | notes                                             |
| ---------- | ------------- | ------------------------------------------------- |
| `file`     | string        | source path of the finding                        |
| `line`     | int \| null   | `null` when the export omits a line               |
| `rule`     | string        | rule / check id                                   |
| `severity` | string        | folded to `critical`/`high`/`medium`/`low`/`info` |

The CSV and JSON exports of the same findings normalise to identical output.

Key aliasing is tolerant (e.g. `File Path`, `filePath`, `path`, `location` all
map to `file`; `Rule ID`, `checkId`, `check`, `type` map to `rule`), so the
normaliser survives dashboard field drift.

### Test

```sh
cd scripts/aikido && python3 -m unittest test_normalise_export -v
```

The test runs the normaliser against the CSV and JSON fixtures in
`scripts/aikido/testdata/`, asserts every finding carries
`file`/`line`/`rule`/`severity`, and checks the scratch dir is gitignored via
`git check-ignore`.
