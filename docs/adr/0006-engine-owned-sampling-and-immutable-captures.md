# ADR 0006: Engine-owned sampling and immutable benchmark captures

- Status: Accepted
- Date: 2026-07-26

## Context

Day 4 created a reusable Go-to-Rust correctness worker. Day 5 must collect
credible scalar timing evidence without allowing process startup, JSON
transport, controller scheduling, or persistence to contaminate every compute
sample.

Three designs were considered.

1. **Go repeatedly calls the Day 4 worker and times each transaction.** This is
   easy to implement, but each sample includes pipe transport, request parsing,
   response encoding, and Go scheduling. It answers the wrong question for
   kernel and runtime work.
2. **Go starts one Rust process per sample.** This is operationally simple but
   process startup dominates small workloads and turns startup variance into
   apparent compute variance.
3. **Go starts one Rust benchmark process per scenario; Rust performs warm-ups
   and all retained samples.** This keeps orchestration outside the sample loop
   while retaining a clean process boundary and reproducible request.

## Decision

Use option 3.

The existing `serve` protocol remains the correctness and lifecycle boundary.
A separate one-shot benchmark contract owns measurement semantics. One Rust
process receives one self-contained scenario, computes an untimed oracle
reference, performs warm-ups and retained samples, validates every iteration,
and returns all raw measurements.

Go remains the control plane. It validates the response as an untrusted peer,
requires controller/engine source identity to align, hashes the measured binary
before and after the suite, captures environment metadata, derives median and
MAD, and persists one immutable capture only after the entire suite succeeds.

Day 5 requires a release-profile engine. Process startup is explicitly excluded
from retained engine samples and separately visible through Go orchestration
time.

## Consequences

### Positive

- Sample boundaries correspond to actual engine work.
- Startup is amortized once per scenario but never hidden completely.
- Warm-up policy is executable and versioned.
- Every sample passes the scalar correctness gate.
- Raw evidence is portable and attributable to one source revision and one
  unchanged engine artifact.
- Future scalar, SIMD, multicore, and GPU backends can reuse the measurement
  envelope without moving workload semantics.
- The Day 4 worker remains small and is not overloaded with benchmark policy.

### Negative

- There are now distinct execution and measurement protocols.
- One scenario process cannot currently share calibration state with another.
- The Day 5 materialized batch includes allocation in generation time, buffer
  reclamation in engine-total time, and uses the current scalar
  `Vec<LogicalRecord>` representation.
- Source state depends on build-time metadata and must be supplied correctly by
  the repository build command.

### Guardrails

- No backend speedup is published until it matches the oracle.
- No debug-profile capture is accepted.
- No aggregate-only result is accepted; raw samples are mandatory.
- No shared CI timing threshold is introduced.
- No existing result file is overwritten.
- No capture is persisted if source identities differ or the engine executable
  changes during the suite.
