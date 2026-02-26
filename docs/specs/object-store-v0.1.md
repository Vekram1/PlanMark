# PlanMark Object Store Contract v0.1

## Status

- Version: `v0.1`
- Scope: local content-addressed object store for immutable PlanMark artifacts

## Purpose

Define deterministic storage semantics for large immutable artifacts (source slices, packet payloads, sync payload bodies) without duplicating bytes across state files.

## CAS Keying

- Object keys are `sha256:<hex>` over canonical bytes.
- Canonical bytes must use the same determinism policy as command payload generation.
- The object store is content-addressed only; path-like names are non-authoritative aliases.

## Layout

- Root: `<state_dir>/objects/`
- Fanout by digest prefix is required for scalability:
  - example: `objects/ab/cd/<full-hex>.blob`
- Sidecar metadata is optional; if present, it is non-canonical and must not alter object identity.

## Write/Read Rules

- Writes are atomic and idempotent:
  - write-if-missing for new digest keys
  - existing keys are never overwritten with different bytes
- Reads are pure and must not mutate object state.
- Missing objects produce machine-readable diagnostics with stable codes.

## References

- Mutable state files (compile manifests, sync manifests, packet indices) reference objects by `sha256:` key.
- Reference holders may include object kind metadata (for example `source_slice`, `context_packet`, `sync_payload`) for reporting and GC visibility.
- References are append/update operations on state manifests; object bytes remain immutable.

## GC Contract

GC is deterministic and policy-driven:

- Mark roots:
  - latest N compile/build manifests (policy-defined)
  - active journals and in-flight sync manifests
  - pinned packet indices selected by retention policy
- Traverse references and mark reachable `sha256:` objects.
- Sweep unreachable objects.
- GC must emit a machine-readable report containing:
  - start/end timestamps
  - marked object count
  - deleted object count
  - bytes reclaimed
  - per-kind counts (if kind metadata is available)

## Safety/Recovery

- GC must not run concurrently with a writer without lock coordination.
- Interrupted GC must be recoverable and must never corrupt live objects.
- Corrupt object bytes must be treated as integrity failures and surfaced with deterministic diagnostics.

## Non-goals

- Remote/distributed object store in `v0.1`
- Deduplication across repositories
- Compression policy standardization in this version

