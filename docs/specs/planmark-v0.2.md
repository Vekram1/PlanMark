# PlanMark Specification v0.2 (Draft)

## Status

- Version: `v0.2-draft`
- Scope: Canonical contract for PLAN authoring + deterministic extraction behavior
- Canonical source: `PLAN.md`
- Note: this draft includes the intended richer Markdown authoring contract; current implementation is narrower until parser, semantic-derivation, and context code catch up.

## Purpose

PlanMark defines a deterministic, lossless pipeline from `PLAN.md` into machine-actionable artifacts while preserving freeform authoring ergonomics.

## Core Model

- Canonical source of truth: `PLAN.md`.
- Extraction model: tolerant parse into Markdown-derived planning structure.
- Source capture preserves verbatim slices, spans, hashes, and coverage.
- Semantic derivation promotes only policy-approved task shapes and metadata into machine-actionable task graph semantics.
- Free-form Markdown remains preserved even when it is not promoted into typed task semantics.
- Strictness boundary: strict execution gating is command-boundary behavior, not global parse behavior.
- Semantic derivation is policy-versioned (`semantic_derivation/v0.1`) and independent from IR schema versioning.

## Authoring Primitives

PlanMark authoring supports mixed Markdown content, task-shaped blocks, and line-oriented metadata.

- Task-shaped checklist entries: Markdown checkboxes (`- [ ]`, `- [x]`) remain source-visible and preserved.
- Task-shaped section headings may define task scope when paired with task metadata or an explicit semantic promotion rule.
- Metadata annotations use line-oriented `@key value` forms attached deterministically to a task or section scope.
- Canonical metadata keys include (initial set):
  - `@id`
  - `@horizon`
  - `@deps`
  - `@accept`
  - `@why`
  - `@touches`
  - `@non_goal`
  - `@risk`
  - `@rollback`
  - `@assume`
  - `@invariant`
- Unknown metadata keys are retained as opaque annotations unless strict policy explicitly rejects them.
- Nested checklist items may exist inside a task scope and are preserved as ordered execution detail unless a semantic policy explicitly promotes them to child tasks.
- Mixed content (prose, tables, diagrams, code fences, partial/broken sections) is preserved and never silently dropped.

Implementation note:

- The current parser implementation now recognizes scope boundaries for heading sections and checkbox blocks, and metadata attachment is partially scope-aware for those shapes.
- The current binary also supports a conservative first semantic pass for the richer model: headings with explicit task metadata can be promoted to tasks, and nested checkbox items default to semantic steps rather than standalone tasks.
- The implementation is still narrower than the full target contract below: it does not yet model a general structural Markdown layer or capture all nested block types as first-class structural entities.

## Task Shapes And Scope

The canonical authoring model recognizes two task shapes.

- Checkbox task: a checklist item plus directly owned metadata and child content.
- Heading task: a heading promoted by task metadata or explicit semantic rule, plus descendant content within the heading boundary.

Task scope rules:

- Checkbox task scope begins at the checkbox line and includes directly attached metadata plus nested child blocks that belong to the item.
- Heading task scope begins at the heading and extends until the next heading of the same or higher level.
- Free-form blocks inside task scope remain part of task provenance/context even when they are not promoted into typed semantic fields.
- The same task scope may contain paragraphs, tables, blockquotes, code fences, examples, and nested lists without losing deterministic ownership.

This model keeps Markdown authoring natural while giving context, sync, and explain flows a larger bounded unit than a single line.

## Metadata Ownership Contract

Metadata ownership is scope-based, not purely line-adjacent.

- Metadata directly under a checkbox task belongs to that checkbox task.
- Metadata directly under a heading belongs to that heading task or section scope.
- Metadata inside nested list indentation belongs to the nested list item it is structurally scoped under.
- Metadata before the first attachable task/section remains unattached and must be preserved with a deterministic diagnostic.
- Ambiguous metadata may only be attached when policy-defined ownership rules resolve it deterministically; otherwise it remains unattached.

Expected merge behavior:

- Repeatable keys append in stable source order: `@deps`, `@accept`, `@touches`.
- Scalar keys use deterministic last-wins behavior with overwrite diagnostics when policy requires them: `@horizon`, `@why`, and similar single-value fields.

