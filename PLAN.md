This plan is anchored in PlanMark’s current contracts for sync and change detection: `PLAN.md` is canonical, tracker sync is a projection layer, the tracker only owns approved runtime overlays, reconcile emits fixed deterministic operation classes, dry-run and apply share the same classification logic, and change detection keys off semantic identity rather than VCS diffs. ([GitHub][1])

# PlanMark planning roadmap

## Add tracker reconcile invariant test matrix

@id pm.reconcile.tests.root
@status done
@horizon now
@accept cmd:test -f docs/specs/tracker-reconcile-test-matrix-v0.1.md

Recent bugs exposed a broader problem in the current test strategy: we have decent component-local tests for compile, projection, planner, and adapter behavior, but not enough explicit seam coverage for how those layers compose over time.

The dangerous bugs are not only "bad function" bugs. They are layer-mismatch bugs:

* manifest memory says one thing while tracker reality says another
* semantic equality is mistaken for tracker correctness
* cleanup and sync use slightly different notions of plan-backed work
* semantic derivation and tracker projection disagree about what counts as task meaning

This initiative defines and implements the practical safety net for reconciliation. The goal is not exhaustive state-space coverage. The goal is explicit invariant coverage for the failure modes that can silently leave Beads out of sync with the canonical plan.

The core contract should become:

> After a successful sync, current plan-backed work converges to current tracker reality, removed work does not remain open, and cleanup cannot permanently break recovery.

Product goals:

* [ ] Define the named reconcile invariants we actually rely on
* [ ] Define scenario classes for source, semantic, projection, manifest, tracker, and cleanup interactions
* [ ] Convert recent real bugs into stable regression categories
* [ ] Make end-to-end convergence behavior a first-class tested surface
* [ ] Reduce the chance that manifest loss or closed tracker state can silently violate current plan intent

Non-goals:

* [ ] No claim of exhaustive state-space coverage
* [ ] No attempt to prove full formal correctness in this initiative
* [ ] No requirement that every reconcile bug must be covered by one giant end-to-end test instead of layered tests

* [ ] Write the test-matrix spec in `docs/specs/tracker-reconcile-test-matrix-v0.1.md`
* [ ] Define the core invariants:

  * current plan tasks converge to open tracker issues
  * removed plan tasks do not remain open
  * semantic task changes propagate to tracker-visible state
  * provenance-only churn does not cause broad tracker churn
  * cleanup does not permanently break sync recovery
  * manifest loss does not prevent eventual convergence
* [ ] Define the scenario classes:

  * canonical intent
  * projection
  * reconcile planning
  * apply and mutation
  * cleanup
  * end-to-end convergence
* [ ] Define the required regression categories for recently discovered bugs
* [ ] Link the tracker reconcile policy spec to the new test matrix

## Add end-to-end reconcile regression coverage

@id pm.reconcile.tests.impl
@status done
@horizon now
@deps pm.reconcile.tests.root
@accept cmd:go test ./internal/cli ./internal/tracker ./internal/syncplanner

Once the matrix is written down, the codebase needs to actually enforce it.

The first implementation pass should focus on the recent failure classes that exposed the seam problem:

* stale tracker issues surviving because they fell out of the manifest
* current plan tasks staying closed because sync treated them as `no-op`
* cleanup closing issues that sync could not restore
* semantic task body changes failing to propagate to tracker-visible content

This section is about turning those bug classes into durable regressions and then expanding coverage to the adjacent invariants around them.

* [ ] Add manifest-loss recovery tests for stale detection
* [ ] Add tests proving current plan-backed issues are reopened when closed remotely or by cleanup
* [ ] Add tests proving removed plan tasks do not remain open after sync
* [ ] Add tests proving cleanup never classifies current plan-backed issues as junk
* [ ] Add tests proving semantic task body changes propagate into tracker-visible output
* [ ] Add tests proving provenance-only churn does not cause unnecessary updates
* [ ] Add tests for duplicate or ambiguous tracker identity surfaces where cleanup or sync must not silently guess
* [ ] Add at least one end-to-end sequence test covering:

  * sync from empty state
  * cleanup
  * sync restore of current tasks
  * plan deletion
  * stale retirement
* [ ] Keep these tests in default CI, not as a manual-only safety net

## Replace level-based context with need-based retrieval

@id pm.context.root
@status done
@horizon now
@deps pm.reconcile.tests.impl
@accept cmd:test -f docs/specs/context-selection-v0.1.md

