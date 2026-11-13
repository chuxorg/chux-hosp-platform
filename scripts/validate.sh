#!/usr/bin/env bash
set -euo pipefail

echo "Hospitality bootstrap validation"
for file in README.md AGENTS.md docs/SDLC.md .github/pull_request_template.md; do
  if [[ ! -s "$file" ]]; then
    echo "ERROR: missing or empty $file" >&2
    exit 1
  fi
done

# Verify no clearly prohibited raw legacy/database artifacts are tracked.
if git ls-files | grep -E '\.(frm|MYD|MYI|ibd|sqlite|sqlite3|db|sql|zip|tar|gz)$' >/dev/null; then
  echo "ERROR: tracked database/archive file; review repository policy" >&2
  exit 1
fi

# This is a configuration/docs-only validation. It is NOT an application test suite.
if [[ -d src || -d app || -d packages ]]; then
  echo "ERROR: application code detected but no stack-specific unit-test command configured." >&2
  echo "Add the real language-specific test/lint commands and CI coverage in the first code PR." >&2
  exit 1
fi
echo "Bootstrap docs/policy validation passed (no app unit tests exist yet)."
