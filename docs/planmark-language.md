# PlanMark Language Reference (v0.1)

PlanMark is a **deterministic, lossless** “plan annotation language” embedded inside Markdown.
It is intentionally **not** a general-purpose programming language.

The core idea:

- `PLAN.md` may contain **any** free-form Markdown (prose, tables, diagrams).
- PlanMark only acts on explicitly marked **actionable blocks**.
- Everything else is preserved **verbatim** in the lossless IR but is not executed/projection-relevant.

This doc defines:

1) what PlanMark can represent (“capabilities”)
2) how to write it (syntax + types)
3) what is validated strictly vs tolerated
4) what the toolchain does with it (doctor / context packets / beads projection)

---

## 1. Capability Summary

PlanMark v0.1 supports these primitives:

- **Tasks / Beads**: define work items with stable IDs
- **Dependencies**: explicit `deps` lists
- **Acceptance criteria**: explicit checklists
- **Invariants**: “must hold” constraints
- **Budgets**: time/cost/token limits (descriptive; enforcement via `plan doctor`)
- **Commands**: suggested commands to run (never run implicitly)
- **Context pins**: pointers to text ranges in `PLAN.md` for context packets
- **Projection**: deterministic mapping into Beads (lossy target) with provenance retained in PlanMark IR

Non-goals in v0.1 (by design):

- no conditionals, loops, arithmetic, or “execute on parse”
- no implicit inference (“guessing”) of tasks from prose
- no automatic execution of commands
- no cross-file ref resolution unless explicitly enabled by a directive

---

## 2. The “Two-Layer” Model

### Layer A — Free-form Markdown (unrestricted)
Anything in `PLAN.md` outside PlanMark blocks is treated as raw text:

- preserved verbatim into `plan.json` (lossless IR)
- eligible for context packets (L0 verbatim slices)
- ignored for projection and validation gating

### Layer B — Actionable PlanMark blocks (structured + validated)
Only fenced code blocks labeled `planmark` are parsed as PlanMark:

```md
```planmark
id: bead-123
title: Deterministic parser
deps: [bead-101]
acceptance:
  - Parses all planmark blocks
  - Produces lossless IR with provenance
```
```

If it isn’t inside a `planmark` block, it’s not actionable.

---

## 3. Syntax

### 3.1 Block form
PlanMark blocks are fenced code blocks with the info string `planmark`:

```md
```planmark
<body>
```
```

Everything inside `<body>` is parsed using a **YAML-like subset** defined below.
This is a *format*, not “YAML the full language.”

### 3.2 Allowed value types (v0.1)
- **String**: `title: Something`
- **Number**: `budget_hours: 6`
- **Boolean**: `done: false`
- **List**:
  - inline: `deps: [bead-1, bead-2]`
  - multiline:
    ```md
    deps:
      - bead-1
      - bead-2
    ```
- **Map**:
  ```md
  budgets:
    dev_hours: 6
    token_budget: 120000
  ```
- **Multiline string** (`|`):
  ```md
  notes: |
    This is preserved exactly.
    Newlines remain newlines.
  ```

### 3.3 Comments
Lines starting with `#` are comments **inside PlanMark blocks**.
They are preserved in the lossless IR but ignored semantically.

### 3.4 Unknown keys (lossless rule)
Unknown keys are allowed and preserved.  
`plan doctor` may warn on unknown keys if strict mode is enabled, but parsing remains lossless.

---

## 4. Core Entities

In v0.1, every block is one “item.” The item’s `kind` controls semantics.
If `kind` is absent, it defaults to `task`.

### 4.1 `task` (default)
Represents a unit of work suitable for Beads projection.

**Recommended fields**
- `id` (string): stable identifier (`bead-123`, `task-foo`, etc.)
- `title` (string): short human title
- `summary` (string): 1–3 sentence intent
- `deps` (list[string]): IDs this task depends on
- `acceptance` (list[string]): acceptance criteria checklist
- `invariants` (list[string]): constraints that must remain true
- `budgets` (map): descriptive budgets (doctor can gate)
- `commands` (list[string]): suggested commands to run (never auto-run)
- `artifacts` (list[map]): output files/paths expected

**Example**
```md
```planmark
kind: task
id: bead-123
title: Implement tolerant parser
summary: Parse planmark blocks without failing on unknown keys; preserve provenance.
deps: [bead-101]
acceptance:
  - Parses all planmark blocks in PLAN.md
  - Preserves verbatim content + line ranges
  - Produces plan.json deterministically
