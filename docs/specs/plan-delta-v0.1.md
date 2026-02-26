# Plan Delta Schema v0.1

## Status

- Version: `v0.1`
- Schema identifier: `plan_delta/v0.1`
- Scope: deterministic proposal/apply payload for controlled PLAN.md replanning

## Purpose

Plan Delta formalizes changes to canonical plan content as explicit, versioned operations.

This prevents silent drift by requiring deterministic preconditions before a delta can be applied.

## Top-Level Envelope

A valid Plan Delta document contains:

- `schema_version` (required, `v0.1`)
- `base_plan_hash` (required, canonical hash of PLAN.md bytes used to derive the delta)
- `created_by` (optional producer identifier)
- `created_at` (optional RFC3339 timestamp)
- `operations` (required, ordered list of deterministic edit operations)

## Deterministic Preconditions

Each operation must include source preconditions so apply is safe after concurrent edits.

Required per operation:

- `op_id` (stable operation identifier within delta)
- `kind` (`insert|replace|delete|move|metadata_upsert|metadata_delete`)
- `target` (object describing deterministic resolution target)
- `precondition` object containing:
  - `source_hash` (required hash of expected source slice)
  - `source_range` (required `{start_line,end_line}`)

`precondition.source_hash` and `precondition.source_range` are mandatory in `v0.1`.

## Target Resolution

`target` must resolve deterministically using canonical identities:

- preferred: `node_ref`
- optional contextual keys: `task_id`, `path`

Resolution policy:

1. Resolve `node_ref` when provided.
2. If `node_ref` missing, resolve by other keys only when unambiguous.
3. Ambiguous or unresolved targets fail with deterministic diagnostics.

## Operation Body

All operations include:

- `op_id`
- `kind`
- `target`
- `precondition`
- `payload` (kind-specific data)

Examples of `payload` fields:

- `insert`: `text`, `position`
- `replace`: `text`
- `delete`: none
- `move`: `destination`
- `metadata_upsert`: `key`, `value`
- `metadata_delete`: `key`

## Apply Semantics

Apply is deterministic and ordered:

1. Verify `base_plan_hash` matches current PLAN bytes.
2. For each operation in order:
   - resolve `target`
   - verify `precondition.source_range`
   - verify hash of current target slice equals `precondition.source_hash`
   - apply mutation
3. If any precondition fails, stop and return structured failure.

No implicit fuzzy patching is allowed in `v0.1`.

## Error Model

Implementations should produce machine-readable errors with stable codes for:

- `base_hash_mismatch`
- `target_unresolved`
- `target_ambiguous`
- `precondition_range_mismatch`
- `precondition_hash_mismatch`
- `operation_kind_unsupported`

## Canonicalization

For deterministic hashing and signatures:

- JSON must use canonical encoding (sorted keys, UTF-8, stable arrays)
- `operations` order is semantically significant and must be preserved
- whitespace-only formatting changes outside operation payloads must not alter semantics

## Security and Non-Goals

- Delta payloads are data only; they do not execute commands.
- `v0.1` does not auto-apply without explicit command invocation.
- Interactive merge/conflict resolution UI is out of scope.
