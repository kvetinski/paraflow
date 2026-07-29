# ParaFlow

[![CI](https://github.com/kvetinski/paraflow/actions/workflows/ci.yml/badge.svg)](https://github.com/kvetinski/paraflow/actions/workflows/ci.yml)

ParaFlow is one evolving parallel-systems portfolio project built alongside
Stanford CS149. It begins with a measured scalar implementation and grows into
a Rust task-DAG runtime that schedules C++/ISPC/CUDA kernels, with a Go control
plane producing reproducible performance evidence.

```text
Go labctl
    | benchmark suites, process orchestration, evidence
    v
Rust ParaFlow engine
    | deterministic generation, scalar oracle, future scheduler
    v
C ABI
    |-- C++ scalar and SIMD
    |-- ISPC
    `-- CUDA
```

The stable workload is:

```text
seeded records -> normalize -> score -> filter/compact -> aggregate
```

This repository is not a collection of disconnected assignments. Every future
backend must preserve the same workload semantics and pass the same correctness
oracle before its performance is considered.

## Current milestone: Day 6

Day 6 explains the scalar baseline without optimizing it:

- the fused Day 5 benchmark remains unchanged as the comparison denominator;
- a separate `materialized-stage-passes-v1` profile exposes generation,
  normalize, score, filter, and aggregate boundaries;
- one coarse `boundary-timers-v1` observer surrounds each complete pass—never
  each record;
- every warm-up and retained profile iteration matches the streaming scalar
  oracle exactly;
- Go pairs a fresh fused baseline and stage profile for every scenario;
- scenario, workload, sampling, release build, source identity, engine hash,
  and exact canonical results must agree;
- all fused and profile samples remain in one self-contained immutable report;
- median/MAD, selectivity, stage shares, dominant stages, per-record costs, and
  observer/topology ratios use overflow-checked integer calculations;
- shared schemas, conformance fixtures, real release-process tests, and 24
  understanding questions make the milestone reviewable.

The materialized stage profile changes allocation, fusion, and memory access.
Its values are diagnostic evidence, not an exact decomposition of the fused
loop and not a speedup claim.

The complete implementation map, design, tests, benchmark setup, limitations,
and expected GitHub outcome are in
[`docs/plans/day-06.md`](docs/plans/day-06.md).

## Why the paired evidence is credible

Go does not time repeated pipe transactions as compute. For each scenario, it
sends one self-contained request to a fused benchmark process and one to a
diagnostic profile process. Rust owns both sample loops.

```mermaid
sequenceDiagram
    participant G as Go labctl
    participant B as Rust fused baseline
    participant P as Rust stage profile
    G->>B: scenario + embedded workload
    B-->>G: fused raw samples + exact result
    G->>P: same scenario + workload
    P->>P: coarse materialized stage passes
    P-->>G: stage raw samples + exact result
    G->>G: pair, validate, analyze, persist
```

Per retained sample:

| Field             | Boundary                                                                                 |
| ----------------- | ---------------------------------------------------------------------------------------- |
| `generation_ns`   | allocation and deterministic materialization                                             |
| `pipeline_ns`     | normalize through aggregate over materialized records                                    |
| `engine_total_ns` | generation, pipeline, exact comparison, temporary-output reclamation, engine bookkeeping |

The profile adds `normalize_ns`, `score_ns`, `filter_ns`, `aggregate_ns`,
`stage_sum_ns`, and `profile_total_ns`. The exact stage sum is conserved and
the enclosing total may be larger. Both Rust responses include an
`experiment_total_ns`; Go records each process's `orchestration_total_ns`
separately.

Request and response payloads are bounded at 4 MiB; one optional LF or CRLF
terminator is excluded from that payload limit. All durations use exact
fixed-width hexadecimal nanoseconds. The complete contract is documented in
[`docs/specifications/benchmark-v1.md`](docs/specifications/benchmark-v1.md)
and [`docs/specifications/profile-v1.md`](docs/specifications/profile-v1.md).

## Correctness foundation

### Frozen workload semantics

The language-neutral [`workload-v1`](docs/specifications/workload-v1.md)
contract defines logical records, deterministic distributions, arithmetic
operation order, inclusive filtering, stable compaction, and aggregation.
Physical representation is deliberately not part of the contract.

### Schedule-independent input

Every generated field is a pure function of `(seed, absolute record index,
field ID)`. There is no shared RNG cursor, so later worker partitions can
generate disjoint ranges independently without changing record identity.

### Scalar oracle

Rust keeps typed stage transitions:

```rust
let normalized = normalize_record(record, &spec.pipeline.normalize);
let scored = score_record(normalized, &spec.pipeline.score);
let accepted = filter_record(scored, &spec.pipeline.filter);
```

The reference path streams records in stable input order and produces
`ResultV1`:

- accepted count;
- stable binary64 score sum;
- category histogram;
- wrapping accepted-ID sum;
- mixed accepted-ID XOR.

Day 5 adds an internal materialized execution path only for measurement. It
reuses the same stages and aggregator and does not expose `Vec<LogicalRecord>`
as a public layout or ABI.

### Lossless cross-language results

Every logical `u64` is transported as `0x` plus sixteen lowercase hexadecimal
digits. `score_sum` carries exact IEEE-754 binary64 bits, preserving signed zero
and positive infinity without JSON-number loss.

## Contract boundaries

## Versioned execution boundary

```mermaid
sequenceDiagram
    participant G as Go labctl
    participant R as Rust engine serve
    G->>R: execute + embedded workload (stdin)
    R->>R: validate + scalar oracle
    R-->>G: completed result or correlated error (stdout)
    G->>R: shutdown
    R-->>G: shutdown_ack
```

Go owns the `paraflow-engine serve` child process and keeps at most one request
in flight. A request embeds the complete workload instead of a local file path.
Frames are newline-delimited JSON with a 4 MiB payload limit; stdout is
protocol-only and stderr is drained into a bounded diagnostic tail.

Results are independently validated in Go before use. Every logical `u64` is
encoded as `0x` plus sixteen lowercase hexadecimal digits, and `score_sum`
carries exact IEEE-754 binary64 bits. A correlated invalid workload,
unsupported backend, or execution failure leaves the worker reusable.
Framing, transport, response-schema, correlation, or result-invariant failures
poison the session and cause Go to reap the child. Local request validation can
fail before any write without damaging the session. Requests are not
automatically retried after a write because their outcome may be unknown.

See the normative
[execution protocol](docs/specifications/execution-protocol-v1.md), its
[machine-readable schema](contracts/execution-protocol-v1.schema.json), and
[ADR 0005](docs/adr/0005-long-lived-versioned-worker-protocol.md).

## Architecture

```mermaid
flowchart TD
    G["Go labctl<br/>process lifecycle and evidence"] -->|"bounded NDJSON"| R["Rust engine<br/>runtime and scalar oracle"]
    R --> A["Versioned C ABI"]
    A --> C["C++ / ISPC / CUDA kernels"]
```

| Component            | Responsibility today                                       | Future responsibility                  |
| -------------------- | ---------------------------------------------------------- | -------------------------------------- |
| `paraflow-contracts` | Workload types, stage order, validation                    | Shared semantic authority              |
| `paraflow-protocol`  | Lossless job/result transport types                        | Additional backend identifiers         |
| `paraflow-engine`    | Generation, oracle, fused benchmark, stage profiler        | Task DAG, scheduling, backend dispatch |
| `labctl`             | Diagnostics, orchestration, analysis, evidence persistence | Result indexing and comparison         |
| `abi`                | Documents boundary rules                                   | Narrow Rust-to-native C ABI            |
| `kernels-cpp`        | Documents scope only                                       | Scalar, SIMD, ISPC, and CUDA kernels   |


| Contract    | Responsibility                                            | Status                                               |
| ----------- | --------------------------------------------------------- | ---------------------------------------------------- |
| Workload    | What the computation means                                | `paraflow.workload/v1` frozen                        |
| Execution   | Reusable Go-to-Rust correctness transactions              | Day 4 protocol v1 frozen                             |
| Measurement | Fused warm-ups, samples, boundaries, and capture identity | Day 5 benchmark v1 frozen                            |
| Profiling   | Explicit observer/topology and paired scalar analysis     | Day 6 profile/report v1                              |
| Native ABI  | Rust-to-C++ buffer calls                                  | constraints documented; implementation begins Week 2 |

The Day 4 `serve` worker remains available for reusable correctness execution.
The Day 5 `benchmark` and Day 6 `profile` processes are separate so measurement
and diagnostic policies do not inflate or destabilize that protocol.

## Repository structure

```text
.
├── abi/                         # future narrow Rust-to-native C ABI
├── benchmarks/
│   └── suites/                  # versioned sampling scenarios
├── contracts/
│   ├── *.schema.json            # workload, execution, measurement schemas
│   └── conformance/             # portable exact fixtures
├── docs/
│   ├── adr/                     # architectural decisions
│   ├── architecture/            # system design
│   ├── plans/                   # milestone plans and acceptance gates
│   └── specifications/          # normative contracts
├── engine-rs/crates/
│   ├── paraflow-contracts/      # stable semantic authority
│   ├── paraflow-engine/         # generation, oracle, worker, benchmark loop
│   └── paraflow-protocol/       # lossless execution and measurement types
├── kernels-cpp/                 # C++/ISPC starts Week 2; CUDA later
├── labctl-go/                   # control plane and evidence persistence
├── results/raw/                 # ignored local immutable captures
├── tools/schema-check/          # pinned Draft 2020-12 validation
└── workloads/                   # semantic workload fixtures
```

## Requirements
- Git;
- Go 1.24 or newer;
- Rust 1.97.1 with `rustfmt` and `clippy`;
- a C compiler for race-enabled Go tests;
- Node.js 20 or newer with npm;
- Bash 4 or newer and GNU Make.

C++, ISPC, and CUDA are reported by `doctor` but are not required until their
roadmap weeks.

## Quick start

Run every deterministic quality gate:

```bash
make check
```

Inspect machine and toolchain readiness:

```bash
make go-build
./bin/labctl doctor
./bin/labctl doctor --json
```

Run one workload through the reusable Day 4 worker:

```bash
make rust-release-build go-build
./bin/labctl run \
  --engine ./target/release/paraflow-engine \
  workloads/edge-scalar-v1.json
```

Exercise the complete Day 5 boundary without retaining the output:

```bash
make benchmark-smoke
```

Persist the full raw scalar baseline:

```bash
make benchmark-day05
```

The command creates a unique file similar to:

```text
results/raw/day05-scalar-20260726T120000Z-0123456789ab.json
```

It refuses to overwrite an existing result.

Run the controller directly:

```bash
./bin/labctl benchmark \
  --engine ./target/release/paraflow-engine \
  --suite benchmarks/suites/day05-scalar-baseline-v1.json \
  --output results/raw/my-clean-baseline.json \
  --repository-root .
```

Only a release-profile engine result is accepted. Its embedded commit and
source state must match `labctl`, and its executable hash must remain unchanged
for the complete suite.

Exercise the paired Day 6 boundary without retaining its output:

```bash
make profile-smoke
```

Persist a full paired scalar profile:

```bash
make profile-day06
```

Or invoke the controller directly:

```bash
./bin/labctl profile \
  --engine ./target/release/paraflow-engine \
  --suite benchmarks/suites/day06-scalar-profile-v1.json \
  --output results/raw/my-day06-profile.json \
  --repository-root .
```

The report retains both topologies. The printed stage-pass/fused ratio describes
observer context and is never labeled a speedup.

The checked-in clean-run evidence and its interpretation are available in the
[Day 6 scalar baseline report](docs/reports/day06-scalar-baseline.md).

## Benchmark suites

| Suite                           | Purpose                                                        |
| ------------------------------- | -------------------------------------------------------------- |
| `day05-smoke-v1.json`           | disposable fused integration evidence                          |
| `day05-scalar-baseline-v1.json` | 1K, 64K uniform/hotspot, and 1M fused evidence                 |
| `day06-profile-smoke-v1.json`   | disposable paired fused/profile integration evidence           |
| `day06-scalar-profile-v1.json`  | 1K, 64K uniform/hotspot, and 1M paired stage-analysis evidence |

The two 64K workloads are identical except for the declared distribution, so
the skew experiment changes one controlled variable. The full suite runs
scenarios sequentially. It stores every sample and reports
median and MAD; it does not turn timing into a CI threshold.

## Testing strategy

`make check` covers:

- Draft 2020-12 schema validation and rejection cases;
- semantic validation of every workload;
- generator, scalar, execution, benchmark, profile, report, and question
  conformance fixtures;
- Rust formatting, Clippy, unit, integration, and release tests;
- Go formatting, `vet`, race-enabled tests, and source-identity build smoke;
- real release Go-to-Rust reusable-worker integration;
- real release Go-to-Rust benchmark integration;
- real release Go-to-Rust stage-profile integration;
- exact-limit LF/CRLF framing, unsupported-terminator rejection, and oversized-payload rejection;
- exact stage conservation, paired-result equality, source alignment, and
  engine-mutation rejection before persistence.

`make benchmark-smoke` and `make profile-smoke` additionally prove that complete
evidence can be produced in the current environment. They check structure and
correctness only, never a fixed latency.

## Evidence and limitations

A Day 6 report records enough identity to reproduce both experiment topologies,
but its analysis still has explicit limitations:

- no CPU pinning or NUMA policy;
- no forced frequency or turbo policy;
- no hardware-counter attribution yet;
- the generation boundary includes allocation;
- the pipeline boundary uses the current scalar materialized representation;
- profile stage passes add intermediate allocations and lose fused-loop memory
  behavior;
- boundary timers perturb the diagnostic topology;
- scenarios use separate processes, so they do not share warmed allocator or
  calibration state;
- one machine's timings do not generalize automatically to another.

1. deterministic generation — **complete**;
2. scalar Rust correctness oracle — **complete**;
3. Go-to-Rust experiment protocol — **complete**;
4. reproducible benchmark harness — **complete**;
5. observer-aware scalar profiling and analysis — **complete**;
6. SIMD/ISPC kernels;
7. multicore task execution and scheduling;
8. CUDA and heterogeneous execution.

These are not hidden defects. They define the questions later layout and runtime
experiments must answer. `make profile-day06` produces a fresh report on the
machine whose metadata it records.

See [`docs/benchmark-methodology.md`](docs/benchmark-methodology.md) and
[ADR 0006](docs/adr/0006-engine-owned-sampling-and-immutable-captures.md).

## Architecture decisions

- [ADR 0001 — Language ownership](docs/adr/0001-language-ownership.md)
- [ADR 0002 — Logical contract before layout](docs/adr/0002-logical-contract-before-layout.md)
- [ADR 0003 — Counter-derived generation](docs/adr/0003-counter-derived-generation.md)
- [ADR 0004 — Typed streaming scalar oracle](docs/adr/0004-typed-streaming-scalar-oracle.md)
- [ADR 0005 — Long-lived versioned worker protocol](docs/adr/0005-long-lived-versioned-worker-protocol.md)
- [ADR 0006 — Engine-owned sampling and immutable captures](docs/adr/0006-engine-owned-sampling-and-immutable-captures.md)
- [ADR 0007 — Observer-aware scalar profiling](docs/adr/0007-observer-aware-scalar-profiling.md)

## Next milestone

Day 7 hardens reproducibility, documentation, failure behavior, and curated
evidence before tagging the Week 1 scalar `v0.1` baseline. SIMD work begins only
after that stable checkpoint.
