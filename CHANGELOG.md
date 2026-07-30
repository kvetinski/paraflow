# Changelog

All notable ParaFlow portfolio milestones are documented here.

## [0.1.0] — Week 1 scalar baseline

### Added

- Deterministic counter-derived workload generation.
- Typed streaming Rust scalar oracle and exact cross-language result encoding.
- Reusable Go-to-Rust execution protocol with bounded framing and lifecycle
  ownership.
- Release-profile benchmark harness with raw samples, environment/build
  identity, and median/MAD summaries.
- Paired fused/materialized scalar profiling with explicit observer topology
  and integer-only analysis.
- Offline verification of repository identities, raw evidence, summaries, and
  profile analysis through `labctl verify`.
- Versioned verification receipts, adversarial failure tests, and cumulative
  Day 6/7 CS149 understanding sets.

### Release gates

- `make check` runs schemas, formatters, lints, race tests, unit/integration
  tests, real Go/Rust boundaries, evidence replay, and version alignment.
- `make release-check` adds a disposable paired profile without a fixed timing
  threshold.

### Explicit non-claims

- No SIMD, multicore, task-runtime, synchronization, GPU, or speedup result is
  part of `v0.1.0`.
- Materialized stage timings are diagnostic and do not exactly decompose the
  fused scalar loop.

[0.1.0]: https://github.com/kvetinski/paraflow/releases/tag/v0.1.0
