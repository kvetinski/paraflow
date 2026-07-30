#!/usr/bin/env bash
set -euo pipefail

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
version="$(tr -d '\r\n' <"${repository_root}/VERSION")"
labctl="${repository_root}/bin/labctl"
engine="${repository_root}/target/release/paraflow-engine"

if [[ ! "${version}" =~ ^0\.[0-9]+\.[0-9]+$ ]]; then
  printf 'VERSION must be a canonical Week 1 semantic version; found %q\n' "${version}" >&2
  exit 1
fi

labctl_output="$("${labctl}" version)"
if [[ "${labctl_output}" != "labctl ${version} "* ]]; then
  printf 'labctl version mismatch: %s\n' "${labctl_output}" >&2
  exit 1
fi

engine_output="$("${engine}" version)"
if [[ "${engine_output}" != "paraflow-engine ${version}" ]]; then
  printf 'engine version mismatch: %s\n' "${engine_output}" >&2
  exit 1
fi

metadata_path="$(mktemp)"
trap 'rm -f "${metadata_path}"' EXIT
(
  cd "${repository_root}"
  cargo metadata --locked --no-deps --format-version 1 >"${metadata_path}"
)
node - "${version}" "${metadata_path}" <<'NODE'
const fs = require("node:fs");
const expected = process.argv[2];
const metadata = JSON.parse(fs.readFileSync(process.argv[3], "utf8"));
const mismatches = metadata.packages
  .filter((pkg) => pkg.version !== expected)
  .map((pkg) => `${pkg.name}=${pkg.version}`);
if (mismatches.length !== 0) {
  console.error(`Cargo package version mismatch: ${mismatches.join(", ")}`);
  process.exit(1);
}
NODE

printf 'version %s is aligned across VERSION, Go, Rust, and Cargo metadata\n' "${version}"
