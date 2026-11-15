#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
output=$(mktemp -d)
trap 'rm -rf "$output"' EXIT
VERSION=0.1.0-test COMMIT=deadbeef BUILD_DATE=2026-11-15T00:00:00Z ./scripts/build.sh "$output/hospitality"
expected='hospitality 0.1.0-test commit=deadbeef built=2026-11-15T00:00:00Z'
[[ "$("$output/hospitality" --version)" == "$expected" ]]
echo 'Executable build metadata verified'
