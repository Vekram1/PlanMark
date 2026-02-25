# PlanMark Semantic Derivation Policy v0.1

## Status

- Policy ID: `semantic_derivation/v0.1`
- Scope: deterministic rules for deriving Semantic IR from Source IR
- Canonical dependency: `docs/specs/ir-v0.2.md`

## Purpose

Define a mechanical, replayable mapping from Source IR nodes/metadata into task-level semantics without heuristic drift.

## Inputs and Preconditions

- Input is canonical Source IR bytes plus explicit policy/version selections.
- Source IR is treated as immutable input for derivation.
- Unknown or unattached metadata remains represented through diagnostics/opaque payloads; it is never silently discarded.

## Deterministic Derivation Rules

1. Node eligibility
- Task candidates come from checklist/task-like source nodes in source order.
- Non-task nodes are preserved as non-task semantic entities only when required for references/provenance.

1. Identity
- Primary identity key is `@id` when present and valid.
- Missing `@id` yields deterministic synthetic identity scoped to node_ref + occurrence index.
- Duplicate explicit IDs emit deterministic diagnostics and preserve deterministic tie handling.

1. Metadata projection
- Known metadata keys map to typed semantic fields (`horizon`, `deps`, `accept`, `why`, `touches`, etc.).
- Unknown keys remain in an opaque metadata bag attached to the semantic entity.
- Repeated keys use policy-defined merge behavior:
  - repeatable fields (`deps`, `accept`, `touches`) append in stable source order
  - scalar fields (`horizon`, `why`) follow deterministic last-wins with diagnostics on overwrite

1. Dependency graph
- `@deps` references are normalized, de-duplicated, and ordered deterministically.
- Unresolvable dependencies remain represented with diagnostic codes; graph output remains stable.
- Cycle checks are separate validation steps and do not mutate derivation ordering.

1. Horizon/readiness semantics
- `@horizon` is derived into semantic readiness class (`now|next|later`) with explicit defaults when omitted.
- Readiness validation is command/profile policy and not a mutation of derived semantic data.

## Output Guarantees

- Identical Source IR + identical `semantic_derivation_policy_version` produce byte-identical Semantic IR.
- Semantic output ordering is stable:
  - entities in source order
  - metadata fields in canonical encoder order
  - dependency lists in deterministic normalized order
- Derivation emits enough provenance (`node_ref`, source path/range, digests) to map every semantic entity back to source.

## Versioning Rules

- Any change that alters derivation semantics (identity, merge behavior, ordering, defaults) requires a policy version bump.
- Policy version bumps must include migration notes in spec/update docs.
- Multiple policy versions may coexist in registry; command outputs must report the active version.

## Non-Goals

- Heuristic semantic inference not explicitly defined by policy.
- Natural-language interpretation beyond structured metadata + source ordering rules.
