# PlanMark Fuzzing Policy v0.1

## Status

- Policy ID: `fuzzing/v0.1`
- Scope: deterministic robustness requirements for compile/context path fuzzing
- Canonical dependencies:
  - `docs/specs/planmark-v0.2.md`
  - `docs/specs/ir-v0.2.md`
  - `docs/specs/semantic-derivation-v0.1.md`
  - `docs/specs/change-detection-v0.1.md`

## Purpose

Fuzzing is a contract-enforcement subsystem for tolerance and determinism, not an optional hardening add-on.

## Core Invariants

- Never panic on malformed or adversarial inputs.
- Deterministic for identical input bytes + identical policy versions.
- No silent data drop: candidates are either attached/accepted or retained with explicit diagnostics.
- Resource limits are explicit (file size, line length, node count, metadata density) and failures are deterministic.

## Mandatory Contract Statements

- "Never panic" is a hard invariant for canonical-path fuzz targets.
- "Crash => regression fixture" is mandatory: every minimized crash input must be checked into regression fixtures and promoted to non-fuzz tests.

## Required Fuzz Targets (v0.1)

- metadata parsing (`@key` line handling, unknown keys, malformed payloads)
- metadata attachment determinism
- source-span/slice recovery behavior
- compile end-to-end tolerance (`Compile` should never panic)
- path and pin safety surfaces once context modules exist

## Seed Corpus Policy

- Canonical seed inputs include:
  - `testdata/malformed/`
  - `testdata/plans/`
- Seed updates must remain deterministic and reviewable.
- CRLF/LF and Unicode normalization edge inputs are required in seed sets.

## Regression Fixture Policy

- Crash => regression fixture.
- Minimized failing cases are stored under `testdata/fuzz/regressions/`.
- Each regression fixture must have:
  - stable file content
  - a deterministic test hook (non-fuzz) to prevent reintroduction
  - short metadata note describing prior failure mode

## CI and Runtime Budget Policy

- CI runs short fuzz budgets (for example 5-10 seconds per target) for smoke-level detection.
- Longer fuzz budgets run in local/nightly jobs and persist expanded corpora under `testdata/fuzz/corpus/`.
- Fuzz CI should avoid non-deterministic flakiness (locale/time/random side effects must be controlled).

## Non-Goals

- Exhaustive adversarial threat modeling in MVP.
- Guaranteeing complete bug absence via fuzzing.
