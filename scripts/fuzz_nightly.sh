#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export TZ=UTC
export LC_ALL=C
export LANG=C
export GOCACHE="${GOCACHE:-$ROOT_DIR/.planmark/.gocache}"

FUZZ_TIME="${FUZZ_TIME:-60s}"

echo "[fuzz-nightly] running extended fuzz suite (${FUZZ_TIME})"
go test ./internal/compile -run FuzzMetadataParse -fuzz=FuzzMetadataParse -fuzztime="$FUZZ_TIME" -count=1
go test ./internal/compile -run FuzzMetadataAttach -fuzz=FuzzMetadataAttach -fuzztime="$FUZZ_TIME" -count=1
go test ./internal/compile -run FuzzSpanRecovery -fuzz=FuzzSpanRecovery -fuzztime="$FUZZ_TIME" -count=1
go test ./internal/compile -run FuzzCompileDoesNotPanic -fuzz=FuzzCompileDoesNotPanic -fuzztime="$FUZZ_TIME" -count=1