PlanMark should stop exposing raw context levels like `L0`, `L1`, and `L2` as the main agent-facing interface. Keep the hierarchy internally if it is useful, but make the public interface task-need based, graph-aware, and automatically escalating.

This work should come before the formal verification initiative already in this plan. The context model is part of the core planning product surface, and getting it right affects every agent workflow that depends on PlanMark.
It should also come after the immediate reconcile invariant and regression work above, because sync and cleanup correctness are prerequisite to trusting tracker-backed execution.

The core thesis is:

> The agent should request the work it is trying to do, not guess the amount of context it needs.

The current `L0` / `L1` / `L2` model has a good systems intuition behind it, but it still asks the agent to perform the wrong decision. The agent must currently predict whether concrete file evidence or dependency closure will be required before it has enough information to know that. That is closer to manual memory management than to a useful paging model.

The better model is deterministic selective retrieval with automatic escalation:

* the agent requests a need
* the tool returns the smallest sufficient context packet for that need
* the tool escalates only when policy-defined miss conditions are present
* the packet tells the agent what was selected, why it was selected, and what remains missing

This direction is consistent with the current research trend around coding-agent retrieval and agent memory:

* selective retrieval matters more than indiscriminately stuffing more context into the prompt
* hierarchical memory is useful, but hierarchy should usually remain an internal retrieval structure rather than a user-facing API
* retrieval should be coupled to the reasoning task rather than treated as a one-time static fetch
* graph structure is a stronger retrieval substrate than broad context dumping for repository and task reasoning

PlanMark is especially well-suited to this design because it already has:

* canonical task IDs
* bounded task source slices
* explicit dependency edges
* provenance and source mapping
* file-touch signals like `@touches`
* acceptance criteria that often reveal when concrete repo artifacts are required

Product goals:

* [ ] Make the smallest sufficient context packet the default outcome
* [ ] Eliminate agent guesswork about raw context levels
* [ ] Keep retrieval deterministic and offline-safe
* [ ] Use PlanMark’s task graph and provenance graph as the retrieval substrate
* [ ] Make miss and escalation behavior explicit and machine-readable
* [ ] Reduce the need to read the full `PLAN.md` except for true milestone planning or global ambiguity cases

Non-goals for the first version:

* [ ] No semantic vector search or embedding-based retrieval in the canonical path
* [ ] No hidden LLM summarization in context packet generation
* [ ] No unrestricted “give me everything around this task” expansion mode as the default
* [ ] No requirement that authors annotate every task with exhaustive retrieval hints before the system becomes useful

* [ ] Write the context-selection spec in `docs/specs/context-selection-v0.1.md`
* [ ] State the product goal: smallest sufficient context packet for a given task need
* [ ] State the agent-facing needs:

  * `execute`
  * `edit`
  * `dependency-check`
  * `handoff`
  * `auto`
* [ ] State the internal retrieval classes:

  * task slice only
  * task plus pinned or touched files
  * task plus dependency closure
  * task plus files and deps
  * full plan fallback
* [ ] State the contract rule: raw `L0` / `L1` / `L2` become implementation details, not the primary public API

## Define deterministic context selection rules

@id pm.context.policy
@status done
@horizon now
@deps pm.context.root
@accept cmd:grep -q "selected_context_class" docs/specs/context-selection-v0.1.md

The tool should choose context size deterministically instead of forcing the agent to guess how much context it needs. This section defines the selection policy and the miss or escalation behavior.

The first version should define a small number of agent-facing needs:

* `execute`

  The agent needs enough task-local information to perform the next concrete unit of work for this task. The default should be the smallest task packet that preserves the task’s bounded scope, acceptance criteria, steps, and evidence.

* `edit`

  The agent needs enough information to safely modify repo artifacts. This should start from the task packet and expand only when deterministic file-backed signals require concrete implementation evidence.

* `dependency-check`

  The agent needs enough information to reason about whether the task can be executed safely in isolation or whether upstream task semantics are required. This need exists to avoid broad dependency stuffing while still making graph-sensitive reasoning available when correctness requires it.

* `handoff`

  The agent needs a richer execution-transfer packet. This is the most provenance-heavy variant, but it still should not imply unbounded context or automatic full-plan expansion.

The selector should follow these rules:

* start from the smallest task-local packet
* expand to file-backed context only when deterministic file evidence is required
* expand to dependency closure only when deterministic dependency reasoning is required
* expand to both only when both are required
* fall back to full-plan access only as an explicit last resort with a stated reason

