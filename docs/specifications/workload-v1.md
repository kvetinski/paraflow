# ParaFlow workload v1

Status: **normative**

Schema identifier: `paraflow.workload/v1`

## Purpose

This contract freezes logical behavior shared by Rust, C++/ISPC, and CUDA
implementations. It does not freeze a programming-language struct, byte layout,
alignment, allocation strategy, or ABI.

## Logical record

| Field | Type | Meaning |
| --- | --- | --- |
| `id` | unsigned 64-bit | Stable input position |
| `category` | unsigned 32-bit | Value in `[0, category_count)` |
| `feature_a` | signed 32-bit | First raw feature |
| `feature_b` | signed 32-bit | Second raw feature |
| `flags` | unsigned 32-bit | Bit field consumed by scoring |

## Stage order

Every backend preserves:

```text
generate → normalize → score → filter → aggregate
```

Stages may later be fused when fusion is observationally equivalent.

## 1. Generate

`splitmix64-v1` is a counter-derived generator. All arithmetic below is
unsigned 64-bit arithmetic with wraparound:

```text
mix(x):
    z = x + 0x9E3779B97F4A7C15
    z = (z xor (z >> 30)) × 0xBF58476D1CE4E5B9
    z = (z xor (z >> 27)) × 0x94D049BB133111EB
    return z xor (z >> 31)

sample(seed, index, field):
    key = seed
          xor (index × 0xD1B54A32D192ED03)
          xor (field × 0x94D049BB133111EB)
    return mix(key)
```

Field identifiers are:

| Field | Identifier |
| --- | --- |
| Category selector | `0` |
| Feature A | `1` |
| Feature B | `2` |
| Flags | `3` |
| Hotspot fallback category | `4` |

It does not depend on task order, worker count, scheduling, or prior generator
state. Day 2 adds the executable implementation and cross-language reference
vectors for this frozen algorithm.

For each record:

- `id` equals the zero-based record index;
- each feature uses the field identifier listed above and widened arithmetic:

  ```text
  span = u64(i64(feature_max) - i64(feature_min))
  offset = sample(seed, id, field) % span
  feature = i32(i64(feature_min) + i64(offset))
  ```

  Validation guarantees that `span` is nonzero. The intermediate subtraction
  and addition are signed 64-bit operations, so every schema-valid `i32` range
  is defined without signed overflow. The result lies in
  `[feature_min, feature_max)`;
- uniform category is `sample(field=0) % category_count`;
- hotspot category selection succeeds when
  `sample(field=0) % 10000 < probability_bps`;
- when `category_count == 1`, hotspot generation always selects category zero;
- otherwise, a failed hotspot selection excludes the hot category with this
  exact mapping:

  ```text
  hot = configured hotspot category
  slot = sample(seed, id, field=4) % (category_count - 1)
  category = slot + u32(slot >= hot)
  ```

  where `u32(false) = 0` and `u32(true) = 1`;
- flag bit zero is set when
  `sample(field=3) % 10000 < flag_probability_bps`; all other flag bits are
  zero in workload v1.

Probabilities use integer basis points to avoid language-specific JSON float
interpretation.

`record_count = 0` is valid and will produce the aggregation identity. Zero is
useful for correctness tests, but not as a performance scenario.

## 2. Normalize

The reference operation order is:

```text
a = clamp((f32(feature_a) + offset_a) × scale_a, -clip, clip)
b = clamp((f32(feature_b) + offset_b) × scale_b, -clip, clip)
```

Scales and `clip` are finite and strictly positive. Offsets are finite.

The scalar reference is compiled without fast-math. Future optimized backends
must document contraction or reordering that can change rounding.

## 3. Score

The reference operation order is:

```text
score = (a × weight_a + b × weight_b) + bias

if (flags & flag_mask) == flag_mask:
    score = score + flag_bonus
```

All score parameters are finite. A zero `flag_mask` intentionally enables the
bonus for every record.

## 4. Filter and compact

A record is accepted when:

```text
score >= min_score
```

Compaction is stable: accepted records preserve input order. Datasets used for
cross-backend performance reports must avoid threshold-sensitive values unless
the report is specifically studying floating-point differences.

## 5. Aggregate

The canonical `ResultV1` contains:

| Field | Type | Definition |
| --- | --- | --- |
| `accepted_count` | `u64` | Number of accepted records |
| `score_sum` | `f64` | Accepted `f32` scores converted to `f64` and added in stable input order |
| `category_histogram` | `Vec<u64>` | Exactly `category_count` bins; each accepted record increments its category with wrapping `u64` addition |
| `accepted_id_sum` | `u64` | Wrapping sum of accepted record IDs |
| `accepted_id_xor` | `u64` | Bitwise XOR of `mix(id)` for every accepted record |

All integer addition wraps modulo `2^64`. The empty-input identity is
`accepted_count = 0`, `score_sum = 0.0`, an all-zero histogram,
`accepted_id_sum = 0`, and `accepted_id_xor = 0`.

Stable compacted IDs are a diagnostic output, requested separately from
`ResultV1`; when requested they contain every accepted ID in ascending input
order. They are used for detailed correctness tests and are not required in
stored performance results.

The scalar oracle performs each `score_sum = score_sum + f64(score)` operation
in stable input order. Parallel reductions may use a different tree.
`accepted_count`, histogram bins, both ID checksums, and requested compacted IDs
are exact. Score sums use the verification tolerance documented with the
benchmark result.

## Validation

Validation accumulates independent semantic errors in one pass:

- unsupported schema;
- blank or overlong name;
- zero or excessive category count;
- empty feature range;
- probability above 10,000 basis points;
- hotspot outside the category domain;
- non-finite pipeline parameters;
- non-positive normalization scale or clamp.

Unknown JSON fields are rejected. This prevents silently misspelled benchmark
parameters.

## Evolution

Breaking semantic changes require `paraflow.workload/v2`. Adding an execution
backend, data layout, scheduler, or measurement policy does not change this
schema.
