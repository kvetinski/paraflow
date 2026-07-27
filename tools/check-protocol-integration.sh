#!/usr/bin/env bash
set -euo pipefail

repository_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
controller_path="${1:-${repository_root}/bin/labctl}"
engine_path="${2:-${repository_root}/target/release/paraflow-engine}"

if [[ "${controller_path}" != /* ]]; then
	controller_path="${repository_root}/${controller_path}"
fi
if [[ "${engine_path}" != /* ]]; then
	engine_path="${repository_root}/${engine_path}"
fi

if [[ ! -x "${controller_path}" ]]; then
	printf 'controller is not executable: %s\n' "${controller_path}" >&2
	exit 1
fi
if [[ ! -x "${engine_path}" ]]; then
	printf 'engine is not executable: %s\n' "${engine_path}" >&2
	exit 1
fi

scratch_dir="$(mktemp -d)"
cleanup() {
	rm -rf -- "${scratch_dir}"
}
trap cleanup EXIT

workloads=(
	"workloads/edge-empty-v1.json"
	"workloads/edge-scalar-v1.json"
)

for workload in "${workloads[@]}"; do
	output_path="${scratch_dir}/$(basename -- "${workload}").out"
	(
		cd -- "${repository_root}"
		"${controller_path}" run \
			--engine "${engine_path}" \
			"${workload}" >"${output_path}"
	)
	if [[ ! -s "${output_path}" ]]; then
		printf 'controller returned no result for %s\n' "${workload}" >&2
		exit 1
	fi
done

printf 'Go-to-Rust protocol integration passed (%d workloads; no timing collected)\n' \
	"${#workloads[@]}"