* [ ] Define deterministic triggers for file-backed expansion:

  * `@touches`
  * `@pin`
  * acceptance references to repo files
  * explicit file or module references in task scope
  * deterministic parser recognition of canonical repo paths in scoped task text
* [ ] Define deterministic triggers for dependency expansion:

  * non-empty deps required for correctness
  * blocker resolution depends on upstream task semantics
  * integration or migration tasks that cannot be reasoned about in isolation
  * dependency acceptance or invariants required before safe execution
* [ ] Define the fallback rule for full-plan access
* [ ] Define explicit escalation reasons emitted in machine-readable output
* [ ] Define the no-guessing rule: the agent requests a need, not a raw context level
* [ ] Define a stable notion of a context miss:

  * missing file evidence
  * missing dependency semantics
  * unresolved ambiguity in task identity or bounded scope
  * explicit policy requirement that cannot be satisfied by the selected packet
* [ ] Define the rule that the selector may choose a smaller packet than the richest available packet if that smaller packet is sufficient for the requested need

Failure-handling rules:

* [ ] If `execute` is requested and task-local context is sufficient, do not escalate
* [ ] If `edit` is requested but no file-backed trigger is present, return the task packet and mark it sufficient
* [ ] If `dependency-check` is requested but deps are empty and no blocker rule requires expansion, do not escalate
* [ ] If full-plan fallback is chosen, return an explicit reason rather than hiding the escalation

Determinism constraints:

* [ ] The same plan, config, and need must select the same context class
* [ ] Escalation reasons must be stably ordered and machine-readable
* [ ] The selector must not depend on network access or LLM interpretation
* [ ] The selector must not use heuristic “maybe useful” retrieval in the canonical path

Research-informed product principles that should be captured in the spec:

* [ ] selective retrieval beats broad context stuffing
* [ ] hierarchical context is useful as an internal structure
* [ ] retrieval should be coupled to the requested task need
* [ ] dependency and provenance graphs are first-class retrieval signals
* [ ] context packets should be evaluated on sufficiency and noise, not just size

## Replace context CLI levels with need-based commands

@id pm.context.cli
@status done
@horizon now
@deps pm.context.policy
@accept cmd:grep -q -- "--need" docs/specs/context-selection-v0.1.md

The CLI should evolve toward need-based commands while preserving a migration path for existing callers.

The command contract should read like a task system, not a caching interface.

* [ ] Add a first-class `--need` selector to `plan context`
* [ ] Support these values:

  * `execute`
  * `edit`
  * `dependency-check`
  * `handoff`
  * `auto`
* [ ] Keep `--level` only as a temporary compatibility layer
* [ ] Define deprecation messaging for raw levels
* [ ] Make `--need auto` the default for agent-facing usage once the policy is stable
* [ ] Define whether `plan handoff` becomes a thin alias over `plan context --need handoff`
* [ ] Define whether `plan open` remains a pure source-slice command with no selection logic
* [ ] Define migration behavior for existing agents and scripts that already call `--level`
* [ ] Define whether text output should name the selected internal class even when the request is phrased as a need

Proposed command direction:

* [ ] `plan context <id|node-ref> --need execute`
* [ ] `plan context <id|node-ref> --need edit`
* [ ] `plan context <id|node-ref> --need dependency-check`
* [ ] `plan context <id|node-ref> --need handoff`
* [ ] `plan context <id|node-ref> --need auto`

Compatibility direction:

* [ ] keep `--level` during transition
* [ ] document that `--level` is deprecated and implementation-shaped
* [ ] map old levels onto the new internal retrieval classes only for compatibility
* [ ] remove raw level-first messaging from help text once `--need` is stable

## Emit machine-readable sufficiency and escalation metadata

@id pm.context.protocol
@status done
@horizon now
@deps pm.context.cli
@accept cmd:grep -q "escalation_reasons" docs/specs/context-selection-v0.1.md

The returned packet should tell the agent what the tool selected, why it selected it, and whether the current packet is sufficient for the requested task need.

* [ ] Add output fields for:

  * `query`
  * `need`
  * `selected_context_class`
  * `sufficient_for_need`
  * `escalation_reasons`
  * `included_files`
  * `included_deps`
  * `remaining_risks`
  * `fallback_used`
  * `full_plan_required`
* [ ] Define stable JSON behavior for those fields
* [ ] Define how text output should summarize the selected context class and escalation reasons
* [ ] Define whether unresolved insufficiency is a successful response with `sufficient_for_need=false` or a validation failure
* [ ] Define stable field ordering and omission rules for empty arrays and booleans

