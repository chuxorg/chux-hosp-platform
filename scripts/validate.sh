#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
export GOTOOLCHAIN="go$(cat .go-version)"
for file in README.md AGENTS.md docs/SDLC.md .github/pull_request_template.md go.mod go.sum .go-version; do
  [[ -s "$file" ]] || { echo "Missing required file: $file" >&2; exit 1; }
done
# Only reviewed versioned migrations are allowed; this is not a secret scanner.
while IFS= read -r -d '' file; do
  lower=$(printf '%s' "$file" | tr '[:upper:]' '[:lower:]')
  if [[ "$lower" =~ \.(frm|myd|myi|ibd|sqlite|sqlite3|db|sql|zip|tar|gz|bak)$ ]]; then
    [[ "$file" =~ ^migrations/[0-9]{5}_[a-z0-9_]+\.sql$ ]] || {
      echo "Error: prohibited tracked database/archive artifact: $file" >&2; exit 1;
    }
  fi
done < <(git ls-files -z --cached --others --exclude-standard)
unformatted=$(gofmt -l cmd internal migrations)
[[ -z "$unformatted" ]] || { printf 'Run gofmt on:\n%s\n' "$unformatted" >&2; exit 1; }
go mod verify
go vet ./...
go test ./...
./scripts/build.sh
if [[ "${FULL_VALIDATION:-0}" == 1 ]]; then
  go test -race ./...
  ./scripts/test-integration.sh
fi
