# ADR 0002: Freeze semantics before physical layout

- Status: Accepted
- Date: 2026-07-22

## Context

SIMD, caches, multicore scheduling, FFI, and GPUs favor different layouts.
Choosing a physical representation on Day 1 would bias later experiments.

## Decision

`paraflow.workload/v1` freezes logical fields, generation requirements,
pipeline order, formulas, filtering, and aggregation. It does not freeze:

- AoS or SoA representation;
- padding or alignment;
- host/device allocation;
- Rust or C++ struct layout;
- an FFI buffer type.

## Consequences

Later layout changes remain comparable because semantics do not move. The ABI
must use explicit versioning and adapters rather than exposing internal structs.
