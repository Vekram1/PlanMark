# PlanMark Specification v0.2 (Draft)

## Status

- Version: `v0.2-draft`
- Scope: Canonical contract for PLAN authoring + deterministic extraction behavior
- Canonical source: `PLAN.md`

## Purpose

PlanMark defines a deterministic, lossless pipeline from `PLAN.md` into machine-actionable artifacts while preserving freeform authoring ergonomics.

## Core Model

- Canonical source of truth: `PLAN.md`.
- Extraction model: tolerant parse into a two-layer IR.
- Layer 1: Source IR captures verbatim slices, spans, hashes, and coverage.
- Layer 2: Semantic IR deterministically derives task graph semantics from Source IR.
- Strictness boundary: strict execution gating is command-boundary behavior, not global parse behavior.
- Semantic derivation is policy-versioned (`semantic_derivation/v0.1`) and independent from IR schema versioning.

## Authoring Primitives

PlanMark authoring supports mixed Markdown content and task metadata lines.

- Task-like checklist entries: Markdown checkboxes (`- [ ]`, `- [x]`) remain source-visible and preserved.
- Metadata annotations: line-oriented `@key value` forms attached deterministically.
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
- Mixed content (prose, tables, diagrams, code fences, partial/broken sections) is preserved and never silently dropped.

## Tolerance contract

Tolerance is a first-class requirement:

- Parsing is best-effort and should succeed on imperfect plans when possible.
- Every source line must be accounted for as either interpreted slice coverage or explicit opaque/uninterpreted coverage.
- Metadata that cannot be attached deterministically must be preserved in unattached metadata with stable diagnostics.
- Unknown constructs are preserved verbatim; they are not discarded.
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
  - Metadata attachment resolves using deterministic nearest-node rules; tie handling is explicit and stable.
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

## Tracker Authority Split

- PLAN/IR defines structure and intent.
- Tracker systems (for example Beads) own runtime-ish overlays only (status, assignee, priority) under explicit reconcile rules.
- Reconcile planning is deterministic and policy-driven; destructive behavior is opt-in by explicit deletion policy.

## Change and Replanning Principles

- Canonical change truth is semantic diff from deterministic IR + fingerprints.
- VCS diff context is advisory only.
- Identity defaults to `@id`; ID changes are delete+add unless explicit identity-evolution annotations are defined by policy.

## Semantic Derivation Policy Link

- Source of semantic derivation rules: `docs/specs/semantic-derivation-v0.1.md`
- Policy identifier: `semantic_derivation/v0.1`
- Contract: identical Source IR bytes + identical semantic policy version produce byte-stable Semantic IR.

## Machine Protocol Requirements

- Commands exposing machine output emit a stable, versioned JSON envelope.
- Diagnostics use stable code enums and stable ordering.
- Exit code taxonomy is documented and stable for machine branching.

## Non-Goals for This Draft

- JSON Schema publication in this task.
- Full parser backend matrix in this task.
- Multi-tracker behavior standardization in this task.
