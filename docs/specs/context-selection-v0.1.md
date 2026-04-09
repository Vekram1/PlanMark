# PlanMark Context Selection Specification v0.1

## Status

- Version: `v0.1-draft`
- Scope: deterministic selection of task context packets for agent work
- Canonical source: `PLAN.md`
- Related specs:
  - `docs/specs/planmark-v0.2.md`
  - `docs/specs/semantic-derivation-v0.2.md`
  - `docs/specs/ir-v0.2.md`
  - `docs/specs/tracker-reconcile-v0.1.md`

## Purpose

Define a deterministic, agent-facing context selection model that replaces raw public context levels such as `L0`, `L1`, and `L2` with task-need based retrieval.

The guiding rule is:

> The agent should request the work it is trying to do, not guess the amount of context it needs.

PlanMark already has a strong retrieval substrate:

- canonical task IDs
- bounded source slices
- deterministic semantic task payloads
- dependency edges
- provenance and source mapping
- file-touch signals such as `@touches`
- acceptance criteria that often reveal required implementation surfaces

This spec turns that substrate into a deterministic context-selection contract.

## Problem Statement

Raw public levels such as `L0`, `L1`, and `L2` are an implementation-shaped interface.

They force an agent to predict:

- whether file evidence will be needed
- whether dependency closure will be needed
- whether the current task can be executed from its own scoped slice

before the agent has enough context to know the answer.

That model has value internally as a hierarchy, but it is not the right external contract. It asks the agent to perform a retrieval policy decision instead of requesting a concrete capability.

The result is avoidable context waste, avoidable misses, and avoidable prompt inflation.

## Product Goal

For any task query and agent need, PlanMark should return the smallest sufficient deterministic context packet.

That packet should:

- be derived without LLM inference
- be bounded by deterministic source ownership and graph rules
- explain why it was selected
- expose whether it is sufficient for the requested need
- expose what additional context would be needed if escalation becomes necessary

## Agent-Facing Needs

PlanMark v0.1 defines the following agent-facing needs:

- `execute`
  - The agent needs enough task-local context to execute or reason about the task itself.
- `edit`
  - The agent needs task-local context plus concrete repo surfaces required to modify code, docs, tests, or other touched files.
- `dependency-check`
  - The agent needs enough dependency closure to reason about ordering, blockers, or upstream semantic constraints.
- `handoff`
  - The agent needs a richer transfer packet suitable for handing work to another agent or resuming work later.
- `auto`
  - PlanMark chooses the smallest deterministic packet that satisfies inferred immediate need under policy rules.

These needs are the public API. They replace raw public selection by `L0`, `L1`, and `L2`.

## Internal Retrieval Classes

The public API is need-based, but the implementation may still use a hierarchical retrieval structure internally.

The internal retrieval classes for v0.1 are:

- `task`
  - task-local scoped source slice plus semantic task fields
- `task+files`
  - `task` plus pinned or touched file evidence
- `task+deps`
  - `task` plus dependency closure summaries
- `task+files+deps`
  - `task` plus both file evidence and dependency closure
- `full-plan`
  - full-plan fallback when smaller deterministic packet construction is insufficient

These classes are implementation details. They are not the primary agent-facing API.

## Contract Rule

`L0`, `L1`, and `L2` remain valid implementation concepts for caching, packet builders, and compatibility paths, but they are no longer the primary public interface for context selection.

The public contract is:

- the caller declares a task and a need
- PlanMark deterministically selects the smallest sufficient internal retrieval class
- PlanMark returns a packet plus machine-readable selection metadata

## Deterministic Selection Policy

Context selection must be deterministic for identical:

- plan bytes
- policy versions
- task ID or node ref
- requested need
- effective config

Selection must not depend on:

- LLM reasoning
- semantic vector similarity
- environmental network state
- non-deterministic filesystem ordering
- user-specific hidden cache state outside the repo-local state directory

## Selection Rules by Need

### `execute`

Default selection target:

- `task`

`execute` should return only task-local context unless deterministic miss conditions require escalation.

Task-local packet content should include:

- task identity
- task title
- horizon
- deps
- acceptance payload
- semantic sections
- steps
- scoped evidence slices or references
- source path and scoped slice provenance

### `edit`

Default selection target:

- `task+files` when deterministic file-surface triggers are present
- otherwise `task`

`edit` should include concrete repo surfaces when the task itself signals that editing is likely to require them.

Deterministic file-evidence triggers include:

- explicit `@touches`
- explicit `@pin`
- acceptance criteria that reference repo paths or concrete artifacts
- task-local structured metadata that names file paths
- future policy-approved file-reference metadata

### `dependency-check`

Default selection target:

- `task+deps` when deterministic dependency reasoning triggers are present
- otherwise `task`

`dependency-check` is for ordering, blocker analysis, and upstream semantic constraints.

Deterministic dependency triggers include:

- non-empty `@deps`
- explicit blocker metadata when added by future policy
- tasks whose readiness or correctness is defined against dependency state

For v0.1, declared `@deps` are sufficient to justify dependency expansion when the requested need is `dependency-check`.

Dependency packets should include dependency summaries, not unrestricted raw recursive dumps by default.

### `handoff`

Default selection target:

- `task`
- escalate to `task+files`, `task+deps`, or `task+files+deps` when deterministic triggers justify the expansion

