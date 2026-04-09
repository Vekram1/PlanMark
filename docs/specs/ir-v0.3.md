# PlanMark IR Specification v0.3 (Draft)

## Status

- Version: `v0.3-draft`
- Scope: Canonical Source IR + Semantic IR structures and determinism requirements
- Related policies:
  - `determinism/v0.1`
  - `semantic_derivation/v0.4`

## IR Model

PlanMark IR is two-layer:

- Source IR: lossless capture of source content, spans, coverage, and provenance metadata.
- Semantic IR: deterministic task/metadata graph derived from Source IR by versioned policy.

Canonical output includes:

- `ir_version`
- `determinism_policy_version`
- `semantic_derivation_policy_version`

## Source IR Requirements

Source IR must preserve:

- path + line-range provenance for captured slices
- canonical slice digest (`sha256`)
- file digest used for compile context
- coverage accounting for all source lines:
  - interpreted coverage
  - explicit opaque/uninterpreted coverage with reason code
- unattached metadata entries retained with deterministic diagnostics

## Semantic IR Requirements

Semantic IR deterministically derives:

- task identity (`@id` when present)
- canonical task completion status
- horizon/readiness metadata
- dependency edges
- acceptance criteria payloads
- deterministic fingerprints for reconcile/change workflows

Semantic IR must never silently drop ambiguous or unknown source intent; unresolved cases remain represented through diagnostics and/or opaque payloads.

Canonical task completion status is plan-owned semantic state. In v0.3 it is derived from `@status` with deterministic normalization:

- `@status done` => `canonical_status = "done"`
- missing or unrecognized values => `canonical_status = "open"`

This field is distinct from tracker runtime overlay state. It exists so the canonical plan can explicitly say whether a task should remain open work or be projected as completed work.

## Canonical Encoding Rules

Canonical encoder behavior is part of the contract:

- UTF-8 JSON output
- lexicographic object key ordering
- deterministic array ordering by policy-defined sort keys
- no dependence on runtime map iteration order
- canonical bytes are the only digest input for object-level IR hashes

## Hashing and Normalization

- Hash algorithm: `sha256`
- Line-ending normalization for slice hashing: `CRLF`/`CR` -> `LF`
- Unicode normalization: preserve source code points in canonical path
- Path normalization:
  - normalize separators to `/`
  - clean `.` and resolve/validate `..` under repo-root confinement rules
  - deterministic case-sensitivity policy with explicit collision diagnostics

## Stable Identity and `node_ref`

`node_ref` is a stable content-addressed handle derived from:

- plan path scope
- node kind
- canonical slice digest
- deterministic occurrence index for duplicates

Line numbers are provenance metadata and are not the sole identity key.

## Versioning and Compatibility

- Any canonicalization/hash/ordering rule change requires a version bump (`ir_version` and/or policy version) and migration notes.
- Backward-compatible migrations are out-of-scope for this initial draft per current milestone non-goals.
