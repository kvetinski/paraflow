# ADR 0001: Language ownership

- Status: Accepted
- Date: 2026-07-22

## Context

The portfolio should demonstrate professional Go and Rust engineering while
remaining faithful to CS149's C++/ISPC/CUDA hardware work. Reimplementing the
entire engine three times would consume the roadmap and obscure system design.

## Decision

- Rust owns the execution engine, scalar oracle, concurrency runtime, and
  backend dispatch.
- C++ owns low-level scalar comparison, SIMD/ISPC, and CUDA kernels.
- Go owns experiment control, environment evidence, and future result
  orchestration.
- Go remains outside timed compute regions.
- Go-to-Rust control crosses the versioned process protocol accepted in
  [ADR 0005](0005-long-lived-versioned-worker-protocol.md).
- Future Rust-to-C++ compute crosses one narrow, versioned C ABI.

## Consequences

Each language has a non-artificial responsibility. The project gains an FFI
boundary that must be tested and documented, but avoids three drifting engines.
