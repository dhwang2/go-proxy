#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="${1:-$ROOT/.test-overlay.json}"

mkdir -p "$(dirname "$OUT")"

{
  printf '{\n'
  printf '  "Replace": {\n'
  first=1
  while IFS= read -r -d '' file; do
    rel="${file#"$ROOT/_tests/"}"
    orig="$ROOT/$rel"
    if [ "$first" -eq 0 ]; then
      printf ',\n'
    fi
    first=0
    printf '    "%s": "%s"' "$orig" "$file"
  done < <(find "$ROOT/_tests" -name '*_test.go' -print0 | sort -z)
  printf '\n  }\n'
  printf '}\n'
} >"$OUT"

printf '%s\n' "$OUT"
