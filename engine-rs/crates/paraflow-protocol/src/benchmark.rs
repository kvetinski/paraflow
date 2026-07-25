//! Versioned benchmark request and engine-result messages.
//!
//! Measurement is intentionally separate from the Day 4 execution protocol.
//! The benchmark request describes how evidence is collected; it never changes
//! workload meaning.

use serde::{Deserialize, Serialize};
use serde_json::value::RawValue;

use crate::{ExecutionV1, HexU64, ResultWireV1};

/// Version accepted for one benchmark experiment request.
pub const BENCHMARK_REQUEST_SCHEMA_V1: &str = "paraflow.benchmark-request/v1";
/// Version emitted for one engine-side benchmark result.
pub const BENCHMARK_ENGINE_RESULT_SCHEMA_V1: &str =
    "paraflow.benchmark-engine-result/v1";
/// Monotonic clock used by the Rust harness.
pub const BENCHMARK_CLOCK_V1: &str = "std::time::Instant";
/// Unit used by every timing field.
pub const BENCHMARK_TIME_UNIT_V1: &str = "nanoseconds";
/// Correctness oracle used before and during scalar sampling.
pub const BENCHMARK_ORACLE_V1: &str = "rust-scalar-v1";
/// Comparison policy for the Day 5 scalar materialized path.
pub const BENCHMARK_COMPARISON_V1: &str = "exact";

/// Sampling configuration that does not alter workload semantics.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct BenchmarkSamplingV1 {
    /// Untimed iterations executed before samples are retained.
    pub warmup_iterations: u32,
    /// Number of raw timed iterations retained in the result.
    pub sample_iterations: u32,
}

/// One self-contained benchmark request sent by the Go control plane.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct BenchmarkRequestV1 {
    /// Benchmark request schema identifier.
    pub schema_version: String,
    /// Opaque controller-generated experiment correlation identifier.
    pub experiment_id: String,
    /// Scenario identity from the benchmark suite.
    pub scenario_name: String,
    /// Execution settings kept separate from the workload.
    pub execution: ExecutionV1,
    /// Warm-up and retained-sample counts.
    pub sampling: BenchmarkSamplingV1,
    /// Complete self-contained workload object.
    pub workload: Box<RawValue>,
}

/// One retained raw timing sample.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct BenchmarkSampleV1 {
    /// Zero-based retained sample ordinal.
    pub ordinal: u32,
    /// Deterministic materialization and allocation time.
    pub generation_ns: HexU64,
    /// Normalize-through-aggregate time over the materialized records.
    pub pipeline_ns: HexU64,
    /// Generation, pipeline, exact validation, temporary-output reclamation,
    /// and engine bookkeeping.
    pub engine_total_ns: HexU64,
}

/// Engine-side timing metadata that defines the measured boundary.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct BenchmarkTimingV1 {
    /// Monotonic clock implementation.
    pub clock: String,
    /// Unit for all timing values.
    pub unit: String,
    /// Explicitly false: process launch is outside retained samples.
    pub process_start_in_samples: bool,
    /// Total engine time across all warm-ups and retained samples.
    pub experiment_total_ns: HexU64,
}

/// Correctness evidence attached to one engine benchmark result.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct BenchmarkCorrectnessV1 {
    /// Must be `passed` for an emitted successful result.
    pub status: String,
    /// Frozen scalar reference implementation.
    pub oracle: String,
    /// Day 5 comparison policy.
    pub comparison: String,
}

/// Build identity embedded in the Rust engine binary.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct EngineBuildV1 {
    /// Cargo package version.
    pub version: String,
    /// Cargo build profile, normally `release` for publishable evidence.
    pub profile: String,
    /// Rust compilation target triple.
    pub target: String,
    /// Rust compiler version used by the build script.
    pub rustc: String,
    /// Full Git commit when available, otherwise `unknown`.
    pub source_commit: String,
    /// `clean`, `dirty`, or `unknown` at engine build time.
    pub source_state: String,
}

/// Successful engine-side benchmark result.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct BenchmarkEngineResultV1 {
    /// Engine-result schema identifier.
    pub schema_version: String,
    /// Echoed experiment correlation identifier.
    pub experiment_id: String,
    /// Echoed scenario identity.
    pub scenario_name: String,
    /// Workload identity after strict Rust decoding and validation.
    pub workload_name: String,
    /// Execution settings actually used.
    pub execution: ExecutionV1,
    /// Echoed sampling configuration.
    pub sampling: BenchmarkSamplingV1,
    /// Timing-boundary metadata.
    pub timing: BenchmarkTimingV1,
    /// Exact scalar correctness evidence.
    pub correctness: BenchmarkCorrectnessV1,
    /// Build identity of the measured Rust binary.
    pub engine_build: EngineBuildV1,
    /// Every retained raw timing sample, in execution order.
    pub samples: Vec<BenchmarkSampleV1>,
    /// Canonical result produced by every warm-up and retained sample.
    pub result: ResultWireV1,
}
