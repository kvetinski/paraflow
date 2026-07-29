# Benchmarks

Day 5 introduces executable fused benchmark suites. Day 6 reuses the same suite
shape to pair that denominator with an explicit scalar stage profile.

A suite references semantic workloads and adds only execution and sampling
configuration. It never duplicates or changes workload meaning.

## Suites

- `suites/day05-smoke-v1.json` — one 1K scenario, one warm-up, three retained
  samples; intended for correctness and integration smoke checks.
- `suites/day05-scalar-baseline-v1.json` — 1K, 64K uniform, 64K hotspot, and 1M
  scalar scenarios; intended for the first raw baseline capture. The 64K pair
  changes only the distribution, preserving every other workload setting.
- `suites/day06-profile-smoke-v1.json` — one small paired fused/profile
  integration scenario with no latency threshold.
- `suites/day06-scalar-profile-v1.json` — the same controlled sizes and
  distributions as Day 5, measured through both the unchanged fused path and
  the diagnostic materialized stage-pass topology.

## Run

```bash
make benchmark-smoke
make benchmark-day05
make profile-smoke
make profile-day06
```

The smoke output is disposable. The full command creates a unique file under
`results/raw/` and refuses to overwrite an existing capture or report.

The Rust process owns warm-ups and sample timing. Go owns process orchestration,
identity capture, strict validation, source alignment, engine-artifact
immutability checks, statistics, and persistence. See
[`docs/specifications/benchmark-v1.md`](../docs/specifications/benchmark-v1.md)
and [`docs/specifications/profile-v1.md`](../docs/specifications/profile-v1.md)
for exact boundaries.

The profile suite records `materialized-stage-passes-v1` with
`boundary-timers-v1`. Those stage values explain a changed diagnostic topology;
they are not an exact decomposition of the fused pipeline.

No suite contains a pass/fail latency threshold. Shared CI verifies integrity,
not machine performance.
