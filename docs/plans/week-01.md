# Week 1 execution plan

## Week outcome

By the end of Week 1, `labctl` runs a deterministic Rust scalar engine and
persists reproducible benchmark evidence. Day 1 created the contracts that keep
all later changes comparable.

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

## Day 5 — Completed benchmark harness

- Separate measurement contracts that do not inflate the Day 4 execution
  protocol.
- One fresh Rust process per scenario with in-process warm-ups and retained
  samples.
- Generation, pipeline, engine-total, experiment-total, and orchestration-total
  boundaries.
- Exact oracle comparison for every iteration and strict Go-side evidence
  validation.
- Release-profile, source-alignment, and stable-engine-artifact gates.
- Raw samples, median/MAD summaries, environment metadata, and atomic
  no-overwrite capture persistence.
- Controlled 1K, 64K uniform, 64K hotspot, and 1M suites plus CI smoke coverage.

No SIMD, multicore, GPU, or bottleneck claim is made yet.

## Day 6 — Completed baseline analysis

- Unchanged fused Day 5 baseline paired with a separate materialized stage-pass
  profile for every scenario.
- Coarse generation, normalize, score, filter, and aggregate boundaries with an
  explicit observer and topology.
- Exact stage-sum, enclosing-total, result-equality, release-build,
  source-alignment, and engine-immutability gates.
- All raw samples plus median/MAD, selectivity, stage shares, dominant stages,
  per-record costs, and observer/topology ratios.
- Portable request/result/report fixtures, real release integration, disposable
  smoke evidence, and a schema-checked 24-question understanding set.

No optimization, SIMD, multicore, GPU, or speedup claim is made.

## Day 7 — Completed `v0.1.0` qualification

- Strict offline verification for Day 5 captures and Day 6 paired reports.
- Repository-confined suite/workload resolution and SHA-256 identity replay.
- Raw-result, timing, source/build, summary, and profile-analysis recomputation.
- Optional explicit engine-byte verification with an honest receipt bit.
- One release version checked across Go, Rust, Cargo metadata, and binaries.
- CI and local release gates with deterministic checks and threshold-free smoke.
- Adversarial failure coverage, ADR/changelog, and 24 cumulative questions.
- Annotated Week 1 scalar checkpoint before native or parallel work begins.

## Extension rules

- New backends implement the existing workload.
- Execution settings never enter workload files.
- Optimizations do not remove the scalar oracle.
- Benchmark code does not become production engine logic.
- Physical layout remains replaceable until measured.
