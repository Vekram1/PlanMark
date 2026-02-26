# Accept Verify Receipt Schema v0.1

## Status

- Version: `v0.1`
- Policy identifier: `accept_verify_receipt/v0.1`
- Scope: machine-readable receipt emitted by explicit `plan verify-accept` execution modes

## Purpose

Verification receipts provide an auditable record of explicit acceptance-command execution without making command execution part of the canonical compile/doctor/context/sync path.

For acceptance tracking, receipt schema defines command digest, policy, exit status, and capture metadata.

## Receipt Envelope

Each receipt is a single JSON object with deterministic key structure:

- `schema_version`: string (`"v0.1"`)
- `tool_version`: string (Plan CLI version)
- `receipt_id`: string (stable unique identifier for this execution receipt)
- `created_at`: RFC3339 UTC timestamp
- `command`: command invocation record
- `policy`: execution policy record
- `result`: execution result record
- `capture`: output capture metadata
- `context`: optional execution context metadata

## Command Record

The `command` object must include:

- `task_id`: string
- `accept_index`: integer (index of selected `@accept` entry for the task)
- `accept_text`: string (verbatim selected acceptance line)
- `command_text`: string (parsed executable command text)
- `command_digest`:
  - `algorithm`: `"sha256"`
  - `value`: lowercase hex digest over canonical command bytes

Command digest normalization:

- UTF-8 bytes
- no hidden trimming beyond parser-defined normalization
- deterministic across platforms for identical acceptance source bytes

## Policy Record

The `policy` object records execution policy used at run time:

- `policy_version`: string
- `cwd_policy`: string (for example `repo-root`)
- `env_policy`: string (for example `allowlist`, `clean-env`, `inherited`)
- `timeout_ms`: integer
- `network_policy`: string
- `sandbox_policy`: string

Unknown policy dimensions may be added in future schema versions; v0.1 consumers should reject unknown schema versions rather than guess behavior.

## Result Record

The `result` object includes:

- `exit_status`: integer
- `started_at`: RFC3339 UTC timestamp
- `finished_at`: RFC3339 UTC timestamp
- `duration_ms`: integer
- `status`: enum:
  - `ok`
  - `failed`
  - `timeout`
  - `policy_blocked`
  - `internal_error`

`status` is semantic, `exit_status` is process-level; both are required for machine branching.

## Capture Metadata

The `capture` object records what was captured and how:

- `stdout`:
  - `captured`: boolean
  - `bytes`: integer
  - `digest`: optional digest object (`algorithm`, `value`)
  - `truncated`: boolean
- `stderr`:
  - same fields as `stdout`
- `redaction`:
  - `applied`: boolean
  - `policy`: optional string
  - `notes`: optional array of strings

v0.1 does not standardize full stdout/stderr payload storage format.

## Optional Context Record

The `context` object may include:

- `repo_root`: string
- `plan_path`: string
- `state_dir`: string
- `host`: optional string
- `runner_id`: optional string

These fields are informational and do not change canonical command/result semantics.

## Determinism Rules

- Receipts use stable field names and explicit schema version.
- Digest algorithms are explicit.
- Enum values are closed for `v0.1`.
- Unknown `schema_version` must be treated as unsupported.
- Timestamps are required for auditability but do not affect canonical PLAN semantics.

## Minimal Example

```json
{
  "schema_version": "v0.1",
  "tool_version": "0.1.0",
  "receipt_id": "rcpt_20260226_001",
  "created_at": "2026-02-26T04:00:00Z",
  "command": {
    "task_id": "planmark.e2e.compile_fixture",
    "accept_index": 0,
    "accept_text": "@accept cmd:go run ./cmd/plan compile --plan testdata/plans/mixed.md --out .planmark/tmp/plan.json",
    "command_text": "go run ./cmd/plan compile --plan testdata/plans/mixed.md --out .planmark/tmp/plan.json",
    "command_digest": {
      "algorithm": "sha256",
      "value": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
    }
  },
  "policy": {
    "policy_version": "v0.1",
    "cwd_policy": "repo-root",
    "env_policy": "allowlist",
    "timeout_ms": 60000,
    "network_policy": "disabled",
    "sandbox_policy": "workspace-write"
  },
  "result": {
    "exit_status": 0,
    "started_at": "2026-02-26T04:00:01Z",
    "finished_at": "2026-02-26T04:00:04Z",
    "duration_ms": 3000,
    "status": "ok"
  },
  "capture": {
    "stdout": {
      "captured": true,
      "bytes": 512,
      "digest": {
        "algorithm": "sha256",
        "value": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
      },
      "truncated": false
    },
    "stderr": {
      "captured": true,
      "bytes": 0,
      "truncated": false
    },
    "redaction": {
      "applied": false
    }
  },
  "context": {
    "repo_root": "/repo",
    "plan_path": "/repo/PLAN.md",
    "state_dir": "/repo/.planmark"
  }
}
```

## Non-goals

- Standardizing stdout/stderr payload formats across all future executors
- Defining transport protocol for uploading receipts to remote services
- Making receipt emission mandatory for canonical non-execution commands
