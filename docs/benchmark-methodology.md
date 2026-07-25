# Benchmark methodology

Status: **policy defined; executable benchmark harness scheduled for Day 5**

## Milestone boundary

Day 1 rejected timings of schema parsing, CLI startup, and the environment
doctor because they do not measure the intended workload.

Day 2 makes deterministic generation executable and validates it in an
optimized build. It still does not collect or publish timing samples: warm-ups,
raw samples, persistence, and complete experiment identity belong to the Day 5
harness. Until then, `make benchmark-preflight` proves release-build,
conformance, source-identity, and toolchain readiness only.

## Timing boundaries

The future harness reports separate measurements:

| Boundary | Includes |
| --- | --- |
| `generation` | Deterministic record materialization, including allocation |
| `compute` | Normalize through aggregate |
| `engine_total` | Generation, compute, validation, and engine bookkeeping |
| `orchestration` | Go control-plane and process/protocol overhead |

Process startup is excluded from individual compute samples. If measured, it is
reported separately.

An experiment that intentionally reuses preallocated buffers must name that
different boundary explicitly rather than presenting it as the default
`generation` measurement.

## Required experiment identity

Every stored result includes:

- Git commit and dirty-worktree state;
- workload schema, name, and content hash;
- execution configuration;
- compiler and relevant library versions;
- release flags and target architecture;
- OS, architecture, logical CPU count, and available CPU model;
- warm-up count and raw sample count;
- timing clock and units;
- correctness result;
- profiler/tool versions when cited.

## Sampling

- Build optimized binaries before measurement.
- Run warm-ups before recording samples.
- Store every raw sample.
- Report median plus a robust measure of spread.
- Repeat across meaningful input sizes and distributions.
- Run scenarios sequentially unless concurrent interference is itself the
  subject of the experiment.
- Never turn shared-runner timing into a CI pass/fail threshold.

Minimum and best-of-N may be useful for studying achievable machine execution,
but they must not be presented as typical latency.

## Correctness gate

A result is publishable only after the backend matches the scalar oracle under
the declared comparison policy. Incorrect output has no performance value.

Exact comparison applies to:

- accepted count and IDs;
- compacted order;
- category histogram;
- integer checksums.

Floating-point comparison must state absolute and relative tolerances. Results
near the filter threshold require explicit investigation.

## Claim format

A performance statement must answer:

1. Faster than what baseline?
2. On which machine and input?
3. Which timing boundary?
4. Across how many samples?
5. With what correctness check?
6. Where does the optimization lose?

Curated reports live under `results/`. Raw local captures live under
`results/raw/` and are ignored by Git.
