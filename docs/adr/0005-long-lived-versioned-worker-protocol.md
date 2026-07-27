# ADR 0005: Use a long-lived versioned worker protocol

- Status: Accepted
- Date: 2026-07-27

## Context

Day 3 exposed the Rust scalar oracle through a one-shot, human-readable CLI.
That command is useful for inspection, but starting a process for every future
warm-up and sample would mix startup cost with execution and make controller
failures hard to classify.

Go needs a narrow process boundary that preserves exact workload and result
meaning. The boundary must also remain useful when later backends and benchmark
sampling arrive, without becoming a scheduler, persistence format, C ABI, or
in-memory layout.

Ordinary JSON numbers cannot losslessly represent every `u64`, and
`score_sum` can validly become positive infinity. Unbounded line reads would
also let one malformed peer consume arbitrary memory.

## Decision

- Go owns the lifecycle of one long-lived `paraflow-engine serve` subprocess.
- Requests are sent on stdin and responses are read from stdout as one compact
  JSON object per newline-delimited frame.
- Protocol v1 limits each JSON payload to 4 MiB, excluding its LF or CRLF
  terminator. Both peers enforce the bound before trusting a frame.
- Stdout is reserved for protocol frames. Rust diagnostics use stderr, which
  Go drains continuously into a bounded tail.
- Go serializes callers so exactly one request is in flight. Correlation IDs
  are still required and validated.
- An execute request embeds the complete workload object. It never sends a
  controller-local file path.
- Day 4 supports only the `scalar` backend.
- Successful results encode every `u64` as `0x` plus exactly sixteen lowercase
  hexadecimal digits. `score_sum` carries its exact IEEE-754 binary64 bits in
  the same representation.
- Invalid workload shape or semantics, an unsupported backend, and an
  execution failure produce correlated structured error responses. The worker
  remains reusable after these job-level failures.
- Malformed, oversized, uncorrelatable, or otherwise invalid protocol frames
  are fatal because stream alignment or peer trust cannot be recovered safely.
  Go also poisons and reaps a session after transport, framing, response
  validation, correlation, or in-flight cancellation failures.
- Go does not automatically retry a request after a write. Its execution
  outcome may be unknown.
- Shutdown is explicit: Go sends `shutdown`, validates the correlated
  `shutdown_ack`, closes stdin, and reaps the child.

The machine-readable shape is
[`execution-protocol-v1.schema.json`](../../contracts/execution-protocol-v1.schema.json).
The human-readable behavior is specified in
[`execution-protocol-v1.md`](../specifications/execution-protocol-v1.md).

## Consequences

- One process can serve future warm-ups and samples without charging process
  startup to every execution.
- Workloads are self-contained and replayable independently of the
  controller's filesystem.
- Strict framing and bounded diagnostics cap memory exposure at the process
  boundary.
- Exact integer and floating-point states survive a language-neutral JSON
  transport.
- The sequential Day 4 session is intentionally not a task runtime or a claim
  of parallel execution.
- Day 5 can reuse the same protocol for execution and correctness while a
  separate engine-side measurement harness captures internal timing boundaries
  without adding timing fields to protocol v1.
- A later pipelined or concurrent protocol would require an explicit version
  and a stronger out-of-order response model.