The contract for authors is simple: keep machine-relevant metadata inside the task scope it belongs to.

## Nested Checklist Contract

Nested checklist items provide execution detail without forcing every step to become an independent tracker item.

- A nested checklist inside a task scope is interpreted as ordered execution detail by default.
- Nested checklist items are preserved as structured scoped content for context retrieval and issue rendering.
- A future semantic policy may explicitly promote certain nested checklist shapes into child tasks, but that promotion must be opt-in and versioned.
- Top-level task semantics must not depend on heuristic interpretation of arbitrary nested prose.

This default prevents accidental task explosion while still preserving enough structure for richer context packets and tracker rendering.

## Planning-first authoring

Planning-first authoring keeps PLAN writing cheap while preserving deterministic extraction.

- Only a task shape plus optional `@id`/`@horizon` are required.
- A task shape may be either a checklist item or a heading with task metadata or explicit promotion rule.
- Additional metadata (`@accept`, `@deps`, `@touches`, `@why`, and others) can be added incrementally as scope stabilizes.
- Free-form prose and evidence blocks may live inside task scope without losing provenance.
- Nested checklist items should be treated as steps by default, not forced child tasks.
- Missing metadata for `next`/`later` work is surfaced as diagnostics, not parse failures.
- Strict execution gating still applies only when generating L0 execution packets for `horizon=now`.

## Tolerance contract

Tolerance is a first-class requirement:

- Parsing is best-effort and should succeed on imperfect plans when possible.
- Every source line must be accounted for as either interpreted slice coverage or explicit opaque/uninterpreted coverage.
- Metadata that cannot be attached deterministically must be preserved in unattached metadata with stable diagnostics.
- Unknown constructs are preserved verbatim; they are not discarded.
- Richer Markdown structure may be captured without requiring every block type to receive task semantics.
- Canonical commands (`compile`, `doctor`, `context`, `open`, `explain`, `sync`) must remain deterministic and offline-safe.

## Determinism Policy v0.1

Policy identifier: `determinism/v0.1`

This policy defines what "deterministic" means for PlanMark compile and read paths.

- Hash algorithm: `sha256` for canonical payloads, source slices, and content-addressed references.
- Canonical JSON:
  - UTF-8 output.
  - Stable object key ordering (lexicographic).
  - No non-deterministic map iteration at encoding boundaries.
  - Arrays preserve policy-defined order only; filesystem/OS iteration order is never trusted.
- Source text normalization for hashing:
  - Line-ending policy: normalize `CRLF` and bare `CR` to `LF` before slice hashing.
  - Unicode policy: preserve raw code points (no implicit NFC/NFD rewrite in canonical path).
- Path canonicalization for policy checks and manifest references:
  - Normalize separators to `/`.
  - Clean `.` segments.
  - Reject escaping repo root via `..`.
  - Case-sensitive matching by default; case-collision diagnostics are explicit where needed.
  - Symlink resolution policy is explicit and versioned per command surface.
- Traversal/attachment ordering:
  - Source node traversal order follows source order.
  - Metadata attachment resolves using deterministic scope-ownership rules; tie handling and ambiguity retention are explicit and stable.
- Stable source identity:
  - `node_ref` is content-addressed from plan scope + node kind + canonical slice digest + deterministic occurrence index.
  - Line ranges remain provenance, not identity.
- Version pinning:
  - Canonical outputs include `ir_version` and `determinism_policy_version`.
  - Any canonicalization/hash behavior change requires a version bump and migration note.
- Reproducibility guarantee:
  - Identical input bytes + identical policy versions + identical effective config produce byte-identical canonical JSON and digests.

## Horizon and Readiness Rules

`@horizon` controls readiness semantics by intent horizon:

- `now`: execution-ready only when required fields pass policy.
- `next`: trackable with warnings for underspecification by default.
- `later`: intent-level allowed with minimal readiness requirements by default.

L0 execution packet default requirements for `@horizon now`:

- required `@id`
- at least one `@accept`
- resolvable `@deps`
- resolvable required L0 pin/invariant references when configured

## Strictness Profiles (Boundary-Oriented)

