# Benchmark methodology

## Purpose

ParaFlow benchmarks are engineering evidence, not decorative numbers. A timing
is useful only when its workload, implementation, boundary, machine, source,
and correctness status can be reconstructed.

Day 5 establishes the scalar baseline. Day 6 profiles and explains it. Later
weeks may compare SIMD, multicore, scheduling, layout, synchronization, and GPU
backends under the same evidence rules.

## Contract separation

- **Workload contract:** dataset and pipeline meaning.
- **Execution contract:** backend and process interaction.
- **Measurement contract:** warm-ups, samples, clocks, timing boundaries,
  metadata, and persistence.
- **Profile contract:** diagnostic topology, observer, stage boundaries, and
  paired analysis.

A benchmark setting may change how evidence is collected, but not what the
workload computes.

| Boundary        | Includes                                                   |
| --------------- | ---------------------------------------------------------- |
| `generation`    | Deterministic record materialization, including allocation |
| `compute`       | Normalize through aggregate                                |
| `engine_total`  | Generation, compute, validation, and engine bookkeeping    |
| `orchestration` | Go control-plane and process/protocol overhead             |

## Required measurement process

1. Build the measured engine in release mode.
2. Record controller and engine source identity, including dirty state, and
   require both binaries to report the same commit and source state.
3. Hash the exact suite, workloads, and engine executable; hash the engine again
   after the final scenario and reject a mixed-binary run.
4. Capture OS, kernel, CPU model, physical/logical CPU counts, Go runtime,
   compiler/tool versions, and `GOMAXPROCS`.
5. Strictly validate the workload and compute the scalar-oracle reference before
   sampling.
6. Execute warm-ups without retaining them as observations.
7. Retain every timed sample; never store only an average.
8. Validate every sample's result against the oracle.
9. Run scenarios sequentially unless interference is the subject under study.
10. Persist the complete capture atomically and never overwrite prior evidence.

## Day 5 boundaries

One Rust process performs all warm-ups and retained samples for one scenario.
Process startup is excluded from per-sample engine time and included in Go's
orchestration time.

- `generation_ns`: fresh allocation plus deterministic logical-record
  materialization.
- `pipeline_ns`: normalize through aggregate over that materialized batch.
- `engine_total_ns`: generation, pipeline, exact correctness comparison,
  reclamation of per-iteration record/result buffers, and engine bookkeeping.
- `experiment_total_ns`: all warm-ups and retained iterations inside Rust.
- `orchestration_total_ns`: Go encoding, process launch, transport, complete
  engine execution, decoding, and validation.

The current materialized representation is a scalar `Vec<LogicalRecord>`. This
is a measurement boundary, not a frozen AoS ABI or a decision against future
SoA layouts.

## Day 6 profile boundaries

Day 6 does not modify the fused benchmark boundary. For each scenario, Go first
collects a fresh fused result and then starts a separate Rust profile process.
The profile uses `materialized-stage-passes-v1` and identifies its instrumentation
as `boundary-timers-v1`.

| Profile boundary | Includes |
| --- | --- |
| `generation_ns` | fresh logical-record allocation and materialization |
| `normalize_ns` | normalized-buffer allocation and complete normalize pass |
| `score_ns` | scored-buffer allocation and complete score pass |
| `filter_ns` | accepted-buffer allocation and stable filter pass |
| `aggregate_ns` | histogram allocation and stable aggregate pass |
| `stage_sum_ns` | exact sum of the five declared stage boundaries |
| `profile_total_ns` | stage work, exact comparison, reclamation, and bookkeeping |

One timer surrounds each pass; no clock is read per record. Every profile sample
must satisfy exact stage-sum and enclosing-total conservation. Both the
materialized profile and fused path must match the streaming scalar oracle and
each other exactly.

The stage passes alter allocation, fusion, memory traffic, and lifetime. Their
medians therefore do not reconstruct the fused pipeline median. The
stage-pass/fused ratio is retained to quantify observer/topology context, not to
claim a speedup.

## Sampling and summaries

The checked-in baseline suite uses five warm-ups and 20–25 retained samples per
scenario. The CI smoke suite uses one warm-up and three samples only to exercise
the boundary.

Report at least:

- sample count;
- median;
- median absolute deviation (MAD);
- minimum;
- maximum;
- all raw samples.

Median and MAD describe typical behavior and spread without allowing one large
outlier to dominate the summary. Minimum can help study achievable execution,
but it must not be labeled typical latency. Best-of-one is not a portfolio
claim.

Day 6 additionally reports:

- accepted-record selectivity in basis points;
- stage shares apportioned to exactly 10,000 basis points;
- dominant all-stage and pipeline-only boundaries;
- fused and materialized pipeline costs per record;
- MAD relative to the median;
- the explicitly labeled materialized-stage/fused-pipeline ratio.

Those fields are derived with overflow-checked integer arithmetic so another
reader can reproduce them exactly from the raw report.

## Correctness gate

Incorrect output has no performance value.

Exact comparison applies to:
- accepted count;
- category histogram;
- integer checksums;
- compacted IDs and their order when the diagnostic is requested.

Day 5's scalar materialized path must match the stable streaming scalar oracle
exactly, including binary64 score-sum bits. Go independently checks result
encodings and conservation invariants before persistence.

Future parallel reductions may use a declared absolute/relative tolerance for
finite floating-point sums while still requiring exact structural results.
That policy must be versioned rather than silently introduced.

## Noise controls

Before a curated local run:

- close unrelated CPU-heavy work;
- use AC power and a stable performance policy when available;
- record, rather than hide, container or virtualization context;
- avoid concurrent scenario execution;
- repeat the complete suite when results appear unstable;
- do not mix samples from different commits or binaries; the controller
  enforces source alignment and re-hashes the engine after the suite.

The 64K uniform and hotspot cases keep seed, dimensions, feature domain, flag
probability, and pipeline parameters identical; only distribution changes.

CPU pinning, frequency control, NUMA placement, and hardware counters are not
required for the Day 6 baseline report, but their absence is a limitation to
state in analysis.

## CI policy

Shared hosted runners are suitable for:

- schema and conformance checks;
- exact correctness tests;
- process-boundary smoke tests;
- proving that raw samples and metadata are produced.

They are not suitable for a fixed percentage performance gate. Placement,
contention, frequency policy, and virtualization noise are uncontrolled. CI
must never fail because a shared-runner timing moved by an arbitrary threshold.

## Claim format

A publishable statement must answer:

1. Faster than which named baseline?
2. On which exact machine and source state?
3. For which workload and execution settings?
4. Which timing boundary is compared?
5. Across how many raw samples and with what spread?
6. Which correctness policy passed?
7. Where does the change lose or stop scaling?

Day 6 still makes no speedup claim because only one implementation exists. The
fused output remains the denominator future comparisons will require; the
profile supplies bounded, observer-aware hypotheses about where to investigate.
