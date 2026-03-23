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

## Tracker-Neutral Rendering Implication

The reconcile policy is intentionally tracker-neutral even when the first implementation targets Beads.

- Semantic task identity, provenance, readiness metadata, and scoped evidence come from PLAN/IR.
- A tracker-neutral `TaskProjectionV2` sits between Semantic IR and adapter rendering so adapters consume canonical task semantics rather than Beads-specific fields.
- Each adapter also exposes deterministic `TrackerCapabilities` so future rendering/template policy can validate backend support without hardcoding tracker assumptions into the compiler.
- Tracker adapters choose how to render that semantic task into the target system's available fields.
- Runtime overlays remain tracker-owned regardless of rendering shape.
- The current Beads projection payload schema is `v0.2` and is built from that projection layer, carrying the subset of adapter-neutral task fields that Beads currently renders:
  - `horizon`
  - ordered `dependencies`
  - ordered execution `steps`
  - ordered `evidence_node_refs`
  - provenance/source mapping and acceptance digest
- Sync planning hashes the full canonical `TaskProjectionV2`, including reserved projection fields such as structured `sections` and evidence `kind`, so semantic projection changes are still detected before every adapter renders every field.

Current capability descriptor categories:

- title support
- body text mode (`unsupported` | `plain` | `markdown`)
- ordered step/checklist support (`unsupported` | `rendered` | `native`)
- child-work support (`unsupported` | `rendered` | `native`)
- custom-field support (`unsupported` | `rendered` | `native`)
- safe runtime overlay support (`status`, `assignee`, `priority`)

The descriptor is deterministic adapter metadata. It does not change the canonical task projection or reconcile operation classes.

Current proven adapters:

- `beads`: projection-backed local tracker with safe runtime overlays for `status`, `assignee`, and `priority`
- `github`: GitHub Issues proof adapter with deterministic markdown issue rendering and safe runtime overlays for `status` and `assignee`

Current built-in rendering profiles:

- `default`: balanced issue/task rendering with section headings, rendered checklist steps when needed, and explicit provenance
- `compact`: condensed body layout for backends or views where terse rendering is preferred
- `agentic`: agent-facing layout with explicit task metadata, rationale, acceptance, evidence, and provenance blocks
- `handoff`: handoff-oriented variant of the agentic layout with the same deterministic section ordering and fuller provenance emphasis

Profiles are deterministic named policies layered on top of `TaskProjectionV2`. They are validated against `TrackerCapabilities` before adapter-specific payloads are built.

Current adapter/profile usage:

- `beads` uses the `default` render profile internally while preserving its existing projection payload schema
- `github` uses the `default` render profile to build deterministic issue title/body payloads and tracker labels

Selection sources are deterministic and repository-local:

- explicit `plan sync` CLI flags (`--adapter`, `--profile`)
- explicit positional sync target (`beads`, `github`)
- `.planmark.yaml` `tracker:` defaults
- built-in defaults when no tracker config is present

Illustrative target mapping for the same semantic task:

- Beads:
  - title from task title
  - body from rationale, steps, acceptance lines, rollback note, provenance footer
  - safe-pull runtime fields remain `status`, `assignee`, `priority`
- GitHub Issues / Linear / Jira style adapters:
  - title from task title
  - description/body from scoped rationale and evidence
  - checklist or adapter-specific child-work representation from ordered execution steps
  - provenance footer or custom metadata field from canonical source mapping

The operation classes do not change across trackers.
Only payload rendering changes.

## Dry-Run / Apply Consistency

- Dry-run and apply must consume the same operation plan.
- Apply may retry transient tracker failures, but retry behavior must not mutate operation classification.
- Operation IDs are stable for journaling/auditability across retries.

## Non-Goals

- Multi-tracker conflict matrices in MVP.
- Implicit destructive tracker mutation defaults.
