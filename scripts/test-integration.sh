#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
export GOTOOLCHAIN="go$(cat .go-version)"
: "${TEST_DATABASE_URL:?Set TEST_DATABASE_URL to a dedicated PostgreSQL database ending in _test}"
# Tests create/drop only their own fresh database, never the supplied database.
go test -race -count=1 -tags=integration ./...
