# PlanMark Semantic Derivation Policy v0.4

## Status

- Policy ID: `semantic_derivation/v0.4`
- Scope: deterministic rules for deriving Semantic IR from Source IR
- Canonical dependency: `docs/specs/ir-v0.3.md`
- Prior version: `docs/specs/semantic-derivation-v0.3.md`

## Purpose

Define a mechanical, replayable mapping from Source IR nodes and metadata into task-level semantics without heuristic drift.

## Delta From v0.3

Policy v0.4 makes one material normalization change:

- semantic section bodies now normalize paragraph breaks the same way tracker-neutral projection hashing and rendering already do

This means:

- leading and trailing blank lines in scoped section bodies remain ignored
- repeated interior blank lines collapse to a single blank separator
- single blank lines between paragraphs remain semantic and therefore affect fingerprints

This is a derivation-semantic change because semantic fingerprints now preserve paragraph structure instead of dropping all blank lines from section bodies.

## Inputs and Preconditions

- Input is canonical Source IR bytes plus explicit policy and version selections.
- Source IR is treated as immutable input.
- Unknown or unattached metadata remains represented through diagnostics or opaque payloads; it is never silently discarded.
- Structural ownership and scope boundaries must be derived from deterministic source ordering and block-boundary rules rather than natural-language interpretation.

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
- Blocks inside task scope remain eligible as contextual evidence or scoped semantic content even when they are not promoted into standalone typed entities.
- Scope formation must never depend on natural-language cues or unstated formatting conventions.

1. Identity
- Primary identity key is `@id` when present and valid.
- Missing `@id` yields deterministic synthetic identity scoped to `node_ref` plus occurrence index.
- Duplicate explicit IDs emit deterministic diagnostics and preserve deterministic tie handling.

1. Metadata projection
- Known metadata keys map to typed semantic fields (`status`, `horizon`, `deps`, `accept`, `why`, `touches`, and similar policy-approved fields).
- Unknown keys remain in an opaque metadata bag attached to the semantic entity.
- Metadata ownership is scope-based:
  - metadata directly owned by a checkbox belongs to that checkbox task
  - metadata directly under a heading belongs to that heading task or section scope
  - metadata inside nested list indentation belongs to the nested list item it is structurally scoped under
  - unattached metadata remains unattached with deterministic diagnostics
- Repeated keys use policy-defined merge behavior:
  - repeatable fields (`deps`, `accept`, `touches`) append in stable source order
  - scalar fields (`status`, `horizon`, `why`) follow deterministic last-wins with diagnostics on overwrite
- Ambiguous metadata may only be attached when policy-defined ownership rules resolve it deterministically.

1. Dependency graph
- `@deps` references are normalized, de-duplicated, and ordered deterministically.
- Unresolvable dependencies remain represented with diagnostic codes; graph output remains stable.
- Cycle checks are separate validation steps and do not mutate derivation ordering.

1. Horizon and readiness semantics
- `@horizon` is derived into semantic readiness class (`now|next|later`) with explicit defaults when omitted.
- Readiness validation is command or profile policy and is not a mutation of derived semantic data.

1. Canonical completion status
- `@status` is a plan-owned semantic field.
- Accepted normalized values in v0.4 are:
  - `open`
  - `done`
- Missing `@status` defaults to `open`.
- Unrecognized `@status` values normalize to `open` in the current tolerant implementation.
- Canonical completion status is separate from tracker runtime overlay fields such as assignee, priority, or tracker-local workflow labels.
- Canonical completion status participates in semantic fingerprints.

1. Nested checklist semantics
- Nested checklist items inside a task scope are treated as ordered execution steps by default.
- Nested checklist items do not become independent child tasks unless an explicit policy rule promotes them.
- Default step interpretation preserves structure for retrieval, explainability, and tracker rendering without forcing task explosion.
- If a future policy version promotes child-task shapes, that promotion must be opt-in, deterministic, and versioned.

1. Scoped semantic sections
- Promoted heading tasks may derive deterministic semantic `sections` from scoped freeform body content.
- Section extraction is deterministic and scope-bounded:
  - the promoted heading line itself is excluded
  - task-owned metadata lines are excluded
  - directly owned checkbox steps are excluded from section bodies because they already project into structured `steps`
  - directly owned promoted child tasks are excluded from section bodies because they already project as separate tasks
- Remaining scoped freeform lines are preserved in source order as section body content.
- Empty leading and trailing blank lines are trimmed.
- Runs of repeated interior blank lines collapse to a single blank line.
- A single blank line between non-empty lines is preserved as semantic paragraph structure.
- The current implementation emits a default `details` section for this scoped heading-task body content.
- Section extraction must remain deterministic and must never depend on heuristic summarization.

1. Scoped evidence blocks
- Paragraphs, tables, blockquotes, code fences, and similar non-task blocks inside task scope remain eligible as scoped evidence.
- Scoped evidence may be referenced by context, open, explain, handoff, and tracker rendering layers without requiring those blocks to become standalone tasks.
- Evidence ordering follows source order within the task scope.
- Evidence references and structured sections may coexist: sections carry semantic task body content, while evidence references preserve provenance to scoped non-task nodes.

1. Ambiguity handling
- Ambiguous structure must be retained rather than guessed.
- If a heading lacks the metadata or shape requirements for task promotion, it remains a non-task structural block.
- If nested content could be interpreted as either child task or evidence, the non-promoting interpretation is the default unless policy explicitly says otherwise.
- Semantic derivation must prefer stable under-promotion over unstable over-promotion.

## Output Guarantees

- Identical Source IR plus identical `semantic_derivation_policy_version` produce byte-identical Semantic IR.
- Semantic output ordering is stable:
  - entities in source order
  - metadata fields in canonical encoder order
  - dependency lists in deterministic normalized order
  - section bodies in scoped source order with normalized paragraph separators
  - scoped evidence ordering in source order when emitted
- Derivation emits enough provenance (`node_ref`, source path/range, digests) to map every semantic entity back to source.

## Versioning Rules

- Any change that alters derivation semantics (identity, merge behavior, ordering, defaults, section extraction rules) requires a policy version bump.
- Policy version bumps must include migration notes in spec or update docs.
- Multiple policy versions may coexist in registry; command outputs must report the active version.

## Non-Goals

- Heuristic semantic inference not explicitly defined by policy.
- Natural-language interpretation beyond structured metadata plus source ordering rules.
- Implicit promotion of every nested checklist item into a tracker-visible child task.
- LLM-generated summarization or rewriting in the canonical derivation path.