- `Loose`: tolerant extraction; warnings prioritized.
- `Build`: enforce graph and syntax sanity while remaining tolerant to future work detail gaps.
- `Exec`: enforce strict L0 execution readiness for `horizon=now`.
- `CI` (future): repository-configured policy promotion (warn->error) with explicit versioning.

## Strictness Matrix (v0.1)

This matrix makes strictness explicit at command boundaries rather than parser entry.

| Command | Loose (default) | Build | Exec | CI (future profile hook) |
| --- | --- | --- | --- | --- |
| `plan compile` | Tolerant parse; preserve unknowns and unattached metadata with diagnostics; produce IR whenever possible | Same tolerance; may elevate structural graph issues to stronger diagnostics without dropping source coverage | Same as Build for extraction behavior | Repo-configurable severity promotion only; no nondeterministic behavior |
| `plan doctor` | Report issues with warnings for underspecified `next/later` work | Enforce ID uniqueness, dep resolvability, cycle checks, metadata shape sanity | Includes Build checks plus execution-readiness framing for `horizon=now` | Policy-driven warn->error promotion with versioned config |
| `plan context --level L0` | Allowed, but blocks execution packet generation for `horizon=now` tasks missing readiness requirements | Same as Loose with stronger diagnostics on missing/invalid readiness fields | Strict gate: requires `@id`, `@accept`, resolvable `@deps`, and required L0 references | Same as Exec plus repo policy overlays |
| `plan context --level L1/L2` | Tolerant packet generation with freshness/diagnostic signaling | Same | Same extraction tolerance; strictness remains L0-only for execution gating | Same |
| `plan open` / `plan explain` | Always retrieval/diagnostic focused; never mutates canonical state | Same | Same | Same |
| `plan sync <adapter> --dry-run` | Deterministic reconcile planning; non-destructive preview | Same with stronger conflict/drift surfacing | Same | Same with repo policy severity mapping |
| `plan sync <adapter>` apply | Uses same deterministic operation plan as dry-run; destructive operations remain opt-in by deletion policy | Same | Same | Same with explicit policy gates |

Notes:
- Parsing tolerance is invariant across profiles; profiles mainly control diagnostic severity and execution/apply gating.
- Strict execution gating is intentionally limited to L0 `horizon=now` packet semantics.

## Tracker Authority Split

- PLAN/IR defines structure and intent.
- Tracker systems (for example Beads) own runtime-ish overlays only (status, assignee, priority) under explicit reconcile rules.
- Reconcile planning is deterministic and policy-driven; destructive behavior is opt-in by explicit deletion policy.

## Change and Replanning Principles

- Canonical change truth is semantic diff from deterministic IR + fingerprints.
- VCS diff context is advisory only.
- Identity defaults to `@id`; ID changes are delete+add unless explicit identity-evolution annotations are defined by policy.

## Change Detection Policy Link

- Source of change detection rules: `docs/specs/change-detection-v0.1.md`
- Policy identifier: `change_detection/v0.1`
- Contract: deterministic semantic change classifications come from semantic fingerprints/IR deltas; VCS data is hint-only.

## Tracker Reconcile Policy Link

- Source of tracker reconcile rules: `docs/specs/tracker-reconcile-v0.1.md`
- Policy identifier: `tracker_reconcile/v0.1`
- Contract: PLAN remains canonical for structure/intent; tracker runtime fields are merged under explicit safe-pull rules.
- Tracker rendering now flows through a tracker-neutral task projection layer before adapter-specific payloads are built.
- Tracker adapters expose deterministic capability descriptors so rendering/template policy can validate backend support for body text, steps, child work, custom fields, and safe runtime overlays.
- Built-in rendering profiles are deterministic named policies (`default`, `compact`, `agentic`, `handoff`) layered on top of the tracker-neutral projection.
- Future adapter-local template names, if introduced, are expected to be deterministic aliases over those built-in profiles plus adapter-local field layout choices; they must not become arbitrary user-authored text templates in the canonical sync path.
- The current proven adapters are `beads`, a GitHub Issues proof adapter, and a Linear proof adapter.
- Current Beads projection payloads expose the Beads-rendered subset of that projection layer, including `horizon`, ordered `dependencies`, ordered `steps`, and ordered `evidence_node_refs`.
- The GitHub and Linear proof adapters render deterministic markdown issue title/body payloads from the same tracker-neutral projection and render-profile layer.
- Sync planning hashes the canonical semantic tracker-neutral projection, so reserved semantic fields for future adapters, such as scoped `sections` and evidence `kind`, still participate in change detection even before the Beads renderer consumes them directly.
- Provenance remains part of the rendered/audited tracker payload, but provenance-only movement such as line-range churn or source slice re-addressing does not by itself force routine tracker updates for a stable task identity.
- The staged adapter roadmap is now:
  - markdown-issue adapters first (`linear`, `jira` phase 1 basic issue rendering)
  - then agentic trackers with native machine-oriented child work (`ticket`, `trekker`, `beans`)
  - then field-heavy enterprise adapters (`jira` phase 2 extended custom-field mappings)

