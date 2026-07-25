# ADR 0003: Use counter-derived deterministic generation

- Status: Accepted
- Date: 2026-07-23

## Context

ParaFlow must feed identical logical records to a scalar oracle, SIMD kernels,
multicore workers, and GPU execution. A mutable pseudorandom generator would
make record identity depend on traversal order, partition boundaries, worker
count, or synchronization.

Large workloads must also be divisible into independent absolute index ranges.
Requiring every backend to generate and discard an earlier prefix would turn
input construction into a serial dependency.

## Decision

Workload v1 uses `splitmix64-v1` as a pure function of:

```text
(seed, absolute record index, field identifier)
```

Every addition and multiplication wraps modulo `2^64`. Record generation has no
shared mutable state. The Rust reference exposes random access first, then
builds lazy iteration and safe range materialization on that operation.

The checked-in conformance document stores exact `u64` inputs and outputs as
fixed-width hexadecimal strings. This avoids JSON number rounding and gives
future Go, C++, ISPC, and CUDA implementations the same language-neutral
acceptance vectors.

Workload manifests keep numeric `seed` and `record_count` fields for readable
configuration, but restrict them to `2^53 - 1` so every supported JSON parser
preserves them exactly. The generator functions themselves remain defined for
the full `u64` domain.

The current `Vec<LogicalRecord>` returned for a requested range is a scalar
engine convenience. It is not a workload contract, ABI, or decision in favor of
array-of-structs over structure-of-arrays.

## Consequences

- Any record can be regenerated during incident analysis from its workload
  identity and absolute index.
- Future workers can generate disjoint ranges in any order without changing
  input values.
- Golden vectors detect missing wrapping operations and field-ID drift across
  languages.
- Modulo mapping and its small statistical bias are part of workload v1; an
  alternative mapping requires a new generator version.
- SplitMix64 is used for reproducible workload construction, not security or
  cryptography.
