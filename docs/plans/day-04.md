# Day 4 execution record: versioned execution protocol

## Objective

Connect the Go control plane to the Rust scalar oracle through a bounded,
lossless, versioned process protocol. The result should be reusable by Day 5's
benchmark harness without introducing timings, retries, scheduling, or
persistence on Day 4.

## Implemented architecture

```text
labctl run
    |
    | starts and owns
    v
paraflow-engine serve
    |
    | stdin:  execute / shutdown NDJSON
    | stdout: completed / error / shutdown_ack NDJSON
    | stderr: bounded diagnostic capture
    v
ScalarOracle -> ResultV1 -> lossless wire result
```

- Go owns process start, request sequencing, cancellation, shutdown, and reap.
- Rust owns strict request decoding, workload validation, backend dispatch, and
  scalar execution.
- Each execute request embeds one complete `paraflow.workload/v1` object.
- Exactly one request is in flight; the process remains alive across
  sequential jobs.
- Protocol payloads are bounded at 4 MiB and stdout is protocol-only.
- Day 4 selects only the `scalar` backend.

## Implementation map

```text
contracts/
├── conformance/execution-v1.json
├── execution-protocol-v1.schema.json
└── execution-vectors-v1.schema.json
engine-rs/crates/
├── paraflow-protocol/
│   └── src/
│       ├── hex.rs
│       └── lib.rs
└── paraflow-engine/
    ├── src/server.rs
    └── tests/protocol_v1.rs
labctl-go/
├── cmd/labctl/main.go
└── internal/
    ├── app/
    │   ├── app.go
    │   └── app_test.go
    ├── protocol/
    │   ├── protocol.go
    │   └── protocol_test.go
    └── worker/
        ├── frame.go
        ├── real_engine_test.go
        ├── session.go
        └── session_test.go
tools/
└── check-protocol-integration.sh
Makefile
.github/workflows/ci.yml
```

`paraflow-protocol` separates serialization adapters from the logical
`paraflow-contracts::ResultV1`. Neither transport JSON nor Go structs become a
memory layout or future C ABI.

## Protocol invariants

- Request, response, and result schemas are independently versioned.
- Frames contain one compact JSON value plus `\n`.
- Payload size excludes its LF or CRLF terminator and cannot exceed 4 MiB.
- Request IDs are echoed exactly and validated before a result is trusted.
- Go request IDs advance monotonically and exhaust instead of wrapping.
- Unknown fields and mixed message variants are rejected.
- Go projects only the name, record count, and category count needed for
  response validation; Rust retains complete workload validation ownership.
- Workload name, backend, histogram shape, count bounds, and histogram totals
  are cross-checked by Go.
- Every `u64` uses fixed-width lowercase hexadecimal transport.
- `score_sum` uses exact IEEE-754 binary64 bits, including positive infinity.

## Failure ownership

An invalid workload, unsupported backend, or backend execution failure has a
known request ID and receives a structured error. The process remains healthy
and can serve another job.

Malformed or oversized frames, invalid envelopes, uncorrelated responses,
transport failures, and cancellation after a transaction begins make continued
stream use unsafe. Rust exits on fatal request-side failures; Go poisons and
synchronously reaps the session on fatal controller-side failures. There is no
automatic retry after a request write because its outcome may be unknown. A
caller that cancels while waiting behind the active request does not poison the
healthy session.

Healthy sessions end through correlated `shutdown` and `shutdown_ack` messages.

## Verification layers

1. Draft 2020-12 JSON Schema validates the closed request and response unions.
2. Portable execution vectors freeze empty and nonempty exact messages.
3. Rust protocol tests lock strict decoding and lossless integer/floating-point
   adapters.
4. Rust server tests exercise multiple jobs in one process, recoverable errors,
   malformed and oversized frames, writer failure, shutdown, and a maximum
   category-count response under the shared limit.
5. Go protocol tests reject malformed, mismatched, impossible, and
   non-lossless responses.
6. Go worker tests cover lifecycle, one-in-flight serialization, cancellation,
   bounded stderr, process failures, and explicit shutdown.
7. Cross-language conformance runs valid, invalid, then valid work through one
   release Rust process before acknowledged shutdown; CLI smoke tests cover
   the user-facing command separately.

## Benchmark boundary

Day 4 adds no benchmark clock, warm-up loop, raw sample, timing threshold,
benchmark record, persistence layer, worker pool, or retry policy. Process
longevity is an enabling correctness boundary, not itself a performance result.

Day 5 will reuse the same execute exchange for warm-ups, execution, and
correctness. A separate engine-side measurement harness will declare internal
timing boundaries, while the controller persists evidence without adding
timing fields to protocol v1.

## Deferred deliberately

- Day 5: sampling, timing boundaries, experiment identity, and persistence.
- Week 2: C++/ISPC SIMD backends and layout experiments.
- Later weeks: multiple workers, task graphs, scheduling, synchronization,
  pipelined requests, and GPU execution.