## Semantic Derivation Policy Link

- Source of semantic derivation rules: `docs/specs/semantic-derivation-v0.1.md`
- Policy identifier: `semantic_derivation/v0.1`
- Contract: identical Source IR bytes + identical semantic policy version produce byte-stable Semantic IR.
- The semantic policy defines deterministic promotion for checkbox tasks, heading tasks, scope-owned metadata, nested checklist steps, and scoped evidence retention.

## Machine Protocol Contract (v0.1)

- Commands exposing machine output emit a stable, versioned JSON envelope.
- Diagnostics use stable code enums and stable ordering.
- Exit code taxonomy is documented and stable for machine branching.
- Core exit codes:
  - `0`: success
  - `1`: validation/readiness failure
  - `2`: usage/flag error
  - `3`: internal error

## AI Helper Boundary (Non-Canonical)

- `plan ai ...` commands are assistive and non-canonical.
- Canonical commands (`compile`, `doctor`, `context`, `open`, `explain`, `sync`) must remain deterministic and offline-safe.
- `plan ai suggest-fix` may derive prompt/repair proposals from deterministic diagnostics, but it must not mutate `PLAN.md` implicitly.
- Any apply flow (`plan ai apply-fix`) must require explicit approval, emit a reviewable delta/patch proposal, and be re-validated via deterministic commands.
- If a command only previews a fix proposal, output must explicitly state that `PLAN.md` was not mutated.

### AI Provider Config Contract (`.planmark.yaml`)

AI provider selection is repository-local configuration and applies only to `plan ai ...` commands.

Supported mapping:

```yaml
ai:
  provider: openai_compatible
  model: gpt-4o-mini
  base_url: http://127.0.0.1:8080/v1
  api_key_env: PLANMARK_AI_KEY
  timeout_seconds: 30
```

Rules:
- Unknown keys under `ai:` are rejected deterministically.
- AI config values contribute to effective config hashing for reproducibility diagnostics.
- Canonical commands remain unaffected by AI provider configuration.
- Provider selection precedence for `plan ai apply-fix`:
  1. explicit CLI flags
  2. `.planmark.yaml` `ai:` values
  3. built-in defaults (no provider configured => local proposal-only behavior)

### Tracker Sync Config Contract (`.planmark.yaml`)

Tracker adapter and render-profile selection are repository-local configuration.

Supported mapping:

```yaml
tracker:
  adapter: beads  # or: github, linear
  profile: default
```

Rules:
- Unknown keys under `tracker:` are rejected deterministically.
- Tracker config values contribute to effective config hashing for reproducibility diagnostics.
- Selection precedence for `plan sync`:
  1. explicit CLI flags (`--adapter`, `--profile`)
  2. positional sync target (`plan sync beads`, `plan sync github`, `plan sync linear`)
  3. `.planmark.yaml` `tracker:` values
  4. built-in defaults (`adapter=beads`, adapter default render profile)
- Explicit CLI adapter selection must not conflict with an explicit positional target.
- Current repository-local tracker selection is `adapter` + `profile`.
- Future adapter-local template names, if added, must resolve deterministically to an adapter plus a built-in profile-compatible render policy.

## Non-Goals for This Draft

- JSON Schema publication in this task.
- Full parser backend matrix in this task.
- Full multi-tracker standardization beyond the current proven adapters and staged roadmap.
