# PlanMark Tracker Reconcile Test Matrix v0.1

## Status

- Spec ID: `tracker_reconcile_test_matrix/v0.1`
- Scope: required invariant and scenario coverage for deterministic PLAN-to-tracker reconciliation
- Canonical dependencies:
  - `docs/specs/planmark-v0.2.md`
  - `docs/specs/tracker-reconcile-v0.1.md`
  - `docs/specs/change-detection-v0.1.md`
  - `docs/specs/semantic-derivation-v0.2.md`

## Purpose

Define the minimum test and invariant coverage required to trust PlanMark reconcile behavior across the seams that matter in practice:

- source PLAN changes
- semantic derivation
- tracker-neutral projection
- local sync manifest memory
- actual tracker reality
- cleanup interactions

The purpose of this matrix is not to exhaustively enumerate every possible state combination. The state space is too large for that to be the right goal. The purpose is to make the safety-critical invariants explicit, tie them to concrete scenario classes, and ensure that regressions appear as violations of named guarantees rather than as surprising emergent behavior.

## Why This Exists

Recent reconcile bugs were not caused by one obviously broken component. They were caused by layer mismatch.

Examples:

- A plan-backed tracker issue existed remotely but had fallen out of the local sync manifest, so manifest-only stale detection failed to retire it.
- A current plan-backed issue was closed in the tracker, but semantic equality caused sync to classify the task as `no-op`, and the apply phase skipped `no-op` operations entirely, so the tracker did not converge back to current plan intent.
- Rich task body changes in `PLAN.md` did not propagate to tracker descriptions because the semantic layer and tracker projection were too narrow.

These are seam failures:

- source layer vs semantic layer
- semantic layer vs projection layer
- projection layer vs reconcile planning
- reconcile planning vs apply behavior
- local manifest memory vs tracker reality

The matrix exists to force explicit coverage of those seam failures.

## Core Reconcile Model

The system should be reasoned about as five layers:

1. Source layer
- The literal authored contents of `PLAN.md`.

1. Semantic layer
- The canonical interpreted task set emitted by `plan compile`.

1. Projection layer
- The tracker-neutral task payload and deterministic projection hash used for tracker sync.

1. Reconcile planning layer
- The operation classification emitted from desired semantic intent, prior sync memory, and live tracker state.

1. Tracker apply layer
- The concrete mutations that cause tracker reality to converge to current plan intent.

Unexpected outcomes generally arise when one of these layers is treated as more authoritative than it should be.

Examples:

- treating manifest memory as if it were tracker truth
- treating projection hash equality as if it proved tracker state was already correct
- treating provenance churn as if it were semantic task change
- treating cleanup and sync as if they could use different definitions of "plan-backed issue"

## Required System Invariants

These invariants are the primary contract. The matrix exists to test them.

### I1. Current plan tasks must converge to open tracker issues

After a successful `plan sync` apply:

- every current plan-backed task must have exactly one live tracker mapping
- the corresponding tracker issue must be open or otherwise active according to adapter policy
- current tasks must not remain silently closed because their projection hash was unchanged

Implication:

- semantic equality is not enough to classify tracker state as already correct
- reconcile must consider actual tracker status, not only prior local manifest data

### I2. Removed plan tasks must not remain open

After a successful `plan sync` apply:

- a task removed from current `PLAN.md` must not continue to exist as an open plan-backed tracker issue
- this must hold even if local manifest memory was lost or stale

Implication:

- stale detection cannot rely only on the prior manifest
- reconcile must have a path to discover tracker-backed stale candidates from live tracker state

### I3. Semantic changes must propagate to tracker-visible output

If a task’s canonical meaning changes in a way that should affect the tracker-facing task description, title, sections, steps, deps, or acceptance:

- semantic derivation must change accordingly
- projection hash must change accordingly
- tracker-visible rendered state must update accordingly

Implication:

- semantic derivation and tracker projection cannot be so narrow that important task body changes are invisible

### I4. Provenance-only churn must not cause broad tracker churn

If a task’s semantic meaning is unchanged and only source provenance has changed, such as:

- line number movement
- slice re-addressing
- compile id movement

then tracker-visible semantic state should remain stable and routine sync should not emit broad updates solely due to provenance churn.

Implication:

- projection hashing must exclude provenance-only volatility from ordinary semantic update decisions

### I5. Cleanup must not break sync recovery

If cleanup closes a plan-derived tracker issue that should not be live under the current plan, sync should not regress.

If cleanup or stale handling closes a current plan-backed issue by accident or by prior policy:

- the next sync must be able to restore the current-plan issue to a live state

Implication:

- cleanup and sync must agree on what counts as current plan-backed work
- sync must ensure desired tracker reality, not merely skip unchanged semantic tasks

### I6. Manifest loss must not prevent eventual convergence

If `.planmark/sync/beads-manifest.json` is stale, missing entries, or partially lost:

- sync must still be able to converge tracker reality toward current plan intent
- stale issues should still be retired
- current issues should still be restored or updated

Implication:

- manifest is local reconcile memory, not tracker truth
- missing manifest entries cannot permanently blind the system

### I7. Ambiguous identity must become explicit conflict or bounded cleanup

