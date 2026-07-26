# ADR 0004: Use a typed streaming scalar oracle

- Status: Accepted
- Date: 2026-07-26

## Context

ParaFlow needs one readable implementation that defines correct
normalize-through-aggregate behavior before SIMD, multicore, and GPU backends
exist. It must preserve exact stage semantics without prematurely freezing
array-of-structs buffers, an FFI layout, a backend trait, or a parallel merge
algorithm.

Materializing a separate full dataset for every stage would make the reference
implementation easy to inspect but would add avoidable memory traffic and turn
temporary Rust vectors into accidental architecture. Fusing all arithmetic
into one opaque loop would make exact stage testing and future profiling harder.

## Decision

- `paraflow-contracts` owns the logical `ResultV1` fields.
- `paraflow-engine::scalar` owns typed normalize, score, accepted-record, and
  aggregation values. They have no `repr(C)` or serialization promise.
- Each per-record stage remains a separately callable pure scalar function.
- `ScalarOracle` streams generated records through those functions in stable
  input order and accumulates one result.
- The default path stores only the category histogram. Stable compacted IDs are
  collected through an explicit diagnostic option.
- Every output allocation is fallible, and generated categories are checked
  before histogram access.
- Integer result fields wrap exactly as workload v1 specifies. `score_sum` adds
  each accepted `f32` score after conversion to `f64`, in stable input order.
- No public result-merge operation exists. A merge tree would change
  floating-point association and falsely imply bit-exact parallel sums.
- The one-shot `oracle` CLI prints deterministic human diagnostics. It is not
  the versioned Day 4 execution protocol.

## Floating-point boundary

The oracle uses ordinary IEEE `f32` operations with explicit temporaries, no
`mul_add`, no fast-math, and no reassociation. Finite parameters can still
overflow into infinity or produce NaN. With the required finite threshold, NaN
and negative infinity are rejected; positive infinity is accepted and makes
the canonical score sum positive infinity.

Exact scalar fixtures store floating-point bit patterns. Future parallel
backends must match structural fields exactly and compare finite score sums
under a declared tolerance after structural equality succeeds.

## Consequences

- The oracle is readable, deterministic, and cheap enough to run as every
  future backend's correctness gate.
- Stage-level vectors can detect rounding, contraction, threshold, and
  compaction drift.
- Normal execution uses `O(category_count)` result memory rather than
  dataset-sized intermediate buffers.
- Optional diagnostics can consume `O(accepted_count)` memory and therefore use
  fallible growth.
- Day 4 must define a lossless wire representation for full-width integers and
  reachable positive infinity instead of directly serializing `ResultV1`.
- Future layouts and parallel execution strategies remain free to change as
  long as they preserve the workload's observable semantics.