invariants:
  - No implicit execution
  - No deletion commands proposed without approval
budgets:
  dev_hours: 6
commands:
  - go test ./...
  - golangci-lint run
artifacts:
  - path: plan.json
    description: Lossless IR with provenance
```
```

### 4.2 `section`
Used to group multiple tasks under a labeled heading in IR.

```md
```planmark
kind: section
id: sec-parser
title: Parser + IR
```
```

### 4.3 `context_pin`
Pins a slice of `PLAN.md` for context packets.
Pins are explicit and deterministic: they reference file + line ranges.

```md
```planmark
kind: context_pin
id: pin-overview
file: PLAN.md
lines: [12, 47]
label: High-level overview used in L1 context packets
```
```

---

## 5. Determinism Rules (Hard Invariants)

PlanMark’s toolchain must be deterministic. That means:

1) **No guessing**: tasks are only recognized inside `planmark` blocks.
2) **Stable IDs**:
   - If `id` is provided, it is the canonical identifier.
   - If `id` is omitted, tools MAY generate a derived ID based on (file path + block start line + block hash),
     but this should be treated as less stable than explicit `id`.
3) **Canonicalization**:
   - Whitespace normalization is limited to what is required to parse fields.
   - Raw text is preserved exactly for provenance.
4) **No hidden execution**:
   - `commands` are documentation + checklists only.
   - Tools never run commands unless explicitly invoked by the user.

---

## 6. Validation and “Plan Doctor”

`plan doctor` is a validator + gatekeeper, with two modes:

### 6.1 Tolerant parse (always)
- Always produces lossless IR (`plan.json`) if the markdown is readable.
- Preserves unknown keys and raw block text.

### 6.2 Strict gating (optional)
In strict mode, doctor can fail the run if:
- required fields for actionable projection are missing (e.g., missing `id` and strict IDs enabled)
- `deps` reference unknown IDs (if strict dep checking enabled)
- invalid types are provided (`deps` not a list, etc.)
- policy violations occur (repo invariants / budgets / forbidden operations)

Doctor outputs:
- **errors** (block projection/execution)
- **warnings** (allowed but risky / non-idiomatic)
- **notes** (informational)

---

## 7. Beads Projection Semantics

Projection is deterministic and one-way (PlanMark IR remains source of truth).

### 7.0 What happens to a `planmark` block (end-to-end)

At a high level, projection is a *pure transform*:

