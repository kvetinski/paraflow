# ParaFlow benchmark contract v1

## Status

`paraflow.benchmark-request/v1`, `paraflow.benchmark-engine-result/v1`, and
`paraflow.benchmark-capture/v1` are the Day 5 measurement contracts.

They are deliberately separate from:

- `paraflow.workload/v1`, which defines what is computed;
- `paraflow.job/v1`, which defines the reusable Day 4 correctness worker;
- the future Rust-to-C++ C ABI, which will define an in-memory kernel boundary.

Changing warm-up count, sample count, output path, or benchmark suite must never
change workload meaning.

## Process model

One Go controller process executes a suite sequentially. Each scenario starts
one fresh Rust process with:

```text
paraflow-engine benchmark
```

Go writes one self-contained benchmark request to stdin. The Rust process:

1. strictly decodes the request;
2. strictly decodes and semantically validates the embedded workload;
3. computes the frozen streaming scalar-oracle result outside the timed loop;
4. performs all configured warm-up iterations;
5. performs every retained iteration;
6. checks each materialized iteration against the oracle exactly;
7. returns all raw samples and exits.

Process startup is therefore paid once per scenario, not once per retained
sample. It is excluded from engine samples and included in Go's separately
reported `orchestration_total_ns`.

## Request

A request contains:

- a versioned schema identifier;
- a controller-generated experiment ID;
- a scenario name;
- execution settings (`scalar` on Day 5);
- warm-up and retained-sample counts;
- the complete embedded workload object.

The workload is embedded rather than referenced by path so the exact request is
self-contained and replayable. Go keeps the original workload path and SHA-256
in the final capture.

Limits:

- request/response JSON payload: 4 MiB; one optional LF or CRLF terminator is
  excluded from the payload count;
- warm-ups: `0..=1000`;
- retained samples: `1..=10000`;
- scenarios per suite: `1..=64`.

## Timed boundaries

Each retained sample contains three monotonic durations:

| Field | Included work | Excluded work |
| --- | --- | --- |
| `generation_ns` | fresh allocation and deterministic materialization of every logical record | workload parsing, oracle preflight, process startup |
| `pipeline_ns` | normalize → score → filter/compact → aggregate over the materialized records | generation, process startup, Go work |
| `engine_total_ns` | generation, pipeline, exact result comparison, reclamation of per-iteration record/result buffers, and engine bookkeeping | process startup, request/response transport, Go persistence |

The engine also reports `experiment_total_ns`, covering all warm-ups and
retained iterations inside the Rust process. Go reports
`orchestration_total_ns`, covering request encoding, process launch, transport,
strict response validation, and complete scenario execution.

Every duration is encoded as `0x` plus exactly sixteen lowercase hexadecimal
digits. This preserves the complete `u64` nanosecond value in every JSON
consumer.

## Correctness policy

Day 5 benchmarks only the scalar implementation, so every materialized result
must match the frozen streaming scalar oracle exactly, including:

- accepted count;
- exact binary64 bits of `score_sum`;
- category histogram;
- accepted-ID wrapping sum;
- accepted-ID mixed XOR.

A mismatch aborts the process. Go independently validates the result schema,
workload dimensions, histogram conservation, integer encodings, and floating
point state before persistence.

Future parallel reductions may require a documented floating-point tolerance,
but that is not part of benchmark v1's scalar comparison policy.

## Build policy

`labctl benchmark` accepts only an engine result whose embedded profile is
`release`. The engine records:

- crate version;
- Cargo profile;
- target triple;
- `rustc --version`;
- Git commit;
- clean, dirty, or unknown source state.

The Go controller requires the engine commit and source state to match its own
embedded identity. It hashes the exact engine executable before the first
scenario and again after the final scenario; any mutation aborts persistence. A
dirty build may be recorded for local investigation, but a curated portfolio
claim should come from a clean source state.

## Raw data and statistics

Every retained sample is persisted. Go derives, for each boundary:

- count;
- minimum;
- median;
- median absolute deviation (MAD);
- maximum.

For an even sample count, the median is the floor of the integer midpoint. MAD
uses the same rule. These summaries are descriptive conveniences; raw samples
remain authoritative.

No best-of-one value is presented as typical performance. No timing threshold
runs in shared CI.

## Capture identity

One persisted capture includes:

- start and completion timestamps;
- normalized repository-relative suite path when applicable, schema, name,
  and SHA-256;
- controller version, commit, and source state;
- complete Day 5 environment report;
- normalized repository-relative engine path when applicable and SHA-256;
- workload path, schema, name, dimensions, and SHA-256 per scenario;
- complete raw engine result;
- Go orchestration duration;
- derived descriptive statistics.

Persistence is all-or-nothing. Go writes and fsyncs a temporary file, then
publishes it with an atomic no-overwrite link. An existing capture is never
silently replaced.

## Non-claims

A Day 5 capture establishes a measured scalar baseline. It does not establish:

- SIMD, multicore, or GPU speedup;
- causal explanations for hot stages;
- stable performance across machines;
- production service throughput;
- a regression threshold suitable for shared CI.

Day 6 profiles and interprets the baseline before any optimization decision is
made.
