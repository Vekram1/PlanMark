# PlanMark Language Reference (v0.2 draft)

PlanMark is a deterministic planning language embedded directly inside Markdown.
It is not a separate fenced-block language and it is not a general-purpose Markdown AST tool.

The current direction is:

- `PLAN.md` remains ordinary Markdown for humans.
- PlanMark promotes a small set of Markdown shapes into deterministic planning semantics.
- Everything else is preserved for provenance and context, even when it does not receive task semantics.

Current implementation note:

- The current parser and metadata attachment code are narrower than this target language contract.
- In the current binary, headings and checkbox lines are the primary recognized source nodes, and metadata attachment is simpler than the scope-based model described here.
- This document describes the intended richer authoring model that later parser and semantic-policy work should implement.

This document defines:

1. what authors write
2. what PlanMark treats as task structure
3. what metadata means
4. what remains human-only context
5. how this improves tracker projection and agent context

---

## 1. Capability Summary

PlanMark v0.2 is intended to treat Markdown as a planning document with explicit structural conventions.

Supported planning primitives:

- tasks expressed as checklist items
- tasks expressed as section headings
- line-oriented metadata attached to a task or section scope
- dependency lists via `@deps`
- acceptance criteria via `@accept`
- rationale and constraints via `@why`, `@risk`, `@rollback`, `@assume`, `@invariant`, `@non_goal`
- nested checklist items as ordered execution steps by default
- free-form prose, tables, and code fences preserved as contextual evidence inside a task scope

Non-goals:

- no implicit command execution
- no semantic guessing from arbitrary prose alone
- no requirement that every Markdown block become a semantic object
- no tracker-specific authoring syntax

---

## 2. Authoring Model

Under the intended richer model, authors write normal Markdown and follow a few explicit task-shaping rules.

### 2.1 Task shapes

A task may be authored in one of two canonical forms.

Checkbox task:

```md
- [ ] Add migration
  @id api.migrate
  @horizon now
  @deps api.schema
  @accept cmd:go test ./...
```

Heading task:

```md
## Add migration
@id api.migrate
@horizon now
@deps api.schema
@accept cmd:go test ./...

We need additive rollout before removing legacy reads.
```

Use a checkbox task when the task is a compact work item.
Use a heading task when the task needs scoped prose, tables, examples, or nested steps.
A heading becomes a task only when task metadata or an explicit semantic promotion rule says it does; a bare heading remains structural context.

### 2.2 Task scope

Task scope is the bounded region of Markdown that belongs to the task.

- For a checkbox task, scope begins at the checkbox line and includes directly attached metadata plus indented child content.
- For a heading task, scope begins at the heading and extends until the next heading of the same or higher level.
- Blocks inside a task scope are available for retrieval and context even when they are not promoted into task semantics.

### 2.3 Planning-first rule

The minimum viable task remains intentionally cheap:

- checkbox or heading title
- optional `@id`
- optional `@horizon`

Additional metadata can be added as the task becomes execution-ready.
For heading tasks, some task metadata is expected so the heading is promoted as a task rather than treated as plain section structure.

---

## 3. Metadata Rules

Metadata is line-oriented and uses `@key value` syntax.

Example:

```md
## Add migration
@id api.migrate
@horizon now
@deps api.schema,api.verify
@accept cmd:go test ./...
@rollback restore pre-migration snapshot and revert migration file
```

Canonical keys:

- `@id`
- `@horizon`
- `@deps`
- `@accept`
- `@why`
- `@touches`
- `@non_goal`
- `@risk`
- `@rollback`
- `@assume`
- `@invariant`

Unknown keys are preserved as opaque metadata.

### 3.1 Ownership rule

In the intended richer model, metadata belongs to the nearest enclosing task or section scope, not merely the nearest preceding line.

In practice:

- metadata directly under a checkbox belongs to that checkbox task
- metadata directly under a heading belongs to that heading task or section
- metadata inside nested list indentation belongs to the nested item it is indented under
- metadata before the first attachable block remains unattached and should produce a diagnostic

### 3.2 Repeated keys

The expected merge behavior is:

- repeatable keys append in source order: `@deps`, `@accept`, `@touches`
- scalar keys are last-wins with deterministic overwrite behavior: `@horizon`, `@why`

Authors should still prefer one scalar metadata line per scope.

---

## 4. Nested Checklist Semantics

Nested checklists improve execution detail without forcing extra tracker items by default.

Example:

```md
## Add migration
@id api.migrate
@horizon now
@accept cmd:go test ./...

- [ ] Write additive migration
- [ ] Verify rollback path
- [ ] Run local validation
```

Default interpretation:

- the heading is the task
- nested checklist items are execution steps within the task
- the steps are preserved as scoped structured content for context and issue rendering

Nested checklist items should only become child tasks under an explicit future policy.

That default keeps authoring cheap and prevents accidental task explosion.

---

## 5. Free-Form Content Inside A Task

Free-form Markdown remains valuable even when it does not become a typed semantic field.

These blocks are preserved inside task scope:

- paragraphs
- tables
- blockquotes
- code fences
- diagrams
- mixed notes

Example:

~~~md
## Add migration
@id api.migrate
@horizon now

We need additive rollout because old workers may still read legacy columns.

| phase | requirement |
| --- | --- |
| deploy | additive only |
| cleanup | after verification |

```sql
ALTER TABLE ...
```
~~~

The prose, table, and SQL snippet may not all become first-class semantic fields, but they remain part of the task's bounded context and can be exported to handoff packets or tracker bodies.

---

## 6. Ambiguity Rules

PlanMark should prefer explicit structure over heuristics.

Author guidance:

- put metadata directly inside the task scope it belongs to
- use headings to create broad task scopes
- use nested lists for steps
- avoid leaving metadata floating between unrelated sections

Tool behavior:

- ambiguous content is preserved, never silently dropped
- ambiguous metadata should remain attached only when deterministic ownership rules resolve it cleanly
- otherwise it remains unattached with a stable diagnostic

---

## 7. What Authors Must Do

If you want predictable machine behavior, write plans with these habits:

- make each real work item either a checkbox task or a heading task
- keep machine-relevant metadata inside that task's scope
- use explicit `@id` for anything that will be synced to a tracker or depended on by another task
- add `@accept` before marking a `@horizon now` task execution-ready
- put rationale, risks, and rollback notes inside the task section instead of in distant prose
- use nested checklist items for steps, not separate top-level tasks, unless they truly need independent lifecycle

---

## 8. Why This Helps

This authoring model improves three things at once.

Tracker projection:

- a task can project not just title and metadata, but also scoped paragraphs, steps, and evidence blocks
- the same semantic task can render into Beads, GitHub Issues, Linear, or Jira without changing the source authoring style

Agent context:

- `context`, `open`, `explain`, and `handoff` can return the actual task scope, not just one checklist line
- rationale, steps, rollback notes, and scoped evidence become available without ad hoc scraping

Human authoring:

- plans stay readable as Markdown
- authors do not need to learn a second fenced DSL
- richer detail can be added incrementally without breaking deterministic extraction

---

## 9. Summary

The intended contract is:

- write normal Markdown
- define tasks with checkboxes or headings
- keep metadata inside task scope
- use nested checklists as steps by default
- let free-form Markdown remain human-readable but context-preserving

PlanMark should parse Markdown broadly enough to preserve structure, but derive semantics narrowly enough to stay deterministic.
