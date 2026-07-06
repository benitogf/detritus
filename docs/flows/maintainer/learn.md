---
description: Learn from candyland telemetry - mine failure signatures across runs/quests/campaigns into audited KB updates
triggers:
  - learn
  - telemetry mining
  - failure signature
  - failure cluster
  - candyland data
  - improve from runs
  - learnings ledger
  - prospective validation
  - self-harness
when: User invokes /learn to improve the KB from accumulated candyland telemetry (past runs/quests/campaigns), not from a live session correction
related:
  - flows/maintainer/grow
  - flows/maintainer/absorb
  - flows/principles/truthseeker
  - flows/build/candyland
---

# /learn — Telemetry-Driven KB Improvement

> ## /learn IS /grow WITH A DIFFERENT SOURCE
>
> `/learn` is to `/grow` what `/dream` is to `/plan`: **same shipping spine, different source.**
> - `/grow` source = the current **session incident** (a correction the user just made).
> - `/learn` source = **candyland telemetry** — the structured record of many past runs/quests/campaigns.
>
> The shipping machinery is IDENTICAL and is **composed by reference, not restated** (this KB forbids
> duplicated prose). `/learn` replaces only `/grow` Steps 1–2 (session signal extraction + compliance
> check) with a **telemetry-mining** stage. Steps 3–6 of `/grow` run verbatim.

---

## Overlap boundary with /grow (read first)

- `/learn` == `/grow` with **candyland telemetry as the source**, not session incidents.
- **No duplicated machinery.** The generalize / prefer-editing / confirm / ship logic lives in `/grow`;
  `/learn` points at it. If you find yourself re-writing a `/grow` step here, stop — reference it instead.
- Use `/grow` when the signal is "the user just corrected me this session". Use `/learn` when the signal
  is "across the last N candyland runs, a failure pattern recurs".

---

## Step 0: Workspace Precheck

Same as `/grow` Step 0 — search workspace roots for a local detritus clone (path containing
`github.com/benitogf/detritus` with a `docs/` directory). If not found, warn and STOP; changes cannot be
drafted without the local KB clone.

---

## How to read candyland data (data-access map)

Embed this verbatim; do not stumble on ports/keys. Mining reads structured records only — never scrape logs.

- API `:8888`; UI `:8080`; db at `~/.candyland/db`.
- Live glob reads: `GET http://127.0.0.1:8888/runs/*` · `/quests/*` · `/campaigns/*` · `/audits/*`.
- REST: `GET /api/runs/{id}` · `/api/runs/{id}/trace` · `/api/quests/{id}` · `/api/quests/{id}/runs`
  · `/api/quests/{id}/findings` · `/api/campaigns/{id}` · `/api/campaigns/{id}/quests` ·
  `/api/campaigns/{id}/runs`.
- Adventures = `quests/*` filtered client-side on `convergence == "perFinding"` (no adventures key).
- Remote/WSL: forward BOTH ports; UI on :8080 stays empty until API :8888 is reachable.

The records carry the telemetry fields `/learn` mines (effective model + thinking, token/tool-call
counts, phase durations, review-round counts, verdict outcomes, terminal status + summary,
escalation/decision events, blocker postmortems) — this IS the metrics ledger. Each record also carries
a **`prs[]`** list of the GitHub PR URLs it opened.

### Canonical: resolve a PR → its producing unit

