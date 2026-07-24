# Architecture overview

## System thesis

ParaFlow is a batch-analytics engine whose implementation evolves while its
logical workload remains stable. The project is designed to answer one
question with increasing depth:

> How should the same computation be represented, scheduled, synchronized, and
> mapped to scalar CPU, SIMD, multicore CPU, and GPU hardware?

Changing backends must not change the meaning of the workload.

## Component boundaries

### Rust engine

Rust owns:

- the scalar correctness oracle;
- logical buffers and future physical-layout adapters;
- the task graph;
- worker lifecycle, scheduling, cancellation, and backpressure;
- backend selection;
- correctness comparison and execution tracing.

On Day 1, `paraflow-contracts` defines workload types and semantic validation.
`paraflow-engine` only validates manifests and reports the active contract.
There is deliberately no `run` command before the scalar oracle exists.

### Go controller

Go owns:

- developer and runner diagnostics;
- future experiment manifests;
- process orchestration;
- environment, commit, and toolchain evidence;
- future result indexing and optional API exposure.

Go is outside the measured compute path. It will ask one long-lived Rust
process to perform warm-ups and samples so process startup is not confused with
kernel or engine time.

### C++ kernels

C++ will own:

- native scalar comparison kernels;
- explicit SIMD and ISPC implementations;
- CUDA host code and kernels;
- hardware-specific profiling hooks.

C++ begins only when Week 2 introduces SIMD. Day 1 contains no fake native
target.

## Contracts

ParaFlow separates three concerns:

| Contract | Meaning | Status |
| --- | --- | --- |
| Workload | Dataset and pipeline semantics | Frozen as `paraflow.workload/v1` |
| Execution | Backend, workers, chunking, device | Introduced after the scalar engine |
| Measurement | Warm-ups, samples, timing boundaries | Policy defined; executable harness on Day 5 |

A workload file must never contain worker counts, repetitions, backend names,
or expected timings. Those settings do not change what the computation means.

## Deliberately deferred decisions

The following remain open because experiments should decide them:

- array-of-structs versus structure-of-arrays;
- buffer alignment and padding;
- ownership across FFI;
- global queue versus per-worker deques;
- static versus dynamic chunking;
- lock-based versus lock-free ready queues;
- CPU versus GPU selection thresholds.

Freezing these on Day 1 would turn future performance work into confirmation
bias rather than engineering.

## Correctness hierarchy

Every future implementation is checked against the Rust scalar oracle:

1. parsing and semantic validation;
2. deterministic input identity;
3. per-stage invariants;
4. exact structural results;
5. documented floating-point tolerance;
6. performance comparison only after correctness passes.

No backend can earn a performance result by weakening correctness.
