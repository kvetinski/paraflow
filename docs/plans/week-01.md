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

## Day 3 — Completed scalar oracle

- Typed scalar normalize, score, inclusive filter, and accepted-record stages.
- Stable-order streaming execution without dataset-sized intermediate buffers.
- Canonical `ResultV1` with wrapping structural fields and sequential `f64`
  score accumulation.
- Optional, fallibly allocated stable compacted-ID diagnostics.
- Exact `f32` stage bits and `f64` result bits in portable conformance vectors.
- Uniform, hotspot, empty, full-width generator, and dedicated scalar-edge
  results.
- One-shot Rust `oracle` CLI plus optimized release conformance preflight.
- Explicit IEEE infinity/NaN behavior without inventing the Day 4 wire format.

There is still no published timing result.

## Day 4 — Completed execution protocol

- Strict, versioned execute, result, error, shutdown, and acknowledgment
  envelopes.
- One long-lived Rust `serve` process owned and reaped by Go.
- Bounded 4 MiB NDJSON frames over stdin/stdout, with protocol-only stdout and
  bounded stderr capture.
- One in-flight request carrying a complete embedded workload.
- Scalar backend selection kept separate from workload meaning.
- Lossless fixed-width hexadecimal `u64` results and exact binary64 score bits.
- Recoverable correlated job errors separated from fatal protocol and transport
  failures.
- Portable vectors, cross-language conformance, process lifecycle tests, and
  explicit graceful shutdown.

There are still no timings, retries, schedules, or persisted benchmark results.

## Day 5 — Benchmark harness

Reuse protocol v1 for warm-ups, execution, and correctness. Add an engine-side
measurement harness for precise internal timing boundaries, release-build
enforcement, raw samples, and result persistence without adding timing fields
to protocol v1.

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
