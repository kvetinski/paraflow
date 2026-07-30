# ADR 0008 — Offline verification and release qualification

## Status

Accepted for Day 7 and the Week 1 `v0.1.0` checkpoint.

## Context

Day 5 and Day 6 produce self-contained performance artifacts, but JSON Schema
validation alone cannot prove that their derived summaries still match raw
samples or that repository inputs still match recorded digests. Rerunning a
benchmark is also not verification: timing depends on the host and a new run
creates new evidence rather than authenticating the old artifact.

The first optimization release needs a named scalar denominator whose
correctness, measurement boundaries, inputs, and derivations can be audited
without access to the original benchmark machine.

## Decision

ParaFlow will:

1. add one Go-owned `labctl verify` boundary for both
   `paraflow.benchmark-capture/v1` and
   `paraflow.scalar-profile-report/v1`;
2. strictly decode one closed evidence variant selected by `schema_version`;
3. resolve recorded suite and workload paths only beneath an explicit
   repository root;
4. compare current suite/workload bytes with recorded SHA-256 identities;
5. replay raw engine-result, timing, correctness, build, source, and
   orchestration invariants;
6. recompute every summary and profile-analysis field from retained samples;
7. verify supplied engine bytes by digest when `--engine` is explicit and state
   honestly when historical bytes are unavailable;
8. emit a small, versioned success receipt and emit no receipt on failure;
9. keep verification read-only and never repair or overwrite evidence;
10. use a root `VERSION` plus an executable release gate to align Go, Rust,
    Cargo metadata, and both binaries;
11. require deterministic evidence/version gates in CI while keeping
    machine-sensitive timing thresholds out of shared runners.

## Consequences

### Positive

- Historical timing evidence can be audited without pretending to reproduce
  its nanosecond values.
- Raw samples remain authoritative over summaries and prose.
- Repository drift and derived-field tampering fail closed.
- A verification receipt distinguishes repository identity checks from actual
  engine-byte verification.
- Week 2 begins from an addressable scalar oracle and comparison denominator.

### Costs

- Verification sorts retained sample dimensions again, adding
  `O(R log R)` work for `R` samples in addition to linear file hashing.
- Historical reports cannot prove current possession of the original engine
  binary unless those bytes are supplied separately.
- Adding a new evidence family requires an explicit versioned verifier branch.
- Release qualification builds both language surfaces and runs the existing
  integration gates.

## Alternatives considered

### Treat JSON Schema validation as complete verification

Rejected because schemas validate shape but do not recompute hashes, medians,
MAD, stage sums, paired results, or higher-level analysis.

### Rerun and compare timing values

Rejected because wall-clock timing is environment-sensitive and a new run
cannot authenticate an earlier run.

### Check in release binaries

Rejected for this source portfolio. The report retains the expected digest and
the verifier supports an explicitly supplied artifact without growing Git with
platform-specific binaries.

### Silently ignore unknown fields

Rejected because undefined semantic fields could alter how a reader interprets
an artifact while the verifier falsely claims complete v1 coverage.
