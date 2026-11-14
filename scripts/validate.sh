#!/usr/bin/env bash
set -euo pipefail

for file in README.md AGENTS.md docs/SDLC.md .github/pull_request_template.md pyproject.toml; do
  [[ -s "$file" ]] || { echo "Missing required file: $file" >&2; exit 1; }
done

if git ls-files | grep -E '\.(frm|MYD|MYI|ibd|sqlite|sqlite3|db|sql|zip|tar|gz)$' >/dev/null; then
  echo "Error: tracked raw database/archive artifact" >&2
  exit 1
fi

python -m compileall -q src tests migrations
python -m pytest -q
ruff check src tests migrations
ruff format --check src tests migrations
