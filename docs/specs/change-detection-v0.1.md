# PlanMark Change Detection Policy v0.1

## Status

- Policy ID: `change_detection/v0.1`
- Scope: deterministic classification of task-level semantic changes between compiles
- Canonical dependencies:
  - `docs/specs/ir-v0.3.md`
  - `docs/specs/semantic-derivation-v0.4.md`

## Purpose

Define a mechanical, replayable answer to "what changed?" that is stable across environments and independent from prose-only diffs.

## Canonical Inputs

- Prior compile semantic state (or prior manifest-linked semantic fingerprints)
- Current compile semantic state
- Active change-detection policy version

Optional/advisory input:

- VCS hunk data (for UX narrowing and explanations only)

## Canonical vs Advisory Sources

- Canonical truth is derived from semantic identity + fingerprint deltas.
- Git/VCS diff data is never the canonical classifier input.
- In non-git contexts, results remain correct and deterministic.

## Identity Model for Diffing

- Primary identity key: task `@id`.
- Missing explicit IDs use deterministic derived identities per semantic policy.
- Explicit identity-evolution annotations (for example `@supersedes <old-id>`) may map old->new identities when policy allows.
- Without explicit identity-evolution metadata, ID changes are classified as `deleted` + `added`.

## Required Change Classes

Policy v0.1 supports deterministic classes:

- `added`
- `deleted`
- `modified`
- `moved`
- `metadata_changed`
- `deps_changed`
- `accept_changed`
- `horizon_changed`

Implementation may emit one primary class plus stable secondary tags, but output ordering and enums must remain deterministic.

## Classification Rules (v0.1)

1. Entity set comparison
- Build stable sorted identity sets for prior/current semantic entities.
- IDs present only in current => `added`.
- IDs present only in prior => `deleted` unless explicit identity-evolution mapping consumes them.

1. Matched-identity comparison
- Compare canonical semantic fingerprints for matched identities.
- Equal fingerprints => no-op (excluded from change list unless verbose mode is requested).
- Unequal fingerprints => classify by first matching deterministic rule below.

1. Deterministic precedence (matched entities)
- If only horizon field changed => `horizon_changed`.
- Else if only deps changed => `deps_changed`.
- Else if only acceptance payload changed => `accept_changed`.
- Else if structure/content-equivalent payload moved under a different source anchor/path span => `moved`.
- Else if only metadata fields changed => `metadata_changed`.
- Else => `modified`.

Tie-breaking and ambiguous matches must use fixed precedence and stable deterministic ordering.

## Output Stability Requirements

- For identical prior/current semantic states and policy version, output is byte-identical.
- Change list ordering is stable (for example by primary class, then identity, then source provenance).
- JSON enums and field names are versioned machine contract surfaces.

## Non-Goals

- Fully incremental parsing in MVP.
- Heuristic rename/split inference without explicit identity-evolution metadata.
