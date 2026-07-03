---
description: Pre-PR security gate — predict the Aikido CI security verdict locally, offline, before pushing: compute the merge-base diff, run each mapped open-source scanner scoped to only the changed files/deps, print a per-scanner per-severity green/red matrix under Aikido threshold semantics, then fix the in-scope findings before the PR opens. Scoped to the merge-base diff, never a whole-repo scan.
triggers:
  - aikido-guard
  - aikido guard
  - aikido
  - aikido check
  - will aikido pass
  - security gate
  - predict aikido
  - preflight security
  - security scan before pr
  - pre-pr security scan
  - scan my changes for security
when: A change is about to become a PR (interactively or inside an autonomous build/audit loop) and the user wants to know whether the Aikido CI security check will pass — and to fix what would make it red — using local open-source scanners run offline against just the merge-base diff, without waiting on Aikido's cloud scan.
related:
  - flows/github/gh
  - flows/github/gh-self-review
  - flows/github/gh-pr
  - flows/build/smith
  - flows/build/forge
  - flows/build/janitor
  - core/completion
  - core/review-rigor
  - flows/principles/truthseeker
---

# /aikido-guard — Predict the Aikido verdict locally, then fix what's red

`/aikido-guard` is the **pre-PR security gate**: it scans the branch's own changes and predicts the verdict the Aikido bot would post, so the issues Aikido would flag get fixed before the PR opens. It is the security companion to `/gh-self-review` — that audits mechanics and correctness with a fresh agent; this runs the vendored scanners over the diff. Both run in the pre-PR path, and neither posts anything. It fires interactively and inside the autonomous build/audit loops (`/smith`, `/forge`, `/janitor`) right before a PR is opened. The rule sets, scanners, and offline databases are all provisioned by the offline installer [`scripts/aikido-install.sh`](../../../scripts/aikido-install.sh) from a locally-staged bundle — the Opengrep ruleset is a pinned upstream mirror (`plugins/aikido-rules/PINNED_COMMIT`) populated into the gitignored `plugins/aikido-rules/rules/` (not committed; it carries secret-detection test fixtures that trip push protection). Nothing here reaches an external service or uploads the diff.

Aikido's CI check runs in the cloud after you push: it scans the branch, diffs the findings against the base scan, and fails the build when the **new** issues introduced by the branch include anything at or above a configured **minimum severity threshold**. This skill predicts that verdict *before* the push — offline, on your machine, from the merge-base diff — so you fix what would turn the check red instead of discovering it on the PR.

It is a **predictor**, not the authoritative scan. It runs the same *classes* of scanner Aikido runs (via their open-source equivalents), scoped to only what the branch changed, and applies the same threshold semantics. Where it and Aikido disagree, Aikido wins — but the disagreements are bounded and named below (see *Fidelity & limits*), and the skill is tuned to err toward surfacing a finding rather than hiding one.

Nothing here touches the network on the hot path: scanners run against local databases only. A scanner whose tool or offline DB is missing is reported as **not-run** — never silently counted as green. A predicted-green verdict means "every mapped scanner ran and cleared"; it never means "some scanners couldn't run."

## What this is not

- Not the Aikido scan of record — the cloud check still runs on push and is authoritative.
- Not a full-repo audit — it scans the diff's blast radius (changed files + changed deps), matching Aikido's "new issues introduced by this change" gate, not its full inventory.
- Not `/gh-self-review` — that's a correctness/design audit by a fresh agent over the diff. This is the security-gate predictor. Run both before a PR; they don't overlap.

## Phase 0: Track progress

Initialize a `TodoWrite` list mirroring Phases 1–7 and keep it current. This flow has real fan-out (one run per mapped scanner) and a fix loop; the todo list is how the user sees which scanners ran and what's still red.

## Phase 1: Resolve base and compute the merge-base diff

Aikido gates on issues **introduced relative to the base**, not on everything present. So the scope is the diff from the *merge base*, not from the base tip — otherwise commits that landed on base after you branched would show as your findings.

```bash
# Base branch: upstream tracking → origin/HEAD → main
upstream=$(git rev-parse --abbrev-ref --symbolic-full-name @{u} 2>/dev/null)
base=$(echo "$upstream" | sed 's|^origin/||')
base=${base:-$(git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed 's|^refs/remotes/origin/||')}
base=${base:-main}

# Merge base — the commit the branch actually forked from.
# Prefer origin/<base> if present (matches what CI sees), else local <base>.
baseref=$(git rev-parse --verify --quiet "origin/$base" || echo "$base")
mb=$(git merge-base HEAD "$baseref")

# The introduced set: files the branch added or modified since the fork point.
git diff --name-only --diff-filter=ACMR "$mb" HEAD    # A=added C=copied M=modified R=renamed
```