`/learn` accepts a `<unit-id>` OR a `<pr-url>`; a PR argument works standalone exactly like a unit id.
This is the canonical recipe (`/absorb` Phase 2 composes it by reference — don't restate it there):

1. **Footer ref (fast path).** Read the PR body (`gh api repos/<owner>/<repo>/pulls/<n> --jq .body`, per the
   `flows/github/gh` reads-via-`gh api` convention) and parse the candyland footer
   `🍬 Opened by candyland — <kind> \`<id>\`` where `<kind>` ∈ `run` / `quest` / `campaign`. A match gives
   `<kind>`+`<id>` directly — mine that unit (+ children) as the root.
2. **Reverse lookup (fallback + migration path for pre-footer PRs).** No footer → glob the records
   (`GET /runs/*` · `/quests/*` · `/campaigns/*`) and select the unit whose `prs[]` contains the PR URL.
3. **No unit found** → the PR was not candyland-produced; there is no telemetry to mine (a `/learn <pr-url>`
   here reports the miss and stops — `/absorb` routes such a PR to a `/grow`-only leg from the review outcome).

---

## Step 1 (REPLACES /grow Step 1): Telemetry mining — failure signatures

Adapted from arXiv:2606.09498 *Self-Harness* (faithfully — mine signatures, propose minimal audited
edits, validate before adoption; no re-run benchmark, no metered calls). Do NOT overclaim beyond this.

### Build a failure signature per failed record

For each failed run/quest/campaign, extract a **failure signature**:

```
(terminal cause, causal agent behavior, mechanism)
```

- **terminal cause** — the terminal status + summary (e.g. `delivery-failed`, `blocked`, `surfaced-only`).
- **causal agent behavior** — what the agent actually did that led there (from trace/decision events).
- **mechanism** — the underlying *why*. **Symptom ≠ mechanism.** "Test failed" is a symptom;
  "coder claimed done without running the failing path" is a mechanism. Only mechanisms get patched.

### Deterministic clustering

- Cluster the structured records **deterministically** over their fields (terminal status, phase,
  verdict, role, decision/escalation kind). NO embeddings, NO similarity models — group by the
  structured signature keys.

### One bounded LLM attribution pass per failed record

- For each failed record, run **exactly one bounded attribution pass** to name the mechanism from the
  structured signature (batchable across records). This is subscription-only/local-only:
  - ❌ NO metered API calls, NO embeddings, NO fine-tuning.
  - ✅ one bounded pass per record, reading structured fields, producing a mechanism label.

---

## Step 2 (REPLACES /grow Step 2): Evidence bundle → proposed edits (distinct artifacts)

### Evidence bundle (distinct artifact)

Produce an **evidence bundle** — the clustered signatures + record refs proving each mechanism. This is
a **separate artifact** from the proposed doc edits. Do not fold evidence into the edit.

### Proposed edits (each one narrow + audited)

Each proposed edit targets **ONE mechanism** on **ONE named doc surface**, with an audit record:

```
| pattern targeted | doc touched | expected effect | regression risk |
```

### Addressability filter (the paper's central warning)

Before proposing any edit, filter clusters:

- ❌ **Log-only, never patch:** clusters from **model limits**, **flaky env**, or **one-off difficulty**
  are logged to the ledger and NOT turned into doc edits. Patching these accretes generic instruction
  bloat that degrades every future agent — the exact failure mode Self-Harness warns against.
- ✅ **Patchable:** a recurring mechanism attributable to missing/weak KB guidance on a specific doc surface.

✅ cluster: "3 quests terminated `blocked` for a decision the ladder should have absorbed" → patch the
   escalation-ladder doc surface.
❌ cluster: "run failed because the model timed out on a 400-file diff" → log, do not patch.

---

## Learnings ledger (dedup + prospective validation)

Maintain a **learnings ledger** with three states (dedup across invocations so `/learn` doesn't re-propose):

- **adopted** — an edit shipped.
- **rejected** — a cluster reviewed and declined (incl. addressability-filtered), with reason.
- **candidate** — proposed, awaiting confirm/ship.

### Prospective validation (future runs are the held-out split)

- Each **adopted** delta names a **prospective-validation metric**: which telemetry number, on which
  **future** runs, would confirm or refute it (e.g. "`blocked`-terminal rate on quests over the next N
  runs should drop").
- **Validation NEVER re-runs work.** Future runs are the held-out split — you read their telemetry, you
  do not replay past runs. No benchmark re-execution, no metered calls.
- **Promotion/demotion happens on the next `/learn`** invocation: it reads the metrics ledger for the
  runs that occurred since the delta shipped and confirms (keep) or refutes (demote/revert candidate) it.

---

## Steps 3–6 (COMPOSED FROM /grow VERBATIM — do NOT restate)

Once the mining stage has produced the evidence bundle, the addressability-filtered candidate edits, and
the ledger updates, run **`/grow` Steps 3–6 unchanged** (`kb_get flows/maintainer/grow`):

- **Step 3 — Proposed KB Deltas:** generalize past the trigger (encode the *class* of failure the
  mechanism represents, not the one record); prefer editing existing docs over new docs; agent-optimized
  content style.
- **Step 4 — Output Format:** the same signal/compliance/proposed-change/questions layout, with the
  mining signatures + evidence bundle in place of the session signal table.
- **Step 5 — Confirm or Implement:** the same re-confirmation decision rule (implement directly for a
  single unambiguous doc-in-scope delta; confirm for ambiguity / >1 doc / rule conflict).
- **Step 6 — Ship via `/gh`:** route delivery through `/gh` (issue → branch → PR); never commit to the
  default branch; scrub private org/customer/product names (public `benitogf/detritus`).

Do not paraphrase those steps here — read them from `flows/maintainer/grow`.