1) **Extract**: scan `PLAN.md` for fenced ```planmark blocks (and only those).
2) **Parse (tolerant)**: parse each block into a typed map using the YAML-like subset.
3) **Normalize**: canonicalize fields needed for deterministic rendering (ordering, whitespace rules).
4) **Lossless IR write**: emit `plan.json` containing:
   - raw block text (verbatim)
   - parsed fields (including unknown keys)
   - source provenance (file, start/end line, block hash)
5) **Render bead payload**: construct a Beads “create/update” payload from the normalized item.
6) **Project (API)**: create or update the bead using deterministic idempotency rules (below).

Importantly:
- Projection never mutates `PLAN.md`.
- Projection never “guesses” missing fields from nearby prose.
- Unknown keys are preserved in IR; only recognized fields affect the bead.

### 7.1 Mapping
For each `task` item:
- Bead title := `title`
- Bead body := `summary` + `acceptance` + `invariants` + selected pins
- Tags := derived from `kind`, optional `tags` field
- Dependencies := exported as references (exact behavior depends on Beads features)

### 7.1.1 Deterministic bead body template (recommended)

For consistency, the bead body SHOULD be rendered using a stable template and stable section ordering.
Suggested canonical template (Markdown):

````md
## Summary
<summary or empty>

## Acceptance
- [ ] <acceptance[0]>
- [ ] <acceptance[1]>

## Invariants
- <invariants[0]>
- <invariants[1]>

## Suggested commands (manual)
```sh
<commands[0]>
<commands[1]>
```

## Expected artifacts
- `<artifacts[i].path>` — <artifacts[i].description>

## Context pins
- <pin label> (PLAN.md:L12-L47)

---
PlanMark:
- id: <id>
- kind: <kind>
- source: <file>:L<start>-L<end>
- block_hash: <sha256:…>
````

Rules:
- Sections are omitted if their corresponding field is missing/empty (except the final provenance block).
- Lists preserve input order.
- No automatic checkbox toggling based on external state (projection is pure).

### 7.1.2 Field mapping rules (precise)

The following mapping is normative in v0.1:

- `title`:
  - Required for projection in strict mode.
  - Used as the Bead title verbatim (no inference).
- `summary`:
  - Rendered under **Summary** as-is (multiline preserved).
- `acceptance`:
  - Rendered as a checklist under **Acceptance** in the same order as specified.
- `invariants`:
  - Rendered as bullet list under **Invariants** in the same order as specified.
- `commands`:
  - Rendered under **Suggested commands (manual)** as a shell code block.
  - Explicitly labeled “manual” to prevent implied execution.
- `artifacts`:
  - Rendered under **Expected artifacts**.
  - Unknown fields inside each artifact map are preserved in IR but not rendered by default.
- `deps`:
  - Primary representation is via Beads dependency links (if supported).
  - Always also rendered in the bead body under **Dependencies** if Beads does not support structured deps.
- `tags`:
  - Optional list. If present, used verbatim as bead tags (order preserved).
  - If absent, tags MAY include `kind:<kind>` as a derived tag (configurable).

Any keys not listed above:
- MUST be preserved in `plan.json`
- MUST NOT affect projection unless explicitly added to the mapping in a future language version.

### 7.1.3 Handling missing IDs

Best practice is to require explicit `id` for actionable tasks.
If `id` is missing and strict IDs are disabled, tools MAY derive a deterministic ID:

- `id := pm::<file>::<start_line>::<block_hash_prefix>`

Derived IDs MUST be treated as unstable across edits (line shifts) and should produce a warning.

### 7.1.4 Create vs Update (idempotency)

Projection should be idempotent: running it twice with unchanged input should not create duplicates.

Normative behavior:
- The bead’s canonical identity is the PlanMark `id`.
- On projection:
  - If a bead already exists with `planmark.id == <id>`, UPDATE it.
  - Otherwise, CREATE a new bead.

The “exists” check can be implemented via:
- a dedicated Beads metadata field (preferred), or
- a stable marker in the bead body provenance block (fallback).

### 7.1.5 Drift detection (Beads edits)

Because Beads is a lossy target, PlanMark is source-of-truth.
If a bead is edited in Beads, PlanMark does not auto-merge changes back.

Tools SHOULD detect drift by comparing:
- stored `block_hash` (in bead provenance) vs current `block_hash` for the same `id`
and then:
- report drift as a warning (or error in strict sync mode)
- show a recommended patch to apply to `PLAN.md` (human-reviewed)

### 7.2 Provenance
Every projected bead includes:
- the PlanMark `id`
- source location (file + line range)
- a hash of the original block text (to detect drift)

If a bead is edited in Beads, PlanMark does not auto-merge changes back. Instead:
- tools can **report drift** and recommend edits to `PLAN.md`

### 7.3 Lossiness boundaries (what Beads will not preserve)

Beads projection is intentionally lossy. In v0.1:
- formatting inside lists may be normalized by Beads UI
- unknown keys are not represented in Beads fields unless explicitly rendered in the body
- Beads edits do not round-trip back into PlanMark automatically

To keep the workflow deterministic:
- PlanMark IR (`plan.json`) is the only canonical structured representation
- Beads is a view/task-execution surface, not the source of truth

---

## 8. Examples (copy/paste)

### Minimal task
```md
```planmark
id: bead-001
title: Establish repo skeleton
acceptance:
  - go test ./... passes
  - docs/planmark-language.md exists
```
```

### Task with deps + commands
```md
```planmark
id: bead-010
title: Add plan doctor checks
deps: [bead-001]
commands:
  - go test ./...
acceptance:
  - Doctor reports missing IDs as error in strict mode
  - Doctor preserves unknown keys in tolerant mode
```
```

### Context pin
```md
```planmark
kind: context_pin
id: pin-constraints
file: PLAN.md
lines: [88, 122]
label: Invariants section
```
```

---

## 9. FAQ

### “Is PlanMark a compiler language?”
No. It’s a deterministic annotation format for project plans.

### “Can I keep my PLAN.md messy?”
Yes. Only `planmark` blocks are actionable.

### “Do I have to use every field?”
No. Use the minimum fields needed for your workflow; strict mode determines what is required.

### “How do we avoid guessing?”
By requiring explicit blocks for anything that becomes automation input.
