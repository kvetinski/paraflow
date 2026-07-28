# Benchmarks

Day 5 introduces executable, versioned benchmark suites.

A suite references semantic workloads and adds only execution and sampling
configuration. It never duplicates or changes workload meaning.

## Suites

- `suites/day05-smoke-v1.json` — one 1K scenario, one warm-up, three retained
  samples; intended for correctness and integration smoke checks.
- `suites/day05-scalar-baseline-v1.json` — 1K, 64K uniform, 64K hotspot, and 1M
  scalar scenarios; intended for the first raw baseline capture. The 64K pair
  changes only the distribution, preserving every other workload setting.

## Run

```bash
make benchmark-smoke
make benchmark-day05
```

The smoke output is disposable. The full command creates a unique file under
`results/raw/` and refuses to overwrite an existing capture.

The Rust process owns warm-ups and sample timing. Go owns process orchestration,
identity capture, strict validation, source alignment, engine-artifact
immutability checks, statistics, and persistence. See
[`docs/specifications/benchmark-v1.md`](../docs/specifications/benchmark-v1.md)
for exact boundaries.

No suite contains a pass/fail latency threshold. Shared CI verifies integrity,
not machine performance.
