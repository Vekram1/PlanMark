<h1 align="center">
  <br>
  <img width="300" height="300" alt="PlanMark project task overview logo" src="https://github.com/user-attachments/assets/d4be234d-4c58-47cd-a29b-7951fefe5ed2" />
  <br>
  PlanMark
</h1>

<h4 align="center">A deterministic compiler from <code>PLAN.md</code> to issue trackers</h4>

<div align="center">

[![License: MIT](https://img.shields.io/badge/License-MIT-lightblue.svg)](./LICENSE)
![Go](https://img.shields.io/badge/language-Go-00ADD8.svg)
</div>

<p align="center">
  <img src="https://github.com/user-attachments/assets/64f7eac6-20f3-4a16-8c0b-794610a0cf95" alt="PlanMark demo" width="900">
</p>

<p align="center">
  <a href="#why-this-project-exists">Why This Project Exists</a> •
  <a href="#tldr">TL;DR</a> •
  <a href="#quick-example">Quick Example</a> •
  <a href="#installation">Installation</a> •
  <a href="#first-demo">First Demo</a> •
  <a href="#core-commands">Core Commands</a>
</p>

---

<a id="why-this-project-exists"></a>
## Why This Project Exists

Most planning systems drift.

The plan starts as a Markdown file, but the real work gradually moves into issue trackers, chat threads, and ad hoc agent prompts. At that point:
- the tracker becomes the source of truth
- agents scrape prose instead of consuming structure
- provenance is lost
- small edits create confusion about what actually changed

PlanMark keeps `PLAN.md` canonical.

It treats the plan as a source document, compiles it into deterministic machine-usable artifacts, and projects those artifacts into context packets, diagnostics, and trackers without making the tracker canonical.

The core model is:

```text
PLAN.md -> canonical IR -> context / diagnostics / tracker projection
```

More concretely:

```text
                +------------------+
                |     PLAN.md      |
                |  canonical plan  |
                +---------+--------+
                          |
                          v
                +------------------+
                |    Compile to    |
                |   canonical IR   |
                +---------+--------+
                          |
          +---------------+----------------+
          |               |                |
          v               v                v
+----------------+ +---------------+ +------------------+
| Task context   | | Diagnostics   | | Tracker sync     |
| context/open/  | | doctor/query  | | beads/github/    |
| explain/handoff| | changes       | | linear           |
+--------+-------+ +-------+-------+ +---------+--------+
         |                 |                   |
         v                 v                   v
+----------------+ +---------------+ +------------------+
| Agents / users | | Readiness /   | | Beads + bv or    |
| act on tasks   | | drift checks  | | other trackers   |
+----------------+ +---------------+ +------------------+
```

---

<a id="tldr"></a>
## TL;DR

### The Problem

You want to plan real work in Markdown, but:
- raw Markdown is awkward for agents to consume safely
- trackers drift from the source plan
- dependencies and readiness become implicit
- every tool wants to become the source of truth

### The Solution

PlanMark compiles `PLAN.md` into deterministic outputs:
- canonical IR
- task context packets with exact provenance
- readiness and diagnostic output
- tracker projections for systems like Beads
- dependency-aware triage through `bv`

### Why PlanMark

- `PLAN.md` stays canonical
- agents consume structured task context instead of scraping prose
- tracker sync is a projection of the plan, not a replacement for it
- compile, diagnostics, context, and handoff flows are deterministic and offline-safe

---

<a id="quick-example"></a>
## Quick Example

```md
# PLAN

## Stabilize parser
- [ ] Add parser acceptance coverage
  @id parser.acceptance
  @horizon now
  @accept cmd:go test ./internal/compile -run TestCompile

## Ship tracker sync
- [ ] Project tasks into Beads
  @id tracker.sync
  @horizon next
  @deps parser.acceptance
  @accept cmd:go test ./internal/cli -run TestSyncBeadsWritesManifest
```

Compile it:

```bash
planmark compile --plan PLAN.md --out .planmark/tmp/plan.json
```

Get task-local context:

```bash
planmark context tracker.sync --plan PLAN.md --need execute --format json
```

Project it into a tracker:

```bash
planmark sync beads --plan PLAN.md --format json
```

Run dependency-aware triage:

```bash
bv --robot-triage
```

That loop is the whole point: the plan stays canonical, but the rest of the workflow becomes machine-usable.

---

<a id="installation"></a>
## Installation

### Build from source

```bash
git clone https://github.com/Vekram1/PlanMark.git
cd PlanMark
go build -o ./bin/planmark ./cmd/planmark
export PATH="$PWD/bin:$PATH"
planmark version --format json
```

### Install with the project script

macOS or Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/Vekram1/PlanMark/master/scripts/install.sh | bash
planmark version --format json
```

If `planmark` is not on your shell path after install:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

The installer places `planmark` on your path and also installs `plan` as a compatibility alias by default.

### Update in place

macOS or Linux:

```bash
planmark update --check
planmark update
```

Windows PowerShell:

```powershell
powershell -ExecutionPolicy Bypass -c "irm https://raw.githubusercontent.com/Vekram1/PlanMark/master/scripts/install.ps1 | iex"
planmark version --format json
```

Windows release assets are published as `.zip` archives. The PowerShell installer installs `planmark.exe` and `plan.exe` under `%LOCALAPPDATA%\Programs\PlanMark\bin` by default.

---

<a id="first-demo"></a>
## First Demo

Initialize a repo:

```bash
planmark init --dir . --format text
```

This creates repo-local PlanMark state under `.planmark/` and adds starter files such as `PLAN.md`, `.planmark.yaml`, and a managed PlanMark section in `AGENTS.md` when missing.

Compile the initial plan:

```bash
planmark compile --plan PLAN.md --out .planmark/tmp/plan.json
```

Check readiness:

```bash
planmark doctor --plan PLAN.md --profile loose --format rich
```

List tasks:

```bash
planmark query --plan PLAN.md --format text
```

If those commands work, the repo is ready for agent-facing task context.

---

## What PlanMark Understands

PlanMark keeps ordinary Markdown as the authoring surface and derives structure from it.

You do not need a separate DSL. Normal Markdown works, with a small amount of task metadata such as:
- `@id`
- `@horizon`
- `@deps`
- `@accept`

The most common fields are:
- `@id`: a stable task identifier used by context, change detection, and tracker sync
- `@horizon`: when the task should be considered in the execution queue
- `@deps`: task ids that must be completed first
- `@accept`: explicit verification targets such as test commands or file checks

For `@horizon`, the typical values are:
- `now`: actionable work that should be ready for execution now
- `next`: queued work that follows current priorities
- `later`: deferred work that should stay visible but out of the immediate queue

In practice:
- use `now` for tasks an agent or human could pick up immediately
- use `next` for tasks that matter soon but are not the current focus
- use `later` for work you want captured without competing with the near-term queue

When syncing to Beads, PlanMark also maps horizon into native Beads priority so the tracker and `bv` reflect the plan's intended queue:
- `now` -> Beads priority `1`
- `next` -> Beads priority `2`
- `later` -> Beads priority `3`
- any other or missing horizon -> fallback priority `4`

Heading tasks work well when you want rationale, risks, or checklists under the task:

```md
## API rollout

### Add migration
@id api.migrate
@horizon now
@deps api.schema
@accept cmd:go test ./...

We need an additive rollout because older workers may still read legacy columns.

- [ ] Write additive migration
- [ ] Verify rollback path
- [ ] Run local validation
```

Compact checkbox tasks also work:

```md
- [ ] Add migration
  @id api.migrate
  @horizon now
  @deps api.schema
  @accept cmd:go test ./...
```

Use heading tasks when you want nearby context. Use compact tasks when the work item is already small and self-contained.

---

## Plan Context

One of PlanMark's main jobs is turning a task in `PLAN.md` into a deterministic context packet that an agent or operator can actually use.

Instead of forcing callers to read the whole plan every time, PlanMark can emit task-scoped context with exact provenance back to the source plan. That context can include:
- the task id and title
- dependencies
- acceptance criteria
- execution steps
- evidence references
- the exact source slice from `PLAN.md`

The source slice matters. A context packet is not just a summary; it carries provenance such as:
- `source_path`
- `start_line`
- `end_line`
- `slice_hash`
- `slice_text`

That means a caller can see exactly which part of the plan produced the task, rather than relying on brittle string matching or tracker text.

The most common context commands are:

```bash
planmark context api.migrate --plan PLAN.md --need execute --format json
planmark open api.migrate --plan PLAN.md --format json
planmark explain api.migrate --plan PLAN.md --format rich
planmark handoff api.migrate --plan PLAN.md --format json
```

The public interface is need-based:
- `--need execute`: task-local execution context with the exact `PLAN.md` source slice
- `--need edit`: task-local context plus source extracts when the task points to files or ranges
- `--need dependency-check`: task-local context plus dependency-closure context
- `--need handoff`: a handoff-oriented packet for transferring work cleanly

In practice, the right approach is to start small and escalate only when needed:
1. `planmark context <id> --need execute`
2. `planmark open <id|node-ref>`
3. `planmark explain <id>`
4. `planmark context <id> --need edit`
5. `planmark context <id> --need dependency-check`

That keeps context focused while preserving deterministic traceability back to the plan source.


---

<a id="core-commands"></a>
## Core Commands

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

---

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

---

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

---

## Writing Plans That Work Well

Good PlanMark plans usually have these traits:
- important tasks have explicit `@id` values
- rationale sits near the task instead of in a distant section
- `@horizon now` work includes at least one `@accept`
- nested checklists describe execution steps within the task scope
- rollback notes, assumptions, and risks stay attached to the relevant task
- tasks are bounded and specific instead of being a long flat checklist

The goal is not to formalize everything. The goal is to make each task executable with enough nearby context to act safely.

---

## Common Commands

```bash
planmark --help
planmark version --format json
planmark init --dir . --format text
planmark compile --plan PLAN.md --out .planmark/tmp/plan.json
planmark doctor --plan PLAN.md --profile loose --format rich
planmark query --plan PLAN.md --format text
planmark context <id> --plan PLAN.md --need execute --format json
planmark open <id|node-ref> --plan PLAN.md --format json
planmark explain <id> --plan PLAN.md --format rich
planmark handoff <id|node-ref> --plan PLAN.md --format json
planmark sync [beads|github|linear] --plan PLAN.md --dry-run --format json
```

---

## Development

Run the test suite:

```bash
go test ./...
```

---

## License

MIT License. See [LICENSE](LICENSE).