Notes:

- Use `git diff "$mb" HEAD` (two-dot from the merge base) — this is exactly the three-dot `git diff "$baseref"...HEAD` set, i.e. what the branch introduced. Do not use `git diff "$baseref" HEAD` (two-dot from the tip): that folds base-side movement into your scope and inflates the finding count with issues you didn't introduce.
- Include **uncommitted** work when the user is checking before commit: also run `git diff --name-only HEAD` and `git ls-files --others --exclude-standard`, union it into the introduced set, and say in the report that uncommitted files were included. Aikido only ever sees pushed commits, so flag this as "ahead of what CI will scan."
- `--diff-filter` excludes pure deletions (`D`) — a removed file introduces no scannable content. But a *dependency* removal still matters for SCA/license (see Phase 2 dep extraction), so read lockfile deletions from the diff separately.
- Stop with "nothing introduced vs `<base>` — predicted green (nothing to scan)" if the introduced set is empty.

## Phase 2: Classify the diff — which scanners fire, and extract changed deps

Each mapped scanner only fires if the diff contains files in its class. Route every introduced path to zero or more scanners:

| Class | Fires scanner | Matches (illustrative) |
|---|---|---|
| Source code | SAST | `*.go *.py *.js *.ts *.tsx *.rb *.java *.php *.c *.cpp *.cs *.rs *.kt *.scala` |
| Any text file | Secrets | every introduced file (secrets hide anywhere — configs, `.env`, notebooks, docs) |
| Dependency manifest/lockfile | SCA + Malware | `go.mod go.sum package.json package-lock.json yarn.lock pnpm-lock.yaml requirements*.txt poetry.lock Pipfile.lock Gemfile.lock pom.xml build.gradle* Cargo.lock composer.lock` |
| Go source/modules | Go vulns | `*.go go.mod go.sum` (govulncheck runs a call-graph analysis over the changed Go packages) |
| IaC / config | IaC misconfig | `*.tf *.tfvars` `*.yaml/*.yml` under k8s/helm, `Dockerfile*` `docker-compose*.yml` `*.bicep` CloudFormation templates |
| Container | Container/base-image | `Dockerfile*` (base image + declared installs) |

**Changed-dependency extraction.** For SCA/Malware, don't re-scan the entire lockfile's universe — Aikido gates on *newly introduced or bumped* dependencies. Diff the lockfile to get the delta:

```bash
# The set of dependency lines the branch added/changed, per changed manifest:
git diff "$mb" HEAD -- go.sum package-lock.json requirements.txt ... | grep '^+' | grep -v '^+++'
```

Use that delta to filter scanner output in Phase 5: a vuln on a dependency the branch didn't touch is base-inventory, not introduced, and must not turn the verdict red (report it separately as "pre-existing, not gated"). A vuln on an added/bumped dependency is introduced and gated.

Record, per scanner: **fires? (yes/no)**, and the scoped input (file list or dep delta). A scanner that doesn't fire is reported as **n/a (no matching files)** — distinct from not-run and from green.

## Phase 3: Map scanners to local tools and confirm availability (offline)

Aikido is a meta-scanner built on open-source engines. Map each Aikido scanner to its local equivalent and confirm the tool + its offline database are present. Prefer the tool Aikido itself wraps so the findings track.

| Aikido scanner | Local tool | Offline invocation (scoped) | Offline DB requirement |
|---|---|---|---|
| SAST | Opengrep | `opengrep scan --error --json --metrics=off --config <dir> <files>` | rules provisioned locally by the installer — point `--config` at `plugins/aikido-rules/rules/` (gitignored, populated at `PINNED_COMMIT`); never fetch a config over the network |
| Secrets | Gitleaks | `gitleaks detect --no-banner --report-format json --redact` (or `git diff \| gitleaks stdin`) | none (rules are built in) |
| SCA (deps) | osv-scanner | `osv-scanner --offline --format json --lockfile <manifest>` | local OSV DB (`osv-scanner --download-offline-databases` fetched earlier) |
| Malware in deps | osv-scanner (malicious advisories) | same run as SCA; filter advisories tagged malicious | same OSV DB |
| Go vulns | govulncheck | `GOFLAGS=-mod=mod govulncheck -json ./...` scoped to changed Go packages | offline Go vuln DB mirror (`GOVULNDB=file://<db-dir>`) |
| IaC misconfig | Checkov | `checkov -d <dir> --compact --quiet -o json` | bundled policies (no DB) |
| Container | Checkov on Dockerfile | `checkov -f <Dockerfile> --framework dockerfile -o json` | bundled policies |

