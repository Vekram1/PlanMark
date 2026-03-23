# PlanMark Semantic Derivation Policy v0.1

## Status

- Policy ID: `semantic_derivation/v0.1`
- Scope: deterministic rules for deriving Semantic IR from Source IR
- Canonical dependency: `docs/specs/ir-v0.2.md`

## Purpose

Define a mechanical, replayable mapping from Source IR nodes/metadata into task-level semantics without heuristic drift.

Current implementation note:

- The current binary derives tasks from a narrower source-node set than the full target contract described here.
- This policy defines the intended deterministic promotion rules that future parser and context work should implement without heuristic drift.

## Inputs and Preconditions

- Input is canonical Source IR bytes plus explicit policy/version selections.
- Source IR is treated as immutable input for derivation.
- Unknown or unattached metadata remains represented through diagnostics/opaque payloads; it is never silently discarded.
- Structural ownership and scope boundaries must be derived from deterministic source ordering and block-boundary rules rather than free-form natural-language interpretation.

## Deterministic Derivation Rules

1. Node eligibility
- Task candidates come from policy-approved task shapes in source order.
- Checkbox task candidates come from checklist source nodes.
- Heading task candidates come from section headings only when task metadata or explicit task-shape rules promote them.
- Non-task nodes are preserved as non-task semantic entities only when required for references, provenance, or scoped evidence.

1. Task scope formation
- Every promoted task has a deterministic scope boundary.
- Checkbox task scope begins at the checkbox source node and includes directly owned metadata plus structurally owned child content.
- Heading task scope begins at the heading node and extends through its section until the next heading of the same or higher level.
- Blocks inside task scope remain eligible as contextual evidence even when they are not promoted into typed task fields.
- Scope formation must never depend on natural-language cues or unstated formatting conventions.

1. Identity
- Primary identity key is `@id` when present and valid.
- Missing `@id` yields deterministic synthetic identity scoped to node_ref + occurrence index.
- Duplicate explicit IDs emit deterministic diagnostics and preserve deterministic tie handling.

1. Metadata projection
- Known metadata keys map to typed semantic fields (`horizon`, `deps`, `accept`, `why`, `touches`, etc.).
- Unknown keys remain in an opaque metadata bag attached to the semantic entity.
- Metadata ownership is scope-based:
  - metadata directly owned by a checkbox belongs to that checkbox task
  - metadata directly under a heading belongs to that heading task or section scope
  - metadata inside nested list indentation belongs to the nested list item it is structurally scoped under
  - unattached metadata remains unattached with deterministic diagnostics
- Repeated keys use policy-defined merge behavior:
  - repeatable fields (`deps`, `accept`, `touches`) append in stable source order
  - scalar fields (`horizon`, `why`) follow deterministic last-wins with diagnostics on overwrite
- Ambiguous metadata may only be attached when policy-defined ownership rules resolve it deterministically.

1. Dependency graph
- `@deps` references are normalized, de-duplicated, and ordered deterministically.
- Unresolvable dependencies remain represented with diagnostic codes; graph output remains stable.
- Cycle checks are separate validation steps and do not mutate derivation ordering.

1. Horizon/readiness semantics
- `@horizon` is derived into semantic readiness class (`now|next|later`) with explicit defaults when omitted.
- Readiness validation is command/profile policy and not a mutation of derived semantic data.

1. Nested checklist semantics
- Nested checklist items inside a task scope are treated as ordered execution steps by default.
- Nested checklist items do not become independent child tasks unless an explicit policy rule promotes them.
- Default step interpretation preserves structure for retrieval, explainability, and tracker rendering without forcing task explosion.
- If a future policy version promotes child-task shapes, that promotion must be opt-in, deterministic, and versioned.

1. Scoped evidence blocks
- Paragraphs, tables, blockquotes, code fences, and similar non-task blocks inside task scope remain eligible as scoped evidence.
- Scoped evidence may be referenced by context, open, explain, handoff, and tracker rendering layers without requiring those blocks to become standalone tasks.
- Evidence ordering follows source order within the task scope.

1. Ambiguity handling
- Ambiguous structure must be retained rather than guessed.
- If a heading lacks the metadata or shape requirements for task promotion, it remains a non-task structural block.
- If nested content could be interpreted as either child task or evidence, the non-promoting interpretation is the default unless policy explicitly says otherwise.
- Semantic derivation must prefer stable under-promotion over unstable over-promotion.

## Output Guarantees

- Identical Source IR + identical `semantic_derivation_policy_version` produce byte-identical Semantic IR.
- Semantic output ordering is stable:
  - entities in source order
  - metadata fields in canonical encoder order
  - dependency lists in deterministic normalized order
- scoped evidence ordering in source order when emitted
- Derivation emits enough provenance (`node_ref`, source path/range, digests) to map every semantic entity back to source.

## Versioning Rules

- Any change that alters derivation semantics (identity, merge behavior, ordering, defaults) requires a policy version bump.
- Policy version bumps must include migration notes in spec/update docs.
- Multiple policy versions may coexist in registry; command outputs must report the active version.

## Non-Goals

- Heuristic semantic inference not explicitly defined by policy.
- Natural-language interpretation beyond structured metadata + source ordering rules.
- Implicit promotion of every nested checklist item into a tracker-visible child task.
