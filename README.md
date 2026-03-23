# PlanMark

PlanMark turns a `PLAN.md` file into deterministic, machine-usable planning artifacts.

If you want an agent to work from a plan without making the tracker or the model output canonical, this is the tool:

- you write `PLAN.md`
- PlanMark compiles it into structured IR
- PlanMark checks readiness and blockers
- PlanMark produces task context and handoff packets for agents
- PlanMark can project that same plan into a tracker without making the tracker the source of truth

`PLAN.md` stays canonical.

## What A New User Should Expect

You do not need to learn a separate DSL (Domain Specific Language).
You write normal Markdown with a small amount of structured metadata.

The basic workflow is:

1. initialize PlanMark in your repo
2. write a small `PLAN.md`
3. run `plan compile`
4. run `plan doctor`
5. ask for task context with `plan context` or `plan handoff`
6. optionally preview tracker sync with `plan sync --dry-run`

If those steps work, your agent already has something solid to operate on.

## Install

From a source checkout:

```bash
go build -o ./bin/plan ./cmd/plan
export PATH="$PWD/bin:$PATH"
plan version --format json
```

Global install on macOS/Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/Vekram1/PlanMark/master/scripts/install.sh | bash
plan version --format json
```

If `plan` is not found after install, add the installer path to your shell:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

## First-Time Quickstart

Initialize the current repo once:

```bash
plan init --dir . --format text
```

This creates local PlanMark state under `.planmark/` and, if missing, starter files such as `PLAN.md` and `.planmark.yaml`.

Now replace `PLAN.md` with something small and real.

## Write Your First `PLAN.md`

This is a good starting example for an API rollout feature:

```md
# PLAN

## API rollout

### Add migration
@id api.migrate
@horizon now
@deps api.schema
@accept cmd:go test ./...
@rollback restore snapshot and revert migration file

We need additive rollout because older workers may still read legacy columns.

- [ ] Write additive migration
- [ ] Verify rollback path
- [ ] Run local validation

### Validate schema assumptions
@id api.schema
@horizon next

Confirm the new columns are additive and safe for older readers.
```

What matters here:

- each real task has a clear title
- tasks that will be referenced or synced get an explicit `@id`
- work you want agents to execute now gets `@horizon now`
- execution-ready tasks should have at least one `@accept`
- nested checklist items become ordered steps for the parent task
- free-form prose stays useful context for the agent

You can also use a compact checkbox task style:

```md
- [ ] Add migration
  @id api.migrate
  @horizon now
  @deps api.schema
  @accept cmd:go test ./...
```

Use heading tasks when you want rationale, tables, examples, or steps under the task.
Give a heading task explicit task metadata such as `@id` or `@horizon`; a bare heading is just section structure.

## Run The Core Workflow

Compile the plan:

```bash
plan compile --plan PLAN.md --out .planmark/tmp/plan.json
```

This gives you deterministic IR derived from `PLAN.md`.

Check for blockers and readiness problems:

```bash
plan doctor --plan PLAN.md --profile loose --format rich
```

For stricter execution gating on `@horizon now` work:

```bash
plan doctor --plan PLAN.md --profile exec --format rich
```

List tasks:

```bash
plan query --plan PLAN.md --format text
```

You should now be able to see your tasks, IDs, and readiness state.

## Get Agent-Usable Task Context

For an agent, the most useful commands are usually these:

Task context packet:

```bash
plan context api.migrate --plan PLAN.md --level L0 --format json
```

Exact source lookup:

```bash
plan open api.migrate --plan PLAN.md --format json
```

Why a task looks the way it does:

```bash
plan explain api.migrate --plan PLAN.md --format rich
```

Task handoff packet:

```bash
plan handoff api.migrate --plan PLAN.md --format json
```

These are the outputs an agent should usually consume first, rather than scraping `PLAN.md` directly.

Recommended escalation path:

1. `plan context <id> --level L0`
2. `plan open <id>`
3. `plan explain <id>`
4. `plan context <id> --level L1`
5. `plan context <id> --level L2`

That keeps agent context small while preserving deterministic traceability back to source.

## Preview Tracker Sync

PlanMark can project tasks into a tracker, but the tracker is not canonical.
`PLAN.md` remains the source of truth.

Dry-run Beads sync:

```bash
plan sync beads --plan PLAN.md --dry-run --format json
```

Dry-run GitHub proof adapter:

```bash
plan sync github --plan PLAN.md --dry-run --format json
```

You can also select the adapter and render profile explicitly:

```bash
plan sync --plan PLAN.md --adapter github --profile compact --dry-run --format json
```

Current built-in render profiles:

- `default`
- `compact`
- `agentic`
- `handoff`

Current proven adapters:

- `beads`
- `github`

## Optional Repo Config

You can set default tracker selection in `.planmark.yaml`:

```yaml
tracker:
  adapter: beads
  profile: default
```

Then `plan sync --plan PLAN.md --dry-run --format json` will use those defaults.

## What Makes A Good Plan For Agents

If you want good results from agents, write plans with these habits:

- give important tasks explicit `@id`s
- keep rationale near the task, not in a distant section
- add `@accept` before asking an agent to execute `@horizon now` work
- use nested checklists for steps
- keep risks and rollback notes inside the task scope
- prefer a few well-scoped tasks over a long flat checklist with no metadata

The goal is not to encode everything.
The goal is to make each task a bounded unit of work with enough nearby context to act on safely.

## Canonical vs Non-Canonical

Canonical deterministic commands:

- `plan compile`
- `plan doctor`
- `plan context`
- `plan open`
- `plan explain`
- `plan handoff`
- `plan sync`

These are intended to remain deterministic and offline-safe.

AI helpers are optional and non-canonical:

- `plan ai ...`

Use them as assistive tooling, not as the source of truth.

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
plan sync [beads|github] --plan PLAN.md --dry-run --format json
```

## Current Limitations

- `PLAN.md` is the canonical source, so tracker changes are projection/runtime state, not plan edits
- only `beads` and `github` are currently proven adapters
- richer Markdown support is implemented conservatively; PlanMark still promotes only a narrow set of planning shapes into semantics
- AI helper output is non-canonical and should be reviewed before applying anything

## Development

Run the test suite:

```bash
go test ./...
```
