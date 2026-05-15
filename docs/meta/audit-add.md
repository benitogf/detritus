---
description: Normalize arbitrary feedback (prose, transcripts, pasted reviews, structured output) into entries in a flat-checkbox audit list (review.md by default), with deduping and priority-based sort.
category: meta
triggers:
  - audit add
  - add to audit
  - add to review
  - append to review
  - capture feedback
  - dump feedback into review
  - normalize feedback
  - log review item
when: User has raw feedback in any shape (pasted PR review, transcript snippet, numbered list, prose paragraph, structured JSON) and wants it turned into entries in the working audit file used by `/audit-handle`. Producer side of the same workflow.
related:
  - meta/audit-handle
  - meta/truthseeker
  - meta/gh-issue-create
  - patterns/coding-style
---

# /audit-add — Add entries to an audit / review / feedback list

`/audit-handle` consumes a flat-checkbox audit list one entry at a time. This skill is the *producer*: it takes raw feedback in whatever shape the user pastes in, normalizes it to the audit-list entry format, dedupes against what's already there, and inserts the result so the file stays sorted highest-priority-first.

The working file is `review.md` in the repo root by default — same convention as `/audit-handle`. The user may specify a different filename (`audit.md`, `feedback.md`, or any other) as part of the invocation. If the file does not exist, this skill creates it with the standard header.

## Inputs

Anything. The skill must handle:

- Already-formatted checkbox lines (`- [ ] **Title** — body — file:line`).
- Numbered or bulleted lists with mixed structure.
- Prose paragraphs of feedback (one or many items mixed in flowing text).
- PR review output (e.g., the body of a `gh pr review`, the JSON from `gh api`, or a paste from a review tool).
- Transcript snippets where the actual feedback is interleaved with chatter.
- Single-item drops ("hey also add: X is broken because Y at file:line").
- Structured output from another tool (tables, JSON arrays, YAML).

If the input is truly ambiguous (no clear items can be extracted), stop and ask the user to clarify rather than guessing entries into existence.

## Entry format

Every entry written to the list MUST follow this shape:

```
- [ ] **<short imperative or noun-phrase title>** — <one or two sentence description of the actual problem and why it matters> — <file:line[, file:line, …]>
```

Rules:

- `- [ ]` flat checkbox. No nesting, no sub-items, no headings between entries.
- Title in `**bold**`. Code identifiers inside the title go in backticks (e.g., `**`setLocked` keeps the memory write…**`). Keep it short — one line, no trailing punctuation.
- Em-dash (`—`) separators between title, body, and file:line. Use the actual em-dash character.
- Body describes the *problem and consequence*, not the fix. The fix belongs in the PR opened by `/audit-handle`, not in the entry.
- File:line refs go at the end. Use the same `path/file.go:NN` or `path/file.go:NN-MM` form as existing entries. Multiple refs comma-separated.
- If the source feedback lacks a concrete file:line, write the entry without one rather than fabricating a location. Flag the missing reference in the user-confirmation summary so the user can supply it.

## Priority ordering

The list is sorted highest priority first. When inserting new entries, place each one at the position consistent with this ordering. Default heuristic (highest to lowest):

1. **Correctness bugs that can corrupt state, lose data, or panic.** Races, data loss, panics, unbounded resource consumption, security holes, broken invariants.
2. **Behavior bugs.** Wrong output, missing enforcement of a documented constraint, broken error handling, silent failures.
3. **Resource leaks and performance regressions.** Memory/goroutine leaks, lock contention on hot paths, unbounded buffers without backpressure.
4. **Architectural smells with concrete consequences.** Duplicated logic, missing locks where siblings have them, bypassed middleware.
5. **Code-quality cleanup.** Style, naming, dead code, fragile string concatenation.
6. **Documentation, comments, cosmetic.**

The heuristic is a default — if the user states a priority explicitly ("blocker", "P0", "drop this at the bottom"), honor that over the heuristic. If existing entries imply a finer ordering (e.g., correctness bugs already grouped by subsystem), respect the file's existing structure rather than imposing a global re-sort.

