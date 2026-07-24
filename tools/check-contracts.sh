#!/usr/bin/env bash
set -euo pipefail

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repository_root"

if ! command -v node >/dev/null 2>&1; then
  printf 'node is required for JSON Schema validation\n' >&2
  exit 1
fi

if ! command -v npm >/dev/null 2>&1; then
  printf 'npm is required for reproducible schema-validator installation\n' >&2
  exit 1
fi

npm --prefix tools/schema-check ci \
  --ignore-scripts \
  --no-audit \
  --no-fund \
  --cache "$repository_root/tools/schema-check/.npm-cache" \
  --logs-dir "$repository_root/tools/schema-check/.npm-logs"
npm --prefix tools/schema-check test
