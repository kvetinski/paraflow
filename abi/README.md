# Native ABI boundary

The physical ABI is intentionally deferred until Week 2 supplies a real SIMD
kernel.

The future boundary must follow these rules:

- C ABI with an explicit version handshake;
- fixed-width integer types;
- pointer-plus-length buffers;
- caller-owned memory unless an opaque handle documents otherwise;
- explicit status codes;
- no Rust panic or C++ exception crossing the boundary;
- no exposure of internal Rust or C++ struct layout;
- scalar fallback and cross-backend correctness tests;
- reviewed `unsafe` isolated to a dedicated Rust adapter crate.

Deferring the concrete buffer layout preserves the Week 4 AoS-versus-SoA
experiment.
