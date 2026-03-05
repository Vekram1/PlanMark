# PlanMark

PlanMark is a deterministic, lossless plan toolchain for `PLAN.md`.

It compiles mixed-content plans (human + LLM authored) into machine-actionable IR, emits diagnostics and context packets, and supports tracker projection workflows without making the tracker canonical.

## How It Works (Visual)

```text
PLAN.md (canonical, mixed markdown)
    |
    +--> plan compile -----------> plan.json (lossless IR + semantic IR)
    |                                 |
    |                                 +--> plan changes (deterministic semantic diff)
    |
    +--> plan doctor ------------> diagnostics/readiness (json/text/rich)
    |
    +--> plan context/open/explain
    |       |
    |       +--> L0/L1/L2 context packets + exact source page-fault retrieval
    |
    +--> plan sync beads --------> deterministic tracker projection/reconcile
    |
    +--> plan ai ... ------------> optional assistive drafts/summaries (non-canonical)
```

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
# 0) Install the CLI (from source checkout)
go build -o ./bin/plan ./cmd/plan
./bin/plan version --format json
export PATH="$PWD/bin:$PATH"

# 1) Initialize current project once
./bin/plan init --dir . --format text

# 2) Capability handshake
plan version --format json

# 3) Compile plan into IR
plan compile --plan PLAN.md --out .planmark/tmp/plan.json

# 4) Diagnose readiness/tolerance issues
plan doctor --plan PLAN.md --profile loose --format rich

# 5) Get execution context for a task
plan context <task-id> --plan PLAN.md --level L0 --format json

# 6) Page-fault into exact source when needed
plan open <task-id-or-node-ref> --plan PLAN.md --format json
plan explain <task-id> --plan PLAN.md --format rich
```

## Standalone Install (curl)

Install PlanMark globally with dependency checks (macOS/Linux):

```bash
curl -fsSL https://raw.githubusercontent.com/Vekram1/PlanMark/master/scripts/install.sh | bash
```

Optional installer environment variables:

```bash
# install location (default: ~/.local/bin)
PLANMARK_INSTALL_DIR="$HOME/bin"

# repo/ref override
PLANMARK_REPO="Vekram1/PlanMark"
PLANMARK_CHANNEL="stable" # default; uses latest GitHub release tag
PLANMARK_CHANNEL="edge"   # uses master
PLANMARK_REF="v0.1.0"     # pin exact ref/tag/branch

# disable automatic dependency install attempts
PLANMARK_AUTO_INSTALL_DEPS=0
```

After install, initialize any project once:

```bash
cd /path/to/your/project
plan init --dir . --format text
```

## Updating PlanMark

Stable update (recommended):

```bash
curl -fsSL https://raw.githubusercontent.com/Vekram1/PlanMark/master/scripts/install.sh | bash
```

Edge update (latest `master`):

```bash
curl -fsSL https://raw.githubusercontent.com/Vekram1/PlanMark/master/scripts/install.sh | PLANMARK_CHANNEL=edge bash
```

Pinned update:

```bash
curl -fsSL https://raw.githubusercontent.com/Vekram1/PlanMark/master/scripts/install.sh | PLANMARK_REF=v0.1.0 bash
```

Local updater wrapper (when running from a checkout):

```bash
./scripts/update.sh
```

## Startup Guide

Use this when bringing up PlanMark in a new environment.

### 1) Install PlanMark

```bash
curl -fsSL https://raw.githubusercontent.com/Vekram1/PlanMark/master/scripts/install.sh | bash
```

### 2) Confirm the CLI is available

```bash
plan version --format json
```

If `plan` is not found, add the installer output path to your shell profile, for example:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

### 3) Initialize a project once

```bash
cd /path/to/your/project
plan init --dir . --format text
```

This creates project-local PlanMark state (`.planmark/`) and optional starter files if missing.
It also creates or updates a managed block in `AGENTS.md` documenting available `plan` commands for coding agents.

### `plan init` creates

- `.planmark/state_version.json`
- `.planmark/build/`, `.planmark/sync/`
- `.planmark/cache/context/`
- `.planmark/cas/sha256/`
- `.planmark/journal/sync/`
- `.planmark/locks/`
- `PLAN.md` template (if missing, unless `--no-plan-template`)
- `.planmark.yaml` template (if missing, unless `--no-config`)
- managed `AGENTS.md` command block

Managed AGENTS block markers:

```md
<!-- planmark:init:start -->
...managed content...
<!-- planmark:init:end -->
```

If markers already exist, only that managed block is replaced.
If markers do not exist, the block is appended and existing AGENTS content is preserved.

### 4) Run the deterministic core workflow

```bash
plan compile --plan PLAN.md --out .planmark/tmp/plan.json
plan doctor --plan PLAN.md --profile loose --format rich
plan changes --plan PLAN.md --format json
plan sync beads --plan PLAN.md --dry-run --format json
```

### 5) Optional AI workflow

```bash
plan ai apply-fix --plan PLAN.md --approve --format json
```

## Core Commands

- `plan version --format text|json`
- `plan init [--dir <path>] [--plan <path>] [--state-dir <path>] [--config <path>] [--no-plan-template] [--no-config] [--format text|json]`
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

### Apply Fix With Configured/Connected Provider

```bash
# Repo-local config in .planmark.yaml
cat > .planmark.yaml <<'YAML'
ai:
  provider: deterministic_mock
  # provider: openai_compatible
  # model: gpt-4o-mini
  # base_url: https://api.openai.com/v1
  # api_key_env: OPENAI_API_KEY
  # timeout_seconds: 30
YAML

plan ai apply-fix --plan PLAN.md --approve --format json
```

You can override provider settings per invocation:

```bash
plan ai apply-fix --plan PLAN.md --approve \
  --provider deterministic_mock \
  --model gpt-4o-mini \
  --format json
```

## Typical Agent Workflow

```bash
# deterministic core loop
plan compile --plan PLAN.md --out .planmark/tmp/plan.json
plan doctor --plan PLAN.md --profile build --format json
plan changes --plan PLAN.md --format json
plan sync beads --plan PLAN.md --dry-run --format json
```

```text
Agent loop (deterministic core):

compile --> doctor --> changes --> sync --dry-run --> (review) --> sync apply
   |          |           |              |
   |          |           |              +--> tracker operation preview
   |          |           +--> task-level semantic drift
   |          +--> blockers/readiness diagnostics
   +--> canonical IR baseline

Escalate context only when needed:
context L0 --> open/explain --> context L1 --> context L2
```

## Context Minimization Workflow

Use an escalation ladder to save context:

1. Start with task-only context (`L0`):
   - `plan context <task-id> --plan PLAN.md --level L0 --format json`
2. Escalate to `L1` only when pin evidence is needed.
3. Escalate to `L2` only when dependency closure is needed.
4. Use `plan open` / `plan explain` for targeted retrieval before broad escalation.
5. Fall back to full `PLAN.md` only when ambiguity remains.

This keeps routine execution sessions small while preserving deterministic traceability to source.

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
