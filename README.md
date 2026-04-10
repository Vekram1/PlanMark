<img width="200" height="200" alt="PlanMark project task overview logo" src="https://github.com/user-attachments/assets/d4be234d-4c58-47cd-a29b-7951fefe5ed2" />
# PlanMark
![planmark-demo](https://github.com/user-attachments/assets/64f7eac6-20f3-4a16-8c0b-794610a0cf95)

PlanMark turns a `PLAN.md` file into deterministic, machine-usable planning artifacts.

Use it when you want:
- `PLAN.md` to stay canonical
- agents to consume structured task context instead of scraping prose
- tracker sync to be a projection of the plan, not the source of truth
- deterministic compile, diagnostics, and handoff outputs

## What It Does

PlanMark keeps ordinary Markdown as the authoring surface and derives structured outputs from it.

The core workflow is:
1. write `PLAN.md`
2. compile it into deterministic IR
3. check readiness and blockers
4. fetch task context or handoff packets for agents
5. optionally sync projected tasks into a tracker

You do not need a separate DSL. PlanMark uses normal Markdown plus a small amount of task metadata such as `@id`, `@horizon`, `@deps`, and `@accept`.

## Install

Build from a local checkout:

```bash
go build -o ./bin/planmark ./cmd/planmark
export PATH="$PWD/bin:$PATH"
planmark version --format json
```

Install with the project script on macOS or Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/Vekram1/PlanMark/master/scripts/install.sh | bash
planmark version --format json
```

If `planmark` is not on your shell path after install:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

The installer places `planmark` on your path and also installs `plan` as a compatibility alias by default.

Check for newer releases or update in place on macOS/Linux:

```bash
planmark update --check
planmark update
```

Install on Windows with PowerShell:

```powershell
powershell -ExecutionPolicy Bypass -c "irm https://raw.githubusercontent.com/Vekram1/PlanMark/master/scripts/install.ps1 | iex"
planmark version --format json
```

Windows release assets are published as `.zip` archives. The PowerShell installer installs `planmark.exe` and `plan.exe` under `%LOCALAPPDATA%\Programs\PlanMark\bin` by default.
To update on Windows, rerun the PowerShell installer.

## First 10 Minutes

Initialize the repo:

```bash
planmark init --dir . --format text
```

This creates repo-local PlanMark state under `.planmark/` and adds starter files such as `PLAN.md`, `.planmark.yaml`, and a managed PlanMark section in `AGENTS.md` when missing.

Compile the initial plan:

```bash
planmark compile --plan PLAN.md --out .planmark/tmp/plan.json
```

Check the plan for readiness issues:

```bash
planmark doctor --plan PLAN.md --profile loose --format rich
```

List tasks:

```bash
planmark query --plan PLAN.md --format text
```

If those commands work, the repo is ready for agent-facing task context.

## Your First `PLAN.md`

A small, realistic example:

```md
# PLAN

## API rollout

### Add migration
@id api.migrate
@horizon now
@deps api.schema
@accept cmd:go test ./...
@rollback restore snapshot and revert migration file

We need an additive rollout because older workers may still read legacy columns.

- [ ] Write additive migration
- [ ] Verify rollback path
- [ ] Run local validation

### Validate schema assumptions
@id api.schema
@horizon next

Confirm the new columns are additive and safe for older readers.
```

A compact checkbox task style also works:

```md
- [ ] Add migration
  @id api.migrate
  @horizon now
  @deps api.schema
  @accept cmd:go test ./...
```

Use heading tasks when you want rationale, examples, risk notes, or a scoped execution checklist under the task. A bare heading without task metadata is just document structure.

## Commands New Users Actually Need

Compile to deterministic IR:

```bash
planmark compile --plan PLAN.md --out .planmark/tmp/plan.json
```

Check health with increasing strictness:

```bash
planmark doctor --plan PLAN.md --profile loose --format rich
planmark doctor --plan PLAN.md --profile build --format rich
planmark doctor --plan PLAN.md --profile exec --format rich
```

Query tasks:

```bash
planmark query --plan PLAN.md --format text
```

Get agent-usable task context:

```bash
planmark context api.migrate --plan PLAN.md --level L0 --format json
planmark open api.migrate --plan PLAN.md --format json
planmark explain api.migrate --plan PLAN.md --format rich
planmark handoff api.migrate --plan PLAN.md --format json
```

Recommended escalation path for agents:

1. `planmark context <id> --level L0`
2. `planmark open <id|node-ref>`
3. `planmark explain <id>`
4. `planmark context <id> --level L1`
5. `planmark context <id> --level L2`

That keeps context small while preserving deterministic traceability back to the plan source.

How to choose context in practice:

- Start with need-based retrieval by default:
  - `planmark context <id> --plan PLAN.md --format json`
- Use explicit needs when the operation is clear:
  - `--need execute`
  - `--need edit`
  - `--need dependency-check`
  - `--need handoff`
- Treat richer context as an escalation, not the starting point.
- Legacy `--level L0|L1|L2` remains compatibility-only and should not be the primary path.

## Tracker Sync

Trackers are projection layers. `PLAN.md` remains canonical.

Preview sync without mutating the tracker:

```bash
planmark sync beads --plan PLAN.md --dry-run --format json
planmark sync linear --plan PLAN.md --dry-run --format json
```

Or select the adapter explicitly:

```bash
planmark sync --plan PLAN.md --adapter linear --profile compact --dry-run --format json
```

Built-in render profiles:
- `default`
- `compact`
- `agentic`
- `handoff`

Current proven adapters:
- `beads`
- `github`
- `linear`

Optional defaults in `.planmark.yaml`:

```yaml
tracker:
  adapter: beads
  profile: default
```

## Canonical vs Non-Canonical

Canonical deterministic commands:
- `planmark compile`
- `planmark doctor`
- `planmark context`
- `planmark open`
- `planmark explain`
- `planmark handoff`
- `planmark sync`

These are intended to stay deterministic and offline-safe.

Optional assistive commands live under:
- `planmark ai ...`

Use AI helpers as drafting support, not as the source of truth.

## Writing Plans That Work Well For Agents

Good PlanMark plans usually have these traits:
- important tasks have explicit `@id` values
- rationale sits near the task instead of in a distant section
- `@horizon now` work includes at least one `@accept`
- nested checklists describe execution steps within the task scope
- rollback notes, assumptions, and risks stay attached to the relevant task
- tasks are bounded and specific instead of being a long flat checklist

The goal is not to formalize everything. The goal is to make each task executable with enough nearby context to act safely.

## Common Commands

```bash
planmark --help
planmark version --format json
planmark init --dir . --format text
planmark compile --plan PLAN.md --out .planmark/tmp/plan.json
planmark doctor --plan PLAN.md --profile loose --format rich
planmark query --plan PLAN.md --format text
planmark context <id> --plan PLAN.md --level L0 --format json
planmark open <id|node-ref> --plan PLAN.md --format json
planmark explain <id> --plan PLAN.md --format rich
planmark handoff <id|node-ref> --plan PLAN.md --format json
planmark sync [beads|github|linear] --plan PLAN.md --dry-run --format json
```

## Development

Run the test suite:

```bash
go test ./...
```
