---
description: Learn from conversation corrections - distill manual fixes into KB updates
triggers:
  - grow
  - correction
  - you missed
  - wrong approach
  - should have used
  - already exists
  - violated rule
  - improve knowledge
  - feedback loop
when: User invokes /grow after correcting agent behavior, or wants to check last interaction against established KB guidance
related:
  - flows/principles/truthseeker
  - flows/maintainer/optimize
  - flows/maintainer/absorb
  - flows/plan/plan
---

# /grow — Conversation-Driven KB Improvement

> ## CRITICAL: THIS IS A CONVERSATION, NOT AN IMPLEMENTATION
>
> When `/grow` is invoked:
> 1. **DO NOT** call file editing tools (edit, multi_edit, write_to_file)
> 2. **DO NOT** run commands that modify the codebase
> 3. **ONLY** produce: Extraction, Compliance Check, Proposed KB Deltas, and the implementation when the decision rule below permits it.
> 4. **Re-confirmation decision rule:**
>    - The user's correction in the triggering message IS the confirmation. Treat it as the instruction.
>    - Re-ask ONLY when (a) the correction is genuinely ambiguous, (b) it would touch >1 doc in non-obvious ways, or (c) it conflicts with an existing KB rule.
>    - Otherwise — single rule-tightening on a doc already in scope — implement directly. Asking the user to confirm what they just told you to do is the failure mode `/grow` exists to fix; do not reproduce it.

---

## Step 0: Workspace Precheck

Resolve a writable KB checkout per `core/kb-writeback` — it finds a local detritus clone if one is present, else provisions a cached clone (forking when the user lacks write access), so `/grow` ships from any machine with no pre-existing clone required. The only hard stop is a true capability blocker (no `gh` auth), owned by `core/kb-writeback` → Preconditions.

---

## Step 1: Conversation Signal Extraction

Scan the current conversation for these signal types:

| Signal | Detection Cues |
|--------|---------------|
| **self_acknowledged_error** | the *agent's own output* concedes a mistake or a doctrine/flow violation — admission phrase-class (e.g. "you are right", "sorry, I…", "my mistake", "I was wrong", "I should have…") or violation phrase-class (e.g. "I didn't follow {doctrine}", "I ignored /{flow}", "that was against {rule}", "I skipped {step}"). **Acknowledgment ≡ detection** — no user escalation needed. Highest-confidence signal; usually co-occurs with `implicit_redirect`. |
| **explicit_correction** | user says "no", "that's wrong", "don't do that", "stop", "I said", "not like that" |
| **missed_existing** | user points to existing doc/workflow/rule that agent should have used: "already exists", "check the docs", "use /X", "kb_get" |
| **rule_violation** | agent action contradicts established KB guidance (e.g., used sleep in tests, added backwards compat, skipped WaitGroup) |
| **skill_invocation_ignored** | user typed `/<name>` in the triggering message — or in any user message earlier in the turn — but the agent did not invoke that skill. The base-prompt rule is unambiguous ("When users reference a slash command or /<something>, they are referring to a skill. Use this tool to invoke it"), but mid-prose mentions like "referencing /X /Y" or "see /X for the pattern" are easy to read as references-to-look-at instead of invoke-now. Default per the base prompt is INVOKE; reading-as-reference requires explicit "don't run X, just look at it" framing from the user. During Step 1 extraction, list every `/<name>` the user typed and verify a matching `Skill` tool call appears in the same turn. |
| **implicit_redirect** | user silently fixes something agent did wrong (e.g., provides corrected code, rewrites a section) |
| **scope_drift** | agent changed requirements without approval, added unrequested features, or deviated from the task |
| **quality_bar** | user raises or clarifies quality expectations ("these docs are for you not humans", "optimize for retrieval") |

> The full incident trigger taxonomy (self-acknowledgment, PR-blocker gate-miss, user correction, telemetry) and its routing live in `core/ego`; grow owns the in-session correction + self-acknowledgment detection cues above.

For each detected signal, produce a row:

```
| SignalType | Evidence (quote or summary) | FailureMode (short key) | Target Doc | Delta Summary |
```

Apply truthseeker principles: extract what actually happened, not what's comfortable. If the agent made no mistakes, say so — do not fabricate issues.

---

## Step 2: Guidance Compliance Check

