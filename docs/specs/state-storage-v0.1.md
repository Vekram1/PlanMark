# Local State/Storage Contract v0.1

## Status

- Version: `v0.1`
- Policy identifier: `state_storage/v0.1`
- Scope: deterministic local state layout and lifecycle for PlanMark command execution

## Purpose

This contract defines how PlanMark stores mutable local artifacts so behavior is deterministic, recoverable, and auditable.

It covers:

- state directory layout and ownership
- artifact schema/version requirements
- migration and invalidation behavior
- lock acquisition and lock recovery
- corruption handling and read-only fallback expectations

## State Directory Resolution

State root resolution is deterministic and explicit:

1. `--state-dir <path>` flag (highest precedence)
2. repo-local configured state path (if supported by command/config)
3. default: `<repo-root>/.planmark`

All machine-readable command outputs that depend on state must echo the resolved `state_dir`.

## Canonical Layout

Within `state_dir`, the canonical layout is:

```text
.planmark/
  state_version.json
  build/
    compile-manifest.json
  sync/
    beads-manifest.json
  cache/
    context/
  cas/
    sha256/
  journal/
    sync/
  locks/
```

Notes:

- Commands may add subdirectories only under these top-level namespaces.
- Unknown top-level entries are ignored unless an explicit policy version introduces them.
- Filesystem enumeration must be canonicalized in code (sorted order), never OS-order-dependent.

## Artifact Versioning Rules

Each mutable artifact class has an explicit schema/format version:

- state root: `state_version`
- build manifests: `schema_version`
- sync manifests: `schema_version`
- journal entries: `schema_version`
- cache metadata/index records: `schema_version`

Version checks are deterministic:

- newer unsupported version => explicit diagnostic + safe refusal for that artifact
- older supported version => migrate (if migration path exists) or invalidate/rebuild

Silent best-effort parsing of unknown versions is forbidden.

## Writes and Atomicity

All state mutations use atomic replace semantics:

- write temp file in target directory
- fsync/close as required by platform policy
- rename into place

Partial writes must never be treated as valid state.

Concurrent mutation of the same artifact class requires lock protection.

## Locking and Lock Recovery

Lock files reside under `state_dir/locks/` and include owner metadata:

- process id (pid)
- host identifier
- acquisition timestamp
- command identity

Lock lifecycle:

1. attempt non-blocking acquire
2. if held and active, fail with deterministic lock diagnostic
3. if stale by policy TTL, perform lock recovery flow

lock recovery flow:

- verify staleness using timestamp + optional liveness probe
- emit explicit recovery diagnostic/event
- break stale lock and retry acquisition once

Force unlock behavior (if exposed by CLI) must be explicit and never implicit.

## Migration and Invalidation

When versions or policy hashes change, behavior is deterministic per artifact class:

- migrate in place when a tested migration exists
- otherwise invalidate and rebuild from canonical inputs

invalidation rules:

- cache artifacts invalidated when relevant input hashes or policy versions differ
- manifests invalidated when schema version is unsupported
- journal replay only when journal schema is readable and operation IDs remain valid

Invalidation must be explicit and observable (diagnostic + reason), never silent.

## Corruption Handling

Corrupt or unreadable state artifacts must degrade safely:

- emit stable diagnostic with artifact path and class
- quarantine/remove only the unreadable artifact instance
- rebuild from canonical source inputs where possible

Corruption in local state must not alter canonical outcomes for the same valid inputs.

## Read-Only and Degraded Modes

Commands that can operate without mutation should provide read-only operation:

- if state mutation unavailable, continue with deterministic compute path when safe
- emit status/diagnostic indicating cache/state bypass

Commands requiring mutation (for example sync apply/journal append) must fail explicitly with actionable diagnostics.

## Determinism and Auditing

State behavior is deterministic when:

- command inputs and policy versions match
- effective config hash matches
- canonical hashing/canonical JSON policy is unchanged

Operationally:

- dry-run and apply must derive the same planned operation list
- state updates should be traceable to compile/sync identifiers
- diagnostics for migration, invalidation, and recovery must use stable codes

## Non-Goals

- backward compatibility with pre-`v0.1` experimental local-state formats
- distributed lock coordination across multiple machines
- replacing canonical PLAN/IR truth with local state artifacts
