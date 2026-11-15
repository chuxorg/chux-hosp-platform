#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
export GOTOOLCHAIN="go$(cat .go-version)"
VERSION=${VERSION:-dev}
COMMIT=${COMMIT:-$(git rev-parse HEAD 2>/dev/null || printf unknown)}
# Commit time keeps identical inputs reproducible.
BUILD_DATE=${BUILD_DATE:-$(git show -s --format=%cI HEAD 2>/dev/null || printf unknown)}
for value in "$VERSION" "$COMMIT" "$BUILD_DATE"; do
  [[ "$value" =~ ^[a-zA-Z0-9.+:_-]+$ ]] || { echo 'Invalid build metadata' >&2; exit 1; }
done
output=${1:-build/hospitality}
mkdir -p "$(dirname "$output")"
CGO_ENABLED=0 go build -trimpath -buildvcs=false \
  -ldflags "-s -w -X main.version=$VERSION -X main.commit=$COMMIT -X main.buildDate=$BUILD_DATE" \
  -o "$output" ./cmd/hospitality
