# PlanMark

PlanMark is a deterministic, lossless plan toolchain for `PLAN.md`.

It compiles mixed-content plans (human + LLM authored) into machine-actionable IR, emits diagnostics and context packets, and supports tracker projection workflows without making the tracker canonical.

## What It Can Do Today

- Deterministic compile from plan markdown to structured IR (`plan compile`).
- Tolerant diagnostics and readiness checks (`plan doctor`) with machine-readable output.
- Provenance-backed context retrieval (`plan context`, `plan open`, `plan explain`).
- Deterministic change reporting (`plan changes`).
- Beads reconcile/sync workflow with dry-run support (`plan sync beads`).
- Deterministic handoff packet generation for task-focused sessions (`plan handoff`).
- Non-canonical AI assistive commands (`plan ai ...`) for faster authoring/review support.

## Quickstart

```bash
# 1) Capability handshake
plan version --format json

# 2) Compile plan into IR
plan compile --plan PLAN.md --out .planmark/tmp/plan.json

# 3) Diagnose readiness/tolerance issues
plan doctor --plan PLAN.md --profile loose --format rich

# 4) Get execution context for a task
plan context <task-id> --plan PLAN.md --level L0 --format json

# 5) Page-fault into exact source when needed
plan open <task-id-or-node-ref> --plan PLAN.md --format json
plan explain <task-id> --plan PLAN.md --format rich
```

## Core Commands

- `plan version --format text|json`
- `plan compile --plan <path> [--out <path>] [--state-dir <path>]`
- `plan doctor --plan <path> [--profile loose|build|exec] [--format text|rich|json]`
- `plan context <id> --plan <path> --level L0|L1|L2 [--format text|json]`
- `plan open <id|node-ref> --plan <path> [--format text|json]`
- `plan explain <id> --plan <path> [--format text|rich|json]`
- `plan handoff <id|node-ref> --plan <path> [--format text|json]`
- `plan query --plan <path> [--horizon now|next|later] [--ready|--blocked] [--format text|json]`
- `plan sync beads --plan <path> [--dry-run] [--format text|json]`
- `plan changes --plan <path> [--format text|json]`

## AI Helper Commands (Non-Canonical)

`plan ai` commands are optional helpers and are not part of the canonical deterministic compile/sync path.

### Suggest Acceptance Criteria

```bash
plan ai suggest-accept <id> --plan <path> [--format text|json]
```

Returns deterministic acceptance suggestion lines derived from explain blockers.

### Summarize Dependency Closure

```bash
plan ai summarize-closure <id> --plan <path> [--format text|json]
```

Returns a dependency closure summary with source pointers (`source_path`, line range, `node_ref`, `slice_hash`).

### Draft Granular Beads

```bash
plan ai draft-beads --plan <path> [--horizon all|now|next|later] [--limit N] [--format text|json]
```

Returns deterministic parent/child draft suggestions with:
- `draft_level` (`parent` or `child`)
- `parent_task_id` (for child rows)
- `child_order_index` (stable child ordering)

## Typical Agent Workflow

```bash
# deterministic core loop
plan compile --plan PLAN.md --out .planmark/tmp/plan.json
plan doctor --plan PLAN.md --profile build --format json
plan changes --plan PLAN.md --format json
plan sync beads --plan PLAN.md --dry-run --format json
```

## Determinism Notes

- `PLAN.md` is canonical.
- Canonical path commands (`compile`, `doctor`, `context`, `open`, `explain`, `sync`) are deterministic/offline-safe by contract.
- AI helpers are explicitly non-canonical and should be treated as assistive output only.

## Current Limitations

- Tracker/runtime coordination depends on explicit sync/reconcile calls; it is not implicit background state.
- Some advanced milestones in `PLAN.md` are intentionally staged and not fully implemented yet.
- AI helper output is not canonical truth; users must review before applying changes.

## Development

```bash
go test ./...
```