`handoff` exists because agent transfer usually benefits from a richer packet than immediate execution, but it must still remain bounded and deterministic.

`handoff` should include:

- task-local slice
- semantic sections
- steps
- evidence
- selection metadata
- file evidence when required
- dependency summaries when required

For v0.1, `handoff` should not automatically include dependency closure merely because the task declares `@deps`.

Instead:

- `handoff` should include file-backed evidence when deterministic file triggers are present
- `handoff` should expose `next_upgrade` and `remaining_risks` when dependency semantics are omitted
- dependency closure should remain the primary responsibility of `dependency-check` unless future policy adds stronger handoff-specific dependency triggers

### `auto`

`auto` selects the smallest packet allowed by deterministic inference rules.

For v0.1, the intended behavior is:

- start from `task`
- escalate only when deterministic triggers show that task-local context is insufficient for the inferred immediate operation
- prefer under-expansion over unrestricted expansion
- preserve a clear upgrade path when the packet is not sufficient

`auto` is allowed to infer need classes only from deterministic task signals and command context. It must not rely on LLM inference.

## Miss Conditions and Escalation Rules

Escalation must happen only when deterministic miss conditions are present.

### File-Evidence Miss

A file-evidence miss exists when:

- the requested need is `edit` or `handoff`
- and the task contains deterministic file-surface signals
- and the current packet does not include those repo surfaces

Escalation path:

- `task` -> `task+files`

### Dependency-Closure Miss

A dependency miss exists when:

- the requested need is `dependency-check` or `handoff`
- and the task has deterministic dependency requirements
- and the current packet does not include dependency summaries

Escalation path:

- `task` -> `task+deps`

For v0.1, dependency-closure misses should normally be satisfied only for `dependency-check`, or for future policy-approved handoff cases that explicitly require graph reasoning in the transfer packet.

### Combined Miss

A combined miss exists when:

- both file-evidence and dependency-closure miss conditions are present

Escalation path:

- `task` -> `task+files+deps`

### Full-Plan Fallback

`full-plan` is a last resort.

It should only be selected when:

- task resolution is ambiguous even after deterministic task or node-ref lookup
- policy requires milestone or whole-plan reasoning
- deterministic bounded retrieval cannot satisfy the requested need

`full-plan` must not become the default behavior for ordinary execution work.

## Sufficiency Metadata

Every need-based context response should expose machine-readable sufficiency metadata.

Minimum fields for v0.1:

- `need`
- `selected_context_class`
- `sufficient_for_need`
- `escalation_reasons`
- `included_files`
- `included_deps`
- `remaining_risks`
- `next_upgrade`

This metadata exists for agent control flow, not for decorative human output.

## Output Shape Expectations

The exact CLI envelope may evolve under protocol rules, but the selection model expects packets to report:

- the queried task
- the requested need
- the selected internal class
- the canonical task-local payload
- any expanded file evidence
- any expanded dependency closure
- deterministic sufficiency and escalation metadata

The command must remain machine-actionable and stable under protocol versioning rules.

## Compatibility and Migration

PlanMark may keep compatibility support for:

- `plan context --level L0|L1|L2`
- internal cache keys keyed by retrieval class or legacy level
- pack/export flows that still refer to levels during transition

However:

- legacy levels are compatibility paths
- new behavior should prefer `--need`
- new docs should describe `--need` as primary

During migration, PlanMark may map:

- `L0` -> `task`
- `L1` -> `task+files`
- `L2` -> `task+deps`

That mapping is intentionally approximate and internal. It should not constrain future need-based improvements.

## Caching Contract

Context caching remains valid and desirable, but cache identity should reflect:

- selected need or selected internal class
- plan path
- IR version
- determinism policy version
- semantic derivation policy version
- task identity
- task semantic fingerprint
- node slice hash
- file-surface hashes when included

The cache key must correspond to the selected deterministic packet, not to incidental human phrasing.

## Determinism and Safety Constraints

The context-selection path is canonical behavior and must preserve core PlanMark constraints:

- no implicit network access
- no hidden LLM assistance
- no semantic vector retrieval in the canonical path
- deterministic source and graph traversal
- stable output ordering
- stable packet hashing and cache keys

Selection must remain offline-safe and replayable.

## Non-Goals

For v0.1, the context-selection system does not attempt to do the following:

- embedding or vector retrieval in the canonical path
- hidden summarization or compression by an LLM
- heuristic “smart expansion” based on free-form language guesses
- arbitrary user-authored retrieval templates in the canonical path
- unrestricted repository dumping as the default context response
- replacing explicit task graph semantics with generic repository search

## Recommended Implementation Sequence

1. Add the spec and make the contract explicit.
2. Add a need-based selector that chooses an internal retrieval class deterministically.
3. Add `plan context --need ...` while keeping `--level` as a compatibility path.
4. Return machine-readable selection metadata in JSON output.
5. Move pack/export and related context consumers toward the same need-based model.
6. Deprecate public documentation centered on `L0`, `L1`, and `L2`.

## Acceptance

- `docs/specs/context-selection-v0.1.md` exists and defines:
  - the product goal
  - the agent-facing needs
  - the internal retrieval classes
  - the contract rule that raw levels become implementation details
  - deterministic miss and escalation rules
  - machine-readable sufficiency metadata expectations
