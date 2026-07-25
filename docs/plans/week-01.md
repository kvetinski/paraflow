# Week 1 execution plan

## Week outcome

By the end of Week 1, `labctl` will run a deterministic Rust scalar engine and
persist reproducible benchmark evidence. Day 1 creates the contracts that keep
those later changes comparable.

## Day 1 — Completed foundation

- Polyglot monorepo and ownership boundaries.
- Versioned workload schema and valid smoke workload.
- Rust semantic types and accumulated validation.
- Rust contract and validation CLI.
- Go environment readiness report.
- Architecture and benchmark policy.
- Unit/integration tests and CI.

## Day 2 — Completed deterministic data

- Pure wrapping `splitmix64-v1` as `(seed, index, field)` generation.
- Validated random-access generation and lazy stable-order iteration.
- Safe reference range materialization without freezing a physical layout.
- Uniform and hotspot workload fixtures.
- Versioned fixed-width conformance vectors shared across language boundaries.
- Boundary tests for ranges, probabilities, flags, and full-width features.

There is still no normalize-through-aggregate pipeline execution or published
timing result.

## Day 3 — Scalar oracle

Implement each pipeline stage and final reference result in Rust. Freeze
correctness fixtures and edge-case behavior.

## Day 4 — Execution protocol

Add versioned job/result envelopes. Go submits one experiment to a long-lived
Rust process and validates the structured response.

## Day 5 — Benchmark harness

Add warm-ups, raw samples, precise timing boundaries, release-build enforcement,
and result persistence.

## Day 6 — Baseline analysis

Profile stage costs across sizes and distributions. Publish the first evidence
without optimizing the result.

## Day 7 — `v0.1`

Harden documentation, CI, reproducibility, and failure behavior. Tag the scalar
baseline before SIMD work begins.

## Extension rules

- New backends implement the existing workload.
- Execution settings never enter workload files.
- Optimizations do not remove the scalar oracle.
- Benchmark code does not become production engine logic.
- Physical layout remains replaceable until measured.