Availability check, per firing scanner: `command -v <tool>` **and** the offline DB present. If either is missing → mark the scanner **not-run (tool: `<x>` / offline DB missing)** and continue. Do **not** fall back to an online scan silently — offline is a hard constraint of this skill (metered/network calls are out of bounds; local-only). If the user wants to install what's missing, point them at the offline installer [`scripts/aikido-install.sh`](../../../scripts/aikido-install.sh): run `scripts/aikido-install.sh --check` to preflight which scanners and DBs are staged, then `scripts/aikido-install.sh` to provision them from the staged bundle (`vendor/aikido-bundle`, or `AIKIDO_BUNDLE_DIR`). The bundle of scanner binaries and mirrored DBs is staged by the operator per host (it is not committed to the repo — binary scanner blobs and vuln DBs don't belong in version control), which is what keeps provisioning fully offline. Name the exact missing tool/DB; don't run the installer for them without asking.

`--redact` / no-raw-secret flags are mandatory on the secrets scanner: never let a detected secret land in scanner output you'll read back or paste. Describe the class and location, never the value.

## Phase 4: Run each firing scanner, scoped, offline (parallel)

Run the firing, available scanners concurrently — independent tools over disjoint file sets. Each run is scoped to **only the introduced files / dep delta** from Phase 2, never the whole tree. Capture JSON output per scanner; keep the raw output so Phase 5 can map severities faithfully and Phase 6 can cite file:line.

Guardrails on the runs:

- Scope is the Phase-2 file list, passed explicitly as arguments — do not let a tool default to recursing the repo root.
- All runs use the offline flags from Phase 3. If a tool ignores its offline flag and reaches for the network, treat the run as **not-run**, not green.
- A scanner that exits non-zero *because it found issues* is a successful run with findings — parse the JSON. A scanner that exits non-zero because it *crashed* (bad args, missing DB) is **not-run** — surface stderr.

## Phase 5: Map severities and apply threshold semantics → the verdict matrix

Each tool has its own severity taxonomy. Normalize every finding to Aikido's four levels — **critical / high / medium / low** — then apply the threshold.