If multiple tracker issues claim the same canonical identity or current tracker identity cannot be resolved safely:

- the behavior must be explicit and deterministic
- silent hijacking, silent duplication, or silent overwrite is not acceptable

Implication:

- identity ambiguity must surface as conflict or as a narrowly classified cleanup case

## Scenario Classes

These scenario classes define the minimum surface that tests must cover.

### S1. Canonical intent scenarios

Focus:

- source `PLAN.md`
- semantic derivation

Required examples:

- task title change
- task details/body change
- task steps change
- task dependency change
- task acceptance change
- task addition
- task deletion
- task id change

Expected checks:

- semantic task set matches authored intent
- semantic fingerprint changes when canonical meaning changes
- semantic fingerprint does not change when only irrelevant churn occurs

### S2. Projection scenarios

Focus:

- semantic task -> tracker-neutral projection
- tracker-visible rendered fields

Required examples:

- title change updates tracker title
- details/body change updates rendered tracker body
- structured sections are preserved in projection
- ordered steps remain ordered
- evidence and acceptance remain stable and deterministic
- provenance-only movement does not force projection churn

Expected checks:

- rendered output contains intended semantic content
- projection hash changes only for tracker-facing semantic changes

### S3. Reconcile planning scenarios

Focus:

- desired semantic state
- prior manifest
- live tracker stale candidates

Required examples:

- desired present, prior absent => `create`
- desired changed, prior present => `update`
- desired unchanged, prior present => `no_op`
- desired absent, prior present => `mark-stale`
- desired absent, manifest absent, live tracker issue present => stale path still discovered
- duplicate desired or duplicate prior identity => `conflict`

Expected checks:

- operation classes are deterministic
- stale planning works even when manifest memory is incomplete

### S4. Apply and mutation scenarios

Focus:

- actual tracker state transitions

Required examples:

- create current task from empty tracker state
- update changed current task
- reopen closed current task with unchanged projection
- close stale removed task
- stale close on already-closed task is idempotent
- retryable tracker failure preserves operation identity

Expected checks:

- current plan-backed tasks end up open
- removed tasks do not remain open
- idempotent apply remains safe under retries

### S5. Cleanup scenarios

Focus:

- non-plan or foreign-plan tracker debris

Required examples:

- issue has `external_ref` missing from current plan
- issue has no `external_ref` but provenance points at a different plan file
- current plan-backed issue with matching `external_ref`
- unrelated manual tracker issue with no recognizable plan provenance

Expected checks:

- cleanup identifies only bounded, justified candidates
- cleanup never classifies a current plan-backed issue as a candidate
- cleanup dry-run and apply share the same candidate set

### S6. End-to-end convergence scenarios

Focus:

- repeated operations over time

Required sequences:

1. empty tracker -> sync
2. edit plan -> sync
3. delete task from plan -> sync
4. cleanup -> sync
5. manifest loss -> sync
6. current task closed remotely -> sync

Expected checks:

- final open tracker state corresponds to current plan intent
- repeated syncs converge instead of oscillating
- cleanup does not permanently break current-plan restoration

## Minimum Assertions By Scenario

Every scenario class should assert at least one of the following:

- exact current plan task count
- exact open plan-backed issue count
- exact tracker issue liveness for a named task id
- exact operation classification
- exact rendered title/body/section presence
- exact manifest inclusion or exclusion
- exact conflict reason or cleanup reason

Tests should avoid vague assertions like "something updated" when a more explicit invariant is available.

## Recommended Test Structure

The matrix should map onto the existing Go testing layers instead of forcing everything into one giant end-to-end test.

### Unit-style semantic tests

Target packages:

- `internal/compile`
- `internal/build`

Purpose:

- prove source-to-semantic behavior
- prove semantic-to-hash behavior

### Unit-style projection and adapter tests

Target packages:

- `internal/tracker`

Purpose:

- prove tracker-neutral projection behavior
- prove tracker-side idempotence, reopen, close, and cleanup classification

### Planner tests

Target packages:

- `internal/syncplanner`

Purpose:

- prove deterministic operation planning under varied desired/prior inputs

### CLI integration tests

Target packages:

- `internal/cli`

Purpose:

- prove end-to-end state convergence across compile -> plan -> apply -> manifest
- prove cleanup and sync interaction

## Required Regression Categories

At a minimum, regressions should exist for these named bug categories:

- manifest-only stale blindness
- no-op skipping closed current tasks
- semantic task prose missing from projection
- provenance-only tracker churn
- stale close idempotence on already-closed issues
- cleanup candidate misclassification against current plan ids

These names are intended to remain stable enough that future bug reports can map back to known categories.

## What This Matrix Does Not Promise

This matrix does not promise exhaustive state-space coverage.

It does not by itself prove:

- full formal correctness of reconcile
- absence of all tracker-specific backend quirks
- complete coverage of all historical legacy tracker debris

It does define the minimum practical safety net that should exist before saying tracker reconcile behavior is trustworthy for day-to-day use.

## Acceptance Guidance

This spec should be treated as active when all of the following are true:

- named invariant coverage exists in code
- cleanup and sync interactions are covered by tests
- manifest-loss recovery scenarios are covered
- current plan restoration after closed tracker state is covered
- CI runs the relevant reconcile packages by default

