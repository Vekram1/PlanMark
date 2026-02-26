# Pack Layout Contract v0.1

## Status

- Version: `v0.1`
- Policy identifier: `pack_layout/v0.1`
- Scope: deterministic on-disk/export layout for `plan pack`

## Purpose

This contract defines how PlanMark exports context packs so they are portable, reproducible, and machine-readable.

It specifies:

- canonical directory layout for unpacked packs
- tarball packaging requirements
- `index` schema and ordering rules
- deterministic `node_ref` resolution semantics inside a pack

## Pack Identity

Each pack has a deterministic identity tuple:

- `plan_path`
- `compile_id`
- selected task set (`ids` or derived selection mode)
- packet level (`L0|L1|L2`)
- effective policy/config hashes used by the export

The pack identity must be recorded in the pack `index` and used for cache/traceability decisions.

## Canonical Unpacked Layout

Unpacked exports use this canonical structure:

```text
pack/
  index.json
  plan/
    plan.json
    compile-manifest.json
  packets/
    <task-id>/
      l0.json
      l1.json
      l2.json
  blobs/
    sha256/
      <aa>/<digest>
```

Rules:

- `index.json` is mandatory.
- Packet files are present only for requested levels.
- `blobs/sha256` stores immutable content-addressed payloads referenced by `index`.
- Path separators in `index` paths are normalized to `/`.

## Tarball Layout

When exporting `tar.gz`:

- archive root must be `pack/`.
- entries must be emitted in canonical sorted path order.
- file modes and mtimes must be normalized for deterministic bytes.
- gzip header fields that introduce nondeterminism (timestamps/original name) must be normalized.

## Index Schema (`index.json`)

Top-level shape:

- `schema_version` (required, `v0.1`)
- `pack_id` (required)
- `plan_path` (required)
- `compile_id` (required)
- `levels` (required, sorted unique values)
- `tasks` (required, sorted by `task_id`)
- `blobs` (required, sorted by digest)

Task entry shape:

- `task_id`
- `node_ref`
- `packet_paths` (map of level -> relative path)
- `packet_hashes` (map of level -> `sha256:<hex>`)

Blob entry shape:

- `digest` (`sha256:<hex>`)
- `path` (relative path under `blobs/sha256/...`)
- `kind` (`plan_json|compile_manifest|packet|pin_extract|other`)

## Deterministic Ordering Rules

All enumerations are canonicalized in code:

- tasks: sort by `task_id` ascending
- levels: sort by `L0`, `L1`, `L2`
- blob entries: sort by `digest`
- map-like fields serialized via canonical JSON encoder

No output ordering may depend on filesystem iteration order.

## `node_ref` Resolution Inside a Pack

`node_ref` remains the canonical handle for source-node identity and must not be rewritten in pack export.

Resolution rules:

1. `tasks[].node_ref` points to the same `node_ref` value emitted by canonical compile output.
2. Consumers resolve `node_ref` by reading `plan/plan.json` source nodes first.
3. If a task references a missing `node_ref`, pack validation fails (deterministic error).
4. Line ranges are provenance only; they do not replace `node_ref` identity.

## Validation Requirements

A valid pack requires:

- `index.schema_version == "v0.1"`
- every `packet_hashes[level]` matches packet content bytes
- every referenced blob digest exists at `blobs` path and matches bytes
- every task `node_ref` resolves in `plan/plan.json`

Validation failures are machine-readable and deterministic.

## Compatibility and Evolution

- Any breaking `index` schema change requires a schema version bump.
- Additive fields may be introduced in newer versions if old required fields remain stable.
- Readers must reject unsupported newer schema versions explicitly.

## Non-Goals

- defining transport/signing policy for remote distribution
- embedding tracker runtime state as canonical pack truth
- replacing canonical PLAN/IR storage with pack artifacts