Normalization (map toward Aikido's scoring; when a tool gives a CVSS score, bucket by it):

| Tool severity | → Aikido level |
|---|---|
| CVSS ≥ 9.0 / tool "CRITICAL" | critical |
| CVSS 7.0–8.9 / tool "HIGH" / Opengrep `ERROR` | high |
| CVSS 4.0–6.9 / tool "MEDIUM" / Opengrep `WARNING` | medium |
| CVSS < 4.0 / tool "LOW" / Opengrep `INFO` | low |
| Any detected secret (verified or not) | critical |
| Malicious-package advisory | critical |

**Threshold.** Read the gated threshold from the repo's Aikido config if present (`.aikido/*`, `aikido.yaml`, or a documented team default); otherwise default to **high** and say so in the report. The gate is: for each scanner, it is **RED** if that scanner produced ≥1 *introduced* finding at a severity **≥ threshold**; otherwise **GREEN**. Findings below threshold, and findings on untouched dependencies (Phase 2), are reported but do **not** flip the verdict — exactly as Aikido's minimum-severity gate on new issues behaves.

Print the matrix — scanner × severity counts, plus the per-scanner and overall verdict:

```
Aikido verdict prediction — <branch> vs <base>  (threshold: high)
merge-base <mb-short>, <N> files introduced, <D> deps changed

scanner        crit  high  med  low   verdict
SAST              0     1    3    0    ● RED     (≥1 ≥high)
Secrets           0     0    0    0    ○ GREEN
SCA               1     0    2    0    ● RED     (≥1 ≥high)
Malware           0     0    0    0    ○ GREEN
Go vulns          0     0    1    0    ○ GREEN
IaC               —     —    —    —    n/a (no matching files)
Container         —     —    —    —    not-run (checkov not installed)

OVERALL: ● RED — 2 scanners over threshold; 1 not-run (verdict provisional)
```

Verdict rules for the OVERALL line:

- Any firing scanner RED → **RED**.
- All firing scanners GREEN **and** none not-run → **GREEN**.
- All firing scanners GREEN but ≥1 **not-run** → **GREEN (provisional)** — state plainly that a not-run scanner could still turn Aikido red; a provisional green is not a pass.

Below the matrix, list each gating finding as one line: `severity — scanner — file:line — rule/CVE — one-phrase description`. Findings below threshold and pre-existing-dependency findings go in a separate "Reported, not gated" block so the user sees them without the verdict counting them.

## Phase 6: Fix the introduced findings, in scope

A red prediction is not the deliverable — a green branch is. For every **gated** finding (the ones flipping the verdict), fix it in this change, then re-run its scanner:

- **SAST**: fix the flagged code (sanitize the input, parameterize the query, drop the weak primitive). A finding you can fix with the same tools in the same diff is in scope — fix it, don't defer it (per `core/completion`: handle-able in-scope work is done now, not parked).
- **Secrets**: remove the secret from the code, rotate it if it was ever committed/pushed, and move it to the secret store / env. A committed secret is compromised regardless of later removal — say so and route rotation.
- **SCA / Malware / Go vulns**: bump the dependency to a fixed version; if none exists, that's the rare genuinely-out-of-scope case → file a tracked issue via `/gh-issue-create` and note it in the report. A malicious package is never "accept the risk" — remove it. For a govulncheck hit, confirm the vulnerable symbol is actually reachable before flagging (call-graph analysis is the point of the tool).
- **IaC / Container**: fix the misconfiguration (pin the base image, drop the privileged flag, set the missing security control).

The disposition test is the same as the rest of the `gh-*` family and `core/completion`: *"is this out of scope for this change?"* — not *"is it a blocker?"* If you could fix it in this diff, it's in scope. Only a fix that genuinely belongs to separate work (no upstream fix exists yet, a repo-wide policy change) is deferrable, and it gets a tracked issue, not a loose mention.

## Phase 7: Loop until green, then report

After fixes, **re-run Phases 1–5** — the diff moved, so the introduced set and every scanner's scope moved with it. Fixing one finding can introduce another (a dependency bump pulls a new transitive vuln; a sanitizer import adds a flagged call). Re-scan the whole matrix, not just the scanner you touched.

Stop conditions:

- OVERALL is **GREEN** (all firing scanners cleared, none not-run) → report green.
- The only remaining gated findings are **genuinely out of scope** (no upstream fix; separate policy work) → each has a tracked issue; report **RED (deferred, tracked)** and name the issues.
- Loop has run 3 iterations → surface the persistent finding to the user for accept/defer/escalate rather than looping forever.

Final report: the verdict matrix, what was fixed, any tracked deferrals, and one explicit sentence on fidelity — that this predicts the Aikido check and the cloud scan on push is authoritative. If any scanner was **not-run**, the headline verdict stays **provisional** and the report says which scanner and why.

## Fidelity & limits — where this and Aikido diverge

State these in the report whenever they bear on the verdict; don't let a green prediction overstate its confidence.

- **Reachability / auto-triage.** Aikido de-duplicates findings and down-ranks unreachable ones. Raw open-source scanner output does none of that, so this predictor tends to be **more red** than Aikido, not less — a predicted-green is a strong signal; a predicted-red may include findings Aikido would auto-triage away. Err toward surfacing: show the finding, note it *may* be auto-triaged.
- **Introduced-set approximation.** Scoping to changed files/deps approximates Aikido's base-diff. It can miss a case where a bumped dependency introduces a vuln that only manifests in an *unchanged* file's usage — the dep delta (Phase 2) catches the dependency, so the vuln is still gated, but the call-site won't be in the SAST scope. Acceptable: the SCA finding still flips the verdict.
- **Threshold source.** If the repo's real Aikido threshold isn't discoverable, the default (**high**) may differ from the team's setting — the report names the threshold it used so the user can correct it.
- **Not-run ≠ green.** Repeated for emphasis because it's the one way this skill could lie: a missing tool or DB is reported, never counted as a pass.
- **Offline DBs age.** A local OSV / Go vuln DB fetched weeks ago misses recent advisories; Aikido's is current. Note the DB's fetch date if the tool exposes it; a stale DB is a source of false green.

## Guardrails

- Never claim GREEN when any firing scanner was not-run — that verdict is GREEN (provisional) at best.
- Never touch the network on the scan path — offline flags are mandatory; a tool that reaches out is treated as not-run. Local-only, no metered calls.
- Never scope a scanner to the whole repo — the introduced set from Phase 1/2 is the scope; whole-repo output would count base-inventory findings as introduced.
- Never use `git diff <base> HEAD` (base tip) for scope — use the merge base, or base-side movement inflates your findings.
- Never print or paste a detected secret's value — redact at the tool, describe the class and location only. A secret that was ever pushed is compromised; route rotation, don't just delete.
- Never defer a gated finding you can fix in this diff — fix it (per `core/completion`); only genuinely-out-of-scope findings get a tracked issue.
- Never treat a malicious-package advisory as an acceptable risk — remove the package.
- Never present this verdict as the authoritative Aikido result — the cloud check on push is authoritative; this predicts it.
- Never invent a threshold — read it from repo config, or default to high and say so.
- Never post or comment — this predicts the verdict; the cloud check on push is what posts. It runs in the pre-PR path alongside `/gh-self-review`, neither of which touches the PR.
