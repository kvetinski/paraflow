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
- future in-engine task-worker lifecycle, scheduling, compute cancellation, and
  backpressure;
- backend selection;
- correctness comparison and execution tracing.

`paraflow-contracts` defines workload types, semantic validation, and the
logical `ResultV1`. Day 2 added validated random-access generation. Day 3 adds
the typed scalar stages and a stable-order streaming oracle under
`paraflow-engine`. Day 4 adds a long-lived, sequential `serve` worker and keeps
its lossless transport types in `paraflow-protocol`. The one-shot `oracle`
command remains a developer diagnostic.

Rust owns future in-engine worker lifecycle and scheduling. That is distinct
from the operating-system process lifecycle, which belongs to Go.

### Go controller

Go owns:

- developer and runner diagnostics;
- the `paraflow-engine serve` subprocess lifecycle;
- request correlation, cancellation, and explicit shutdown;
- strict response and result-invariant validation;
- future experiment manifests;
- environment, commit, and toolchain evidence;
- future result indexing and optional API exposure.

Go is outside the measured compute path. It asks one long-lived Rust process to
execute workloads sequentially, so Day 5 can perform warm-ups and samples
without confusing per-process startup with every execution.

### C++ kernels

C++ will own:

- native scalar comparison kernels;
- explicit SIMD and ISPC implementations;
- CUDA host code and kernels;
- hardware-specific profiling hooks.

C++ begins only when Week 2 introduces SIMD. Day 4 still contains no fake
native target or duplicate generator implementation.

## Contracts

ParaFlow separates three concerns:

| Contract    | Meaning                                                  | Status                                      |
| ----------- | -------------------------------------------------------- | ------------------------------------------- |
| Workload    | Dataset and pipeline semantics                           | Frozen as `paraflow.workload/v1`            |
| Execution   | Process framing, backend selection, job/result transport | Frozen as protocol v1; scalar only          |
| Measurement | Warm-ups, samples, timing boundaries                     | Policy defined; executable harness on Day 5 |

A workload file must never contain worker counts, repetitions, backend names,
or expected timings. Those settings do not change what the computation means.

## Day 4 process boundary

```text
Go Session
    |
    | starts `paraflow-engine serve`
    |
    +-- stdin  --> execute(workload) / shutdown
    +-- stdout <-- completed(result) / error / shutdown_ack
    +-- stderr --> continuously drained bounded diagnostics
```

Messages are strict newline-delimited JSON. Each payload is limited to 4 MiB,
excluding its LF or CRLF terminator, and stdout carries protocol messages only.
Execute requests embed the complete workload object. Go serializes callers to
one in-flight request, validates the correlated response, and decodes
full-width integer and binary64 bit encodings without JSON-number loss.

Failures are divided by whether correlation and stream integrity remain known:

| Failure class | Examples                                                                        | Session outcome                                 |
| ------------- | ------------------------------------------------------------------------------- | ----------------------------------------------- |
| Job           | Invalid workload, unsupported backend, execution failure                        | Correlated error; worker reusable               |
| Protocol      | Malformed or oversized frame, wrong schema/kind, correlation or result mismatch | Fatal; process exits or Go poisons and reaps it |
| Transport     | Broken pipe, EOF, in-flight cancellation, write/read failure                    | Fatal; Go poisons and reaps it                  |

Go never retries automatically after writing a request because the execution
outcome may be unknown. A healthy session ends with an explicit correlated
`shutdown`/`shutdown_ack` exchange.

This boundary is process transport, not the future Rust-to-C++ C ABI. It
introduces no performance timers, samples, persistence, worker pool, task
scheduling, or parallel execution.

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

## Scalar oracle boundary

The scalar module exposes typed logical transitions:

```text
LogicalRecord -> NormalizedRecord -> ScoredRecord -> AcceptedRecord
```

Those types make stage behavior observable but do not define buffers, ABI, or
layout. `ScalarOracle` streams one record at a time through the transitions and
updates `ResultV1` in absolute input order. It retains stable compacted IDs only
when diagnostics request them.

The implementation deliberately has no backend trait or result-merge API yet.
One backend is not enough evidence to design a durable dispatch abstraction,
and merging partial floating-point sums would move the scalar oracle's stable
addition order.

Exact stage and final-result vectors use IEEE bit patterns. Finite validated
parameters may still produce infinity or NaN scores. The finite filter
threshold rejects NaN and negative infinity; positive infinity is accepted and
propagates to the canonical sum. Protocol v1 encodes that state as exact
IEEE-754 binary64 bits and encodes every logical `u64` as fixed-width
lowercase hexadecimal.

## Deterministic input identity

`splitmix64-v1` derives each field from the workload seed, absolute record
index, and stable field identifier. It does not consume mutable RNG state.

This gives future execution layers a clean partitioning boundary:

```text
range 0..N = range 0..A + range A..B + range B..N
```

The ranges may be visited in any order and still contain the same records.
Day 4 still does not schedule those ranges; generation proves the property that
later schedulers will rely on.

Reference range materialization currently uses `Vec<LogicalRecord>`. This is a
replaceable scalar representation, not a workload schema, C ABI, or decision
against later columnar layouts.
