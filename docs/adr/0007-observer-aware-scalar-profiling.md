# ADR 0007 — Observer-aware scalar profiling

## Status

Accepted for Day 6.

## Context

Day 5 established a fused scalar measurement boundary. Day 6 needs enough
stage-level evidence to explain that baseline, but placing a timer around every
record operation would perturb small stages heavily. Splitting the pipeline
into explicit passes makes coarse stage timers possible, but also changes
allocation, fusion, and memory traffic.

Treating those stage numbers as an exact decomposition of the fused loop would
hide the observer effect and create a misleading portfolio claim.

## Decision

ParaFlow will:

1. preserve the Day 5 benchmark contracts and fused implementation unchanged;
2. introduce separate profile request, engine-result, and report schemas;
3. execute a diagnostic `materialized-stage-passes-v1` topology;
4. use one `boundary-timers-v1` timer around each complete stage pass;
5. retain the topology and observer names in every result;
6. pair each profile with a fresh fused baseline over the exact same suite
   scenario;
7. require exact canonical result equality and identical engine build identity;
8. publish the stage-pass/fused ratio only as an observer/topology ratio;
9. retain raw samples and derive reproducible median/MAD and integer ratios;
10. reject overwrite, source mismatch, timing non-conservation, or in-suite
    engine mutation before persistence.

Intermediate stage buffers remain internal to `paraflow-engine`. They are not a
public Rust API, workload field, native ABI, or permanent layout choice.

## Consequences

### Positive

- The fused denominator remains comparable with later backends.
- Stage timing uses five coarse boundaries instead of clocks inside the record
  loop.
- Reports disclose exactly which physical topology produced each number.
- Exact paired results catch semantic drift introduced by diagnostic code.
- The report becomes a reusable evidence envelope for later profile variants.

### Costs

- Every scenario runs twice, increasing experiment duration.
- The stage profile allocates more buffers and moves more data than the fused
  path.
- Stage medians do not reconstruct the fused pipeline median.
- The profiler identifies investigation candidates but cannot by itself prove
  a hardware bottleneck.

## Alternatives considered

### Time every operation per record

Rejected because clock-read and bookkeeping frequency would scale with record
count and could dominate the measured stage.

### Insert stage timers into the fused loop

Rejected because it would silently change the trusted Day 5 denominator and
still lack clean stage boundaries.

### Replace the fused baseline with materialized passes

Rejected because it would turn diagnostic convenience into an unmeasured
layout/fusion decision.

### Use a system profiler only

Deferred. Sampling profilers and hardware counters can add useful attribution,
but Day 6 first needs portable, versioned, cross-language evidence and exact
result pairing.