Suggested packet shape:

* [ ] task-local packet remains the base payload
* [ ] selected context metadata lives alongside the packet, not hidden in logs
* [ ] included files are explicit objects with path plus deterministic inclusion reason
* [ ] included deps are explicit task references, not free-form text
* [ ] remaining risks are short machine-readable strings, not essay prose

Text-mode contract:

* [ ] show requested need
* [ ] show selected context class
* [ ] show sufficiency
* [ ] show escalation reasons
* [ ] show file count and dep count
* [ ] show whether full-plan fallback was used

## Implement graph-aware context selection

@id pm.context.impl
@status done
@horizon now
@deps pm.context.protocol
@accept cmd:go test ./internal/context ./internal/cli

The implementation should use PlanMark’s task graph, provenance, and file signals as the retrieval substrate rather than relying on broad plan stuffing or ad hoc prompt construction.

* [ ] Add a deterministic selector in the context package
* [ ] Reuse existing `open`, `explain`, and `handoff` primitives where possible
* [ ] Expand file-backed context only when policy triggers require it
* [ ] Expand dependency closure only when policy triggers require it
* [ ] Keep the smallest sufficient packet as the default outcome
* [ ] Add deterministic file-signal extraction from task metadata and scoped text
* [ ] Keep dependency expansion bounded, ordered, and explainable
* [ ] Add explicit full-plan fallback handling instead of silent broad expansion
* [ ] Ensure the selector does not duplicate compile work unnecessarily

Implementation shape:

* [ ] a selector in the context package should accept:

  * compiled plan
  * task or node query
  * requested need
  * effective config
* [ ] the selector should return:

  * selected context class
  * sufficiency flag
  * escalation reasons
  * packet payload
* [ ] existing commands should be composed rather than reimplemented where possible:

  * `open` for exact bounded slices
  * `explain` for blockers and metadata suggestions
  * `handoff` as a richer packet mode or alias

First implementation target:

* [ ] `execute` and `handoff` should land first
* [ ] `edit` should land next with deterministic file-backed expansion
* [ ] `dependency-check` should land after the dependency-closure contract is nailed down
* [ ] `auto` should be default only after the other needs are stable and test-covered

## Add evaluation and telemetry for context sufficiency

@id pm.context.eval
@status done
@horizon next
@deps pm.context.impl
@accept cmd:test -f docs/specs/context-selection-v0.1.md

We need a way to evaluate whether the new model actually improves context management and accuracy rather than just renaming levels.

* [ ] Define machine-readable stats for:

  * lines included
  * files included
  * deps included
  * estimated token count
  * escalation path taken
* [ ] Add comparison guidance against full-plan retrieval
* [ ] Define what counts as a context miss
* [ ] Document a small evaluation protocol for agent tasks using `execute`, `edit`, and `dependency-check`
* [ ] Define what counts as over-retrieval noise
* [ ] Define how to compare need-based retrieval against legacy level-based retrieval during migration

Evaluation questions the spec should answer:

* [ ] Did the selector return enough context for the requested need?
* [ ] Did it avoid adding unrelated plan or repo content?
* [ ] Did it escalate only when policy required it?
* [ ] Did it make agent behavior more accurate or just larger?
* [ ] Did it reduce unnecessary full-plan reads?

Suggested acceptance bar for the first version:

* [ ] deterministic selection under repeated runs
* [ ] no reliance on full-plan fallback for the common `execute` path
* [ ] `edit` includes file-backed context only when policy triggers require it
* [ ] `dependency-check` includes deps only when graph reasoning is required
* [ ] machine-readable output is stable enough for agent orchestration

## Open design questions

* [ ] Should `handoff` stay as a separate top-level command or become a named need under `plan context` only?
* [ ] Should `auto` be a first release feature or wait until the explicit needs are stable?
* [ ] Should acceptance references to file paths be parsed structurally or start as deterministic string detection?
* [ ] How much dependency closure is enough for `dependency-check` before it becomes noisy?
* [ ] Should file-backed context include full file contents, bounded excerpts, or only pointers in the first version?
* [ ] Should context packets be allowed to say “insufficient” without treating that as a command failure?

## Recommended execution order for context selection

* [ ] `pm.context.root`
* [ ] `pm.context.policy`
* [ ] `pm.context.cli`
* [ ] `pm.context.protocol`
* [ ] `pm.context.impl`
* [ ] `pm.context.eval`

[1]: https://github.com/Vekram1/PlanMark "GitHub - Vekram1/PlanMark · GitHub"
