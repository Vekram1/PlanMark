# PlanMark Global Context

This document is the stable, minimal context agents should read at session start.
It is not a replacement for `PLAN.md`; it is a compact map for consistent execution.

## Project purpose

PlanMark is a deterministic, lossless planning toolchain for `PLAN.md`.
`PLAN.md` is canonical for structure and intent; tracker state is a projection layer.

Primary outcomes:
- deterministic compile (`plan compile`) to canonical IR (`plan.json`)
- tolerance-first diagnostics (`plan doctor`)
- provenance-backed retrieval and execution context (`plan context`, `plan open`, `plan explain`)
- deterministic tracker reconciliation (`plan sync`)

## Architecture map

Top-level layering:
- `cmd/plan`: CLI entrypoint only
- `internal/cli`: command parsing + output shaping
- core deterministic logic:
  - `internal/compile`
  - `internal/ir`
  - `internal/change`
  - `internal/context`
  - `internal/syncplanner`
- tracker adapters:
  - `internal/tracker` (network-facing integration points)
- shared support:
  - `internal/doctor`, `internal/policy`, `internal/protocol`, `internal/fsio`, `internal/cache`

Non-negotiables:
- no LLM in canonical path (`compile/doctor/context/open/explain/sync`)
- deterministic canonical JSON + explicit policy/versioning
- lossless source capture with provenance

## Command contract

Core command groups:
- Compile/validate: `compile`, `doctor`, `changes`
- Context/retrieval: `context`, `open`, `explain`, `pack`, `query`
- Sync/reconcile: `sync [beads|github|linear]`
- Replanning: `propose-change`, `apply-change`
- Metadata/introspection: `version`, `id`
- Non-canonical AI helpers: `ai ...`

Machine behavior expectations:
- stable JSON envelopes for `--format json`
- stable exit taxonomy (success, validation_failed, usage_error, internal_error)
- deterministic outputs for identical inputs + policy versions

## Agent startup flow

Use this order at session start:
1. Read `AGENTS.md` (execution protocol).
2. Read this file (`docs/context/global.md`) for stable architecture context.
3. Use `bv --robot-next` / `br ready --json` to pick the next actionable bead.
4. Use `br show <id> --json` and `plan open <id|node_ref>` to load only relevant plan slices.
5. Read full `PLAN.md` when:
   - touching contract surfaces
   - reconciling spec ambiguity
   - introducing new milestone-level work
6. Register/refresh MCP Agent Mail, acknowledge pending messages, reserve files, announce start.

This flow keeps context minimal while preserving deterministic traceability back to `PLAN.md`.

## Context budget policy (Need-Based)

Use need-based retrieval by default:
1. Start each bead with implicit `auto`:
   - `plan context <id> --plan PLAN.md --format json`
2. Use explicit needs when the operation is clear:
   - `plan context <id> --plan PLAN.md --need execute --format json`
   - `plan context <id> --plan PLAN.md --need edit --format json`
   - `plan context <id> --plan PLAN.md --need dependency-check --format json`
   - `plan context <id> --plan PLAN.md --need handoff --format json`
3. Use `plan open` / `plan explain` for targeted page faults before asking for richer context.
4. Read full `PLAN.md` only when required by unresolved ambiguity or contract-level work.

Legacy compatibility:
- `plan context --level L0|L1|L2` remains available during migration, but `--need` is the primary interface.

Escalation trigger rule:
- Include a one-line reason in agent updates, e.g.:
  - `Escalating to file-backed context: acceptance references docs/specs/context-selection-v0.1.md.`
  - `Escalating to dependency-check: declared deps require graph reasoning.`
