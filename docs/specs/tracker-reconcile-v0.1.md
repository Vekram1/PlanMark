# PlanMark Tracker Reconcile Policy v0.1

## Status

- Policy ID: `tracker_reconcile/v0.1`
- Scope: deterministic reconcile planning between canonical PLAN semantics and tracker runtime state
- Canonical dependencies:
  - `docs/specs/planmark-v0.2.md`
  - `docs/specs/change-detection-v0.1.md`
  - `docs/specs/semantic-derivation-v0.1.md`

## Purpose

Define safe and deterministic reconcile behavior so PLAN edits and tracker state changes can coexist without silent semantic corruption.

## Authority Split

- PLAN (via Semantic IR) is authoritative for:
  - task identity
  - structure/graph semantics
  - canonical intent fields
- Tracker is authoritative only for approved runtime overlays:
  - status
  - assignee
  - priority

Any non-approved tracker fields that conflict with PLAN semantics must not overwrite canonical plan meaning.

## Reconcile Inputs

- Current Semantic IR + semantic fingerprints
- Prior/local sync manifest state (last projection hashes and mapping metadata)
- Current tracker snapshot (for known mapped items)
- Active policy version

## Deterministic Operation Classes

Reconcile planning emits deterministic operations:

- `create`
- `update`
- `no_op`
- `mark_stale`
- `conflict`

These classes are machine-facing contract surfaces for dry-run/apply behavior.

## Planning Rules (v0.1)

1. Mapping and identity
- Use canonical task identity mapping (primary `@id`) from sync manifests and current semantic state.
- Missing mapping with present semantic task => candidate `create`.
- Mapping with missing semantic task => candidate stale-handling path.

1. Projection delta evaluation
- If projection hash unchanged => `no_op`.
- If projection hash changed and mapped tracker item exists => `update`.
- If runtime safe fields changed remotely and canonical projection unchanged => safe-pull overlay update (non-semantic).

1. Stale/removal handling
- Default deletion policy is `mark-stale`.
- Destructive options (`close`, `detach`, `delete`) require explicit opt-in command/policy flags.
- PLAN removals must never imply hard delete by default.

1. Conflict handling
- Conflicts are explicit when runtime overlay and canonical projection updates cannot be merged under safe-field rules.
- Conflict records include stable identity and reason codes for deterministic retries/resolution.

## Dry-Run / Apply Consistency

- Dry-run and apply must consume the same operation plan.
- Apply may retry transient tracker failures, but retry behavior must not mutate operation classification.
- Operation IDs are stable for journaling/auditability across retries.

## Non-Goals

- Multi-tracker conflict matrices in MVP.
- Implicit destructive tracker mutation defaults.