Never silently re-sort entries that already exist. Insertion may reorder around the insertion point, but a wholesale resort of the file requires explicit user confirmation.

## Dedup

Before inserting any new entry, scan the existing list for overlap.

A new entry is a *duplicate* of an existing one if any of these hold:

- Same file:line span and same root cause.
- Same root cause described against different files (e.g., the same regex bug reported twice from two callsites) — flag as a candidate merge, not an auto-skip.
- Title is a paraphrase of an existing title and the body describes the same symptom.

For each detected duplicate:

- If the new wording adds nothing, *skip* the entry. Report it in the confirmation summary as "skipped: duplicate of '<existing title>'".
- If the new wording adds a missing file:line, missing reproduction detail, or a sharper description, *merge*: append the new file:line refs and the new clarifying clause to the existing entry, rather than creating a parallel entry. Report it as "merged into '<existing title>'".
- If the user explicitly asked for the duplicate to be kept (rare, e.g., for tracking two independent reports of the same issue), preserve both — but flag it so they know the dedup heuristic detected the overlap.

## Workflow

1. **Locate or create the working file.** Default `review.md` in the repo root; user may override. If the file does not exist, create it with this header:
   ```
   # <repo> audit — fix list

   Working file. Do not commit or push. Single flat TODO list, sorted highest priority first. Remove entries as they're addressed.
   ```
   Adjust `<repo>` to the actual repo name. If a custom filename was specified, swap "audit" for the appropriate noun (review, feedback).

2. **Parse the raw feedback into candidate entries.** One entry per distinct issue. If the input mixes several issues into one paragraph, split them. If it repeats the same issue in different words, collapse them before dedup against the existing list.

3. **Normalize each candidate to entry format.** Apply the rules in *Entry format* above. Strip code-identifier noise from titles where it harms readability; keep file:line refs precise.

4. **Dedup against the existing list.** Apply the *Dedup* rules. Build a write plan: new entries to insert, entries to merge, entries to skip.

5. **Determine insertion positions.** Apply the priority heuristic to each new entry. For merges, leave the existing entry in place.

6. **Confirm with the user before writing.** Present:
   - Count and titles of new entries to insert (with their proposed positions).
   - Count and details of merges (existing title + what's being appended).
   - Count and titles of skipped duplicates.
   - Any entries missing a file:line ref the user should supply.

   Wait for explicit approval. The user can edit the proposed batch (drop entries, change priorities, supply missing refs) before the write. No silent writes — same convention as `/audit-handle`'s "never silently delete".

7. **Write the file.** Single atomic write to the working file. Preserve the existing header and any existing entries verbatim except for the merges. Do not touch unrelated whitespace or formatting.

8. **Report.** Print the working file path and a one-line summary: `N inserted, M merged, K skipped`. Do not commit, do not push, do not stage — the file is a working file and stays uncommitted by convention.

## What this skill is not

- Not a fixer. It only *records* items. `/audit-handle` is the consumer that opens issues + PRs per entry.
- Not a router. Feedback that's clearly a single issue the user wants fixed *now* (not queued) should go straight to `/gh-issue-create`, not into the list.
- Not a code reader. It does not verify that the cited file:line still matches the described issue — that verification belongs to `/audit-handle` step 2 ("Verify the issue is real and actionable"). Garbage in is allowed; staleness is caught at consume time.
- Not a committer. The working file stays uncommitted. If the user asks to commit it, push back — they likely want a GitHub issue instead.

## Guardrails

- Never fabricate file:line references. If the source lacks them, the entry goes in without — flagged for the user.
- Never silently overwrite or reorder existing entries. Merges append; resorts require explicit approval.
- Never normalize an entry into the list when the input was a single concrete ask the user wants worked *now*. Offer `/gh-issue-create` (or `/gh-issue-work` if an issue already exists) instead.
- Never dedup so aggressively that two genuinely different issues get collapsed. When in doubt, keep both and let the user prune at confirmation time.
- Truthseeker rule applies: if you cannot confidently extract concrete items from the input, stop and ask rather than emitting a low-quality batch.
