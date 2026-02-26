#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export TZ=UTC
export LC_ALL=C
export LANG=C
export GOCACHE="${GOCACHE:-$ROOT_DIR/.planmark/.gocache}"

echo "[determinism] running golden + deterministic checks"
go test ./... -run 'TestGoldenIRStability|TestCompileDeterminismOnFixtures|TestBuildManifestDeterminism|TestSemanticDiffClassificationStable' -count=1

echo "[determinism] running full test sweep"
go test ./...

