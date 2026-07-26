# Day 3 execution record: scalar correctness oracle

## Objective

Make the complete `paraflow.workload/v1` pipeline executable through one
readable Rust reference implementation. The result must become the correctness
target for every later SIMD, multicore, and GPU backend without freezing a
physical layout, wire protocol, or benchmark harness.

## Implemented architecture

```text
DatasetGenerator
    |
    v
LogicalRecord
    |
    v
normalize_record -> NormalizedRecord
    |
    v
score_record -> ScoredRecord
    |
    v
filter_record -> Option<AcceptedRecord>
    |
    v
stable accumulator -> ResultV1
```

- `paraflow-contracts::ResultV1` owns logical result meaning.
- `paraflow-engine::scalar` owns reference-only stage values and execution.
- `ScalarOracle` validates the workload before execution, borrows it
  immutably, and streams records in absolute input order.
- Normal execution retains only the category histogram. Accepted IDs are
  optional diagnostics.
- Every result allocation is fallible. Generated categories are checked before
  histogram indexing.

## Implementation map

```text
contracts/
├── conformance/scalar-v1.json       # Portable exact stage/result vectors
└── scalar-vectors-v1.schema.json    # Machine-checked vector shape
engine-rs/crates/
├── paraflow-contracts/src/result.rs # Canonical logical ResultV1
└── paraflow-engine/
    ├── src/scalar/
    │   ├── stages.rs                # Typed normalize/score/filter stages
    │   ├── aggregate.rs             # Stable fallible result accumulation
    │   └── mod.rs                   # Streaming ScalarOracle
    └── tests/scalar_v1.rs           # Portable end-to-end conformance
workloads/
├── edge-scalar-v1.json              # Clamp/filter/compaction result edges
├── edge-scalar-ops-v1.json          # Offset order and zero-mask semantics
└── edge-scalar-mask-v1.json         # All-bits mask semantics
```

## Correctness policy

- Normalization and scoring use explicit `f32` operations in the frozen order.
- The inclusive filter is `score >= min_score`.
- Accepted scores are converted to `f64` and added sequentially.
- Counts, histogram bins, and ID sums wrap modulo `2^64`.
- The ID XOR uses `mix_v1(id)`, not raw identifiers.
- Stable compacted IDs preserve absolute input order.
- IEEE infinity and NaN behavior is preserved and tested.

Exact conformance data lives in
`contracts/conformance/scalar-v1.json`. The dedicated
`workloads/edge-scalar-v1.json` case exercises both clamp directions,
flag-controlled bonus behavior, mixed acceptance, threshold equality, stable
compaction, an empty histogram bin, and every canonical result field.
`edge-scalar-ops-v1.json` and `edge-scalar-mask-v1.json` additionally freeze
nonzero-offset operation order, zero-mask behavior, and all-bits mask matching
for future language backends.

## Verification layers

1. JSON Schema validates workloads and both conformance documents.
2. Stage unit tests lock conversion, arithmetic order, clipping, bonus, filter,
   stable `f64` addition, wrapping, and non-finite behavior.
3. Integration tests compare exact `f32` and `f64` bits plus lossless integer
   outputs for seven workloads, including nonzero normalization offsets,
   zero-mask bonus behavior, and all-bits mask matching.
4. CLI tests lock exit codes and deterministic one-shot diagnostics.
5. `make scalar-conformance` runs the generator and scalar suites in release
   mode and smoke-tests the real CLI boundary.
6. `make scalar-check` adds schema and semantic workload validation.

## Benchmark boundary

Day 3 adds no timer, sample loop, threshold, persisted result, or performance
claim. `make benchmark-preflight` now proves that the optimized complete scalar
path is correct and that the host is ready. Day 5 will introduce warm-ups, raw
samples, experiment identity, and separate generation/compute/total timing
boundaries.

## Deferred deliberately

- Day 4: versioned Go-to-Rust job and result protocol.
- Day 5: benchmark sampling and persistence.
- Week 2: C++/ISPC SIMD kernels and physical-layout experiments.
- Later weeks: task graph, scheduling, synchronization, and GPU execution.
