# PlanMark

PlanMark is a deterministic, lossless plan toolchain for `PLAN.md`.

It compiles mixed-content plans (human + LLM authored) into machine-actionable IR, emits diagnostics and context packets, and supports tracker projection workflows without making the tracker canonical.

## Core Commands

- `plan version --format text|json`
- `plan compile --plan <path> [--out <path>] [--state-dir <path>]`
- `plan doctor --plan <path> [--profile loose|build|exec] [--format text|rich|json]`
- `plan context <id> --plan <path> --level L0|L1|L2 [--format text|json]`
- `plan open <id|node-ref> --plan <path> [--format text|json]`
- `plan explain <id> --plan <path> [--format text|rich|json]`
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

## Determinism Notes

- `PLAN.md` is canonical.
- Canonical path commands (`compile`, `doctor`, `context`, `open`, `explain`, `sync`) are deterministic/offline-safe by contract.
- AI helpers are explicitly non-canonical and should be treated as assistive output only.

## Development

```bash
go test ./...
```
