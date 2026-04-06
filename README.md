# PlanMark

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
go build -o ./bin/plan ./cmd/plan
export PATH="$PWD/bin:$PATH"
plan version --format json
```

Install with the project script on macOS or Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/Vekram1/PlanMark/master/scripts/install.sh | bash
plan version --format json
```

If `plan` is not on your shell path after install:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

## First 10 Minutes

Initialize the repo:

```bash
plan init --dir . --format text
```

This creates repo-local PlanMark state under `.planmark/` and adds starter files such as `PLAN.md`, `.planmark.yaml`, and a managed PlanMark section in `AGENTS.md` when missing.

Compile the initial plan:

```bash
plan compile --plan PLAN.md --out .planmark/tmp/plan.json
```

Check the plan for readiness issues:

```bash
plan doctor --plan PLAN.md --profile loose --format rich
```

List tasks:

```bash
plan query --plan PLAN.md --format text
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
plan compile --plan PLAN.md --out .planmark/tmp/plan.json
```

Check health with increasing strictness:

```bash
plan doctor --plan PLAN.md --profile loose --format rich
plan doctor --plan PLAN.md --profile build --format rich
plan doctor --plan PLAN.md --profile exec --format rich
```

Query tasks:

```bash
plan query --plan PLAN.md --format text
```

Get agent-usable task context:

```bash
plan context api.migrate --plan PLAN.md --level L0 --format json
plan open api.migrate --plan PLAN.md --format json
plan explain api.migrate --plan PLAN.md --format rich
plan handoff api.migrate --plan PLAN.md --format json
```

Recommended escalation path for agents:

1. `plan context <id> --level L0`
2. `plan open <id|node-ref>`
3. `plan explain <id>`
4. `plan context <id> --level L1`
5. `plan context <id> --level L2`

That keeps context small while preserving deterministic traceability back to the plan source.

How to choose the level in practice:

- Start with `L0` by default. `L0` already includes the task's identity, dependencies, acceptance targets, steps, evidence references, and the exact source slice from `PLAN.md` via `source_path`, `start_line`, `end_line`, `slice_hash`, and `slice_text`.
- Escalate to `L1` only when the task includes pin-backed references and you need the referenced file or range extracts in addition to the task's own plan slice.
- Escalate to `L2` only when you need dependency-closure reasoning, such as understanding upstream tasks that must be completed or inspected first.
- Treat context as a progressive budget. Do not default to `L2` for routine execution work, because it pulls in adjacent task context that is often unnecessary.

## Tracker Sync

Trackers are projection layers. `PLAN.md` remains canonical.

Preview sync without mutating the tracker:

```bash
plan sync beads --plan PLAN.md --dry-run --format json
plan sync github --plan PLAN.md --dry-run --format json
plan sync linear --plan PLAN.md --dry-run --format json
```

Or select the adapter explicitly:

```bash
plan sync --plan PLAN.md --adapter github --profile compact --dry-run --format json
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
- `plan compile`
- `plan doctor`
- `plan context`
- `plan open`
- `plan explain`
- `plan handoff`
- `plan sync`

These are intended to stay deterministic and offline-safe.

Optional assistive commands live under:
- `plan ai ...`

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
plan version --format json
plan init --dir . --format text
plan compile --plan PLAN.md --out .planmark/tmp/plan.json
plan doctor --plan PLAN.md --profile loose --format rich
plan query --plan PLAN.md --format text
plan context <id> --plan PLAN.md --level L0 --format json
plan open <id|node-ref> --plan PLAN.md --format json
plan explain <id> --plan PLAN.md --format rich
plan handoff <id|node-ref> --plan PLAN.md --format json
plan sync [beads|github|linear] --plan PLAN.md --dry-run --format json
```

## Development

Run the test suite:

```bash
go test ./...
```
