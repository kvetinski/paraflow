# Scalar profile protocol v1

## Scope

The profile protocol carries one self-contained diagnostic scalar stage
experiment between Go and a fresh Rust process. It complements rather than
modifies the Day 5 fused benchmark protocol.

Normative schemas:

- `contracts/profile-request-v1.schema.json`
- `contracts/profile-engine-result-v1.schema.json`
- `contracts/profile-vectors-v1.schema.json`
- `contracts/scalar-profile-report-v1.schema.json`

Portable examples:

- `contracts/conformance/profile-v1.json`
- `contracts/conformance/scalar-profile-report-v1.json`

## Process boundary

`labctl` starts:

```text
paraflow-engine profile
```

It writes one JSON request to stdin, optionally followed by one LF or CRLF, and
reads one JSON result from stdout. The payload limit is 4 MiB excluding that
single optional terminator. Stdout contains only the protocol result; bounded
stderr is diagnostic context for failures.

Process startup and transport are excluded from retained engine samples and
included in Go's `orchestration_total_ns`.

## Request

`paraflow.profile-request/v1` contains:

- an experiment identifier and scenario name;
- scalar execution selection;
- warm-up and retained-sample counts;
- the complete embedded `paraflow.workload/v1`.

The request cannot reference a controller-local workload path.

## Engine result

`paraflow.profile-engine-result/v1` echoes request identity and contains:

- sampling and execution settings;
- the timing clock, unit, topology, and observer;
- exact correctness policy;
- release engine build and source identity;
- every retained raw profile sample;
- the canonical workload result.

The only v1 topology and observer are:

```text
topology = materialized-stage-passes-v1
observer = boundary-timers-v1
```

All nanosecond durations are fixed-width lowercase hexadecimal `u64` values.

## Sampling

Rust validates the workload and computes an untimed streaming scalar result
before starting the experiment. It then runs all warm-ups and retained samples
inside one process. Warm-ups are correctness-checked but not retained.

Every iteration:

1. allocates and deterministically generates `LogicalRecord` values;
2. materializes normalized records;
3. materializes scored records;
4. stably compacts accepted records;
5. aggregates the accepted records;
6. compares the exact result with the streaming oracle.

One monotonic timer surrounds each complete stage pass.

## Sample invariants

For every zero-based, contiguous ordinal:

```text
stage_sum_ns =
    generation_ns
  + normalize_ns
  + score_ns
  + filter_ns
  + aggregate_ns

profile_total_ns >= stage_sum_ns
```

`experiment_total_ns` is at least the sum of retained
`profile_total_ns` values because it also includes warm-ups and experiment
bookkeeping. Integer overflow is a hard failure; durations are never wrapped,
saturated, or converted through JSON numbers.

## Correctness

The declared policy is:

```text
oracle = rust-scalar-v1
comparison = exact
```

Exact comparison includes accepted count, binary64 score-sum bits, category
histogram, accepted-ID sum, and accepted-ID XOR. Every warm-up and retained
iteration must pass.

## Paired report

`paraflow.scalar-profile-report/v1` contains, for each scenario:

- the complete unchanged fused benchmark evidence;
- the complete diagnostic stage-profile evidence;
- suite, workload, executable, environment, controller, and engine identity;
- median, MAD, minimum, maximum, and sample count for every boundary;
- selectivity, stage shares, dominant stages, per-record costs, variability,
  and the stage-pass/fused ratio.

Go rejects a pair unless:

- scenario, workload, execution, and sampling echoes agree;
- controller and engine source identities align;
- the baseline and profile engine builds are identical;
- canonical results are exactly equal;
- the measured engine hash is unchanged across the suite.

Stage shares use largest-remainder apportionment and sum to exactly 10,000 basis
points. Ratio and per-record fields use scaled integer arithmetic with overflow
checks.

## Interpretation rule

The materialized profile is not a decomposition of the fused baseline. It
changes allocation, fusion, and memory-access behavior. Therefore
`stage_pass_to_fused_pipeline_ratio_milli` is descriptive observer/topology
context and must not be described as a speedup or regression.

## Persistence

Reports are encoded as indented JSON, written through a temporary file, and
atomically renamed to a previously absent output path. Existing evidence is
never overwritten.