Independently of user corrections, check the last interaction against established KB:

1. Call `kb_list()` to get all available docs
2. Identify which docs were relevant to the task (by topic, triggers, keywords)
3. For each relevant doc, call `kb_get()` and check:
   - Did the agent follow `forbidden_actions` / `anti_patterns`?
   - Did the agent satisfy `required_outputs` / `canonical_actions`?
   - Were `detection_phrases` / `triggers` present in the conversation but the doc was never consulted?
4. Output any violations as additional rows in the signal table

---

## Step 3: Proposed KB Deltas

For each failure mode identified:

### Generalize past the trigger
The delta must encode the **underlying rule**, not the specific incident that surfaced it. The triggering case is at most **one illustrative example** — never the framing.
- Ask "what is the *class* of changes that share this failure mode?" and write the rule for that class.
- If the delta only fires for the exact situation that prompted the `/grow`, it is **overfit** — widen it until a reviewer hitting a *different* instance of the same problem still matches.
- An example earns its place only as a parenthetical `e.g.`. If the example is doing the load-bearing work, the rule above it is too narrow — lift the general principle out of it.

### Prefer editing existing docs
- If a relevant doc already covers the topic but is missing the specific guidance, propose an edit:
  - Target file path in `docs/`
  - Section to add/modify
  - Exact content to add (optimized for agent detection, not human readability)

### New doc only when
- The failure mode is cross-cutting (applies to multiple docs)
- Or it's a repeated pattern with no natural home in existing docs
- Propose: doc name, description, triggers, and content outline

### Content style requirements
- Write for agent retrieval: keywords, detection phrases, "if X then Y" rules
- Include anti-patterns with concrete examples
- Keep prose minimal — prefer structured lists and tables
- Add to frontmatter: triggers, detection cues, related docs

---

## Step 4: Output Format

```
## Workspace
[detritus clone path or warning]

## Signals Detected
| SignalType | Evidence | FailureMode | Target Doc | Delta Summary |
|------------|----------|-------------|------------|---------------|
| ... | ... | ... | ... | ... |

## Compliance Check
| Relevant Doc | Consulted? | Violations Found |
|-------------|------------|-----------------|
| ... | ... | ... |

## Proposed KB Changes
### [target doc or new doc name]
- Type: edit / new
- Section: [where]
- Content: [draft, agent-optimized]
- Rationale: [why this prevents recurrence]

## Questions
1. [Confirm proposed changes]
2. [Clarify scope if ambiguous]
```

---

## Step 5: Confirm or Implement

Apply the re-confirmation decision rule from the top of this skill:

- **(a) ambiguous correction, (b) >1 doc touched in non-obvious ways, or (c) conflicts with existing rule** → present the plan and wait for user to approve all / approve selectively / request modifications / reject.
- **Otherwise** → the triggering correction is the approval; implement the minimal KB delta directly in the local detritus clone.

Do not produce a second confirmation loop when the first message already settled the question.

---

## Step 6: Ship the KB change via /gh

KB deltas are not done when written to the working tree — they ship like any other change. After implementing (Step 5), route delivery through `/gh` so the change lands as a tracked issue + PR rather than a loose edit on the default branch.

- The detritus clone's edits are uncommitted on the default branch. Per the `/gh` conventions, never commit to the default branch — `/gh` (→ `gh-issue-create` then `gh-issue-work`) opens the issue, branches, commits, and opens the PR.
- Hand `/gh` a one-line description of the KB change (which doc, what rule tightened, what failure it prevents). The issue/PR bodies stay product-focused per the `/gh` conventions; the recurrence-prevention rationale from the signal table is the issue's "why".
- **Carry the authorization forward.** When the user told `/grow` to ship the change (or invoked `/grow` with a shipping directive in the same breath as the correction), that is explicit authorization for the full create-issue-and-open-PR flow — pass it through to `/gh` so the sub-skills don't re-ask (see `flows/github/gh` Phase 2's authorization-propagation rule). If the user only asked `/grow` to *write* the KB delta and said nothing about shipping, the `/gh` confirmation gates stay live — let them fire.
- This is public `benitogf/detritus` — scrub any private org/customer/product names from the issue, PR, branch, and commit bodies before shipping.
- If the user declines `/gh` (wants the edit left in the tree), stop after Step 5 and say so. Default is to ship.
