# Day 5 — Reproducible scalar benchmark harness

## Objective

Measure the scalar pipeline credibly without putting Go process launch or JSON
transport inside each retained compute sample.

**Status:** implemented for `v0.1.0-alpha.2`; the full machine-specific capture
is intentionally produced by `make benchmark-day05`, not fabricated in source.

## Delivered architecture

```text
benchmark suite
      |
      v
Go labctl benchmark
  - validates suite
  - hashes suite/workloads/engine
  - captures host, source, and toolchains
  - starts one process per scenario
      |
      v
Rust paraflow-engine benchmark
  - strict embedded workload validation
  - untimed scalar oracle preflight
  - warm-ups
  - repeated retained samples
  - exact result check per iteration
      |
      v
Go strict validation + median/MAD + atomic capture
```

## Implementation milestones

1. Add versioned suite, request, engine-result, environment, and capture schemas.
2. Refactor the scalar oracle so the benchmark path can execute over a freshly
   materialized logical batch without exposing a public layout contract.
3. Add a one-shot Rust harness with warm-ups, raw samples, stage boundaries,
   build identity, and exact correctness checks.
4. Add a Go benchmark package for strict process control, response validation,
   descriptive statistics, source alignment, pre/post engine hashing, and
   no-overwrite atomic persistence.
5. Add small, medium, large, and skewed scenarios.
6. Add unit, conformance, and real Go-to-Rust process tests.
7. Add `benchmark-smoke` for disposable CI evidence and `benchmark-day05` for a
   full local capture.
8. Document exact timing boundaries, limitations, and non-claims.

## Test strategy

### Rust

- benchmark request/result fixture round-trip;
- warm-up and retained-sample counts;
- exact result equality, including signed-zero bits;
- invalid sample counts;
- strict unknown-field rejection;
- exact-limit LF/CRLF framing and oversized-payload rejection;
- release real-process execution through Go.

### Go

- duplicate-key and unknown-field rejection;
- canonical hexadecimal duration encoding;
- suite validation and safe repository paths;
- release-profile enforcement;
- correlation, sample ordinal, timing conservation, and result invariants;
- integer median and MAD, including overflow boundaries;
- complete capture construction;
- atomic no-overwrite persistence;
- controller/engine source mismatch and in-suite engine mutation rejection;
- CLI parsing and human-readable summary;
- real release-engine smoke test.

### Contracts

- every workload and suite validates under Draft 2020-12;
- portable benchmark request/result and capture fixtures validate;
- malformed duration, mixed process boundary, and unknown-field regressions are
  rejected.

## Benchmark scenarios

| Scenario           |   Records | Distribution | Warm-ups | Samples | Purpose                                                                    |
| ------------------ | --------: | ------------ | -------: | ------: | -------------------------------------------------------------------------- |
| scalar-uniform-1k  |     1,024 | uniform      |        5 |      25 | expose fixed overhead and small-batch variance                             |
| scalar-uniform-64k |    65,536 | uniform      |        5 |      25 | representative CPU batch                                                   |
| scalar-hotspot-64k |    65,536 | hotspot      |        5 |      25 | isolate distribution skew against the otherwise identical 64K uniform case |
| scalar-uniform-1m  | 1,048,576 | uniform      |        5 |      20 | large-batch scaling and allocation pressure                                |

The smoke suite uses one warm-up and three retained 1K samples.

## Commands

```bash
make check
make benchmark-smoke
make benchmark-day05
```

`benchmark-day05` writes a new immutable file under `results/raw/`. Raw captures
are ignored by Git until deliberately curated with a Day 6 analysis.

## Acceptance gate

Day 5 is complete when:

- one Rust process performs all samples for a scenario;
- process startup is excluded from engine samples and visible separately;
- generation, pipeline, engine-total, and orchestration boundaries are distinct;
- every sample is retained;
- every sample matches the scalar oracle exactly;
- only release-profile engine results are accepted;
- suite, workload, engine, environment, and source identity are persisted;
- controller and engine source identity agree, and the engine hash is stable for
  the entire suite;
- output is atomic and never overwrites prior evidence;
- CI runs a disposable smoke suite without a performance threshold;
- the README alone explains how to reproduce the capture and what it does not
  prove.
