#!/usr/bin/env bash
set -euo pipefail

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repository_root"

schema_path="contracts/workload-v1.schema.json"
expected_schema="paraflow.workload/v1"

jq empty "$schema_path" >/dev/null

schema_version="$(
  jq --raw-output '.properties.schema_version.const' "$schema_path"
)"
if [[ "$schema_version" != "$expected_schema" ]]; then
  printf 'unexpected schema identifier in %s: %s\n' \
    "$schema_path" "$schema_version" >&2
  exit 1
fi

mapfile -t workload_files < <(
  find workloads -type f -name '*.json' -print | sort
)
if [[ "${#workload_files[@]}" -eq 0 ]]; then
  printf 'no workload fixtures found\n' >&2
  exit 1
fi

for workload_path in "${workload_files[@]}"; do
  jq empty "$workload_path" >/dev/null

  workload_schema="$(jq --raw-output '.schema_version' "$workload_path")"
  if [[ "$workload_schema" != "$expected_schema" ]]; then
    printf 'unexpected schema identifier in %s: %s\n' \
      "$workload_path" "$workload_schema" >&2
    exit 1
  fi

  jq --exit-status '
    (."$schema" == "../contracts/workload-v1.schema.json") and
    (.name | type == "string" and length > 0) and
    (.dataset.record_count | type == "number") and
    (.dataset.category_count | type == "number" and . > 0) and
    (.pipeline | has("normalize") and has("score") and
      has("filter") and has("aggregate"))
  ' "$workload_path" >/dev/null
done

printf 'contract syntax and fixture identity checks passed (%d workload(s))\n' \
  "${#workload_files[@]}"
