//! Versioned scalar profiling request and engine-result messages.
//!
//! Profiling is deliberately separate from the Day 5 benchmark protocol. The
//! existing fused scalar baseline remains unchanged; this protocol measures a
//! diagnostic materialized stage-pass topology so its observer cost is visible
//! instead of being silently attributed to the baseline.

use serde::{Deserialize, Serialize};
use serde_json::value::RawValue;

use crate::{
    ExecutionV1, HexU64, ResultWireV1,
    benchmark::{BenchmarkCorrectnessV1, BenchmarkSamplingV1, EngineBuildV1},
};

/// Version accepted for one scalar profile request.
pub const PROFILE_REQUEST_SCHEMA_V1: &str = "paraflow.profile-request/v1";
/// Version emitted for one engine-side scalar profile result.
pub const PROFILE_ENGINE_RESULT_SCHEMA_V1: &str = "paraflow.profile-engine-result/v1";
/// Diagnostic execution topology used to isolate stage boundaries.
pub const PROFILE_TOPOLOGY_V1: &str = "materialized-stage-passes-v1";
/// Observer strategy used by the Day 6 profiler.
pub const PROFILE_OBSERVER_V1: &str = "boundary-timers-v1";

/// One self-contained scalar profile request sent by the Go control plane.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ProfileRequestV1 {
    /// Profile request schema identifier.
    pub schema_version: String,
    /// Opaque controller-generated experiment correlation identifier.
    pub experiment_id: String,
    /// Scenario identity from the benchmark suite.
    pub scenario_name: String,
    /// Execution settings kept separate from workload meaning.
    pub execution: ExecutionV1,
    /// Warm-up and retained-sample counts.
    pub sampling: BenchmarkSamplingV1,
    /// Complete self-contained workload object.
    pub workload: Box<RawValue>,
}

/// One retained raw scalar stage-profile sample.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ProfileSampleV1 {
    /// Zero-based retained sample ordinal.
    pub ordinal: u32,
    /// Allocation and deterministic logical-record materialization.
    pub generation_ns: HexU64,
    /// Allocation and normalization of every generated record.
    pub normalize_ns: HexU64,
    /// Allocation and scoring of every normalized record.
    pub score_ns: HexU64,
    /// Allocation and stable filtering of every scored record.
    pub filter_ns: HexU64,
    /// Histogram allocation and stable aggregation of accepted records.
    pub aggregate_ns: HexU64,
    /// Exact sum of the five declared stage durations.
    pub stage_sum_ns: HexU64,
    /// All stages, exact correctness comparison, buffer reclamation, and
    /// profiler bookkeeping.
    pub profile_total_ns: HexU64,
}

/// Engine-side metadata defining the diagnostic profile boundary.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ProfileTimingV1 {
    /// Monotonic clock implementation.
    pub clock: String,
    /// Unit for every timing value.
    pub unit: String,
    /// Explicitly false: process launch is outside retained samples.
    pub process_start_in_samples: bool,
    /// Diagnostic execution topology.
    pub topology: String,
    /// Timing observer implementation.
    pub observer: String,
    /// Total engine time across all warm-ups and retained profile samples.
    pub experiment_total_ns: HexU64,
}

/// Successful engine-side scalar profile result.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ProfileEngineResultV1 {
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
    /// Timing-boundary and observer metadata.
    pub timing: ProfileTimingV1,
    /// Exact scalar correctness evidence.
    pub correctness: BenchmarkCorrectnessV1,
    /// Build identity of the profiled Rust binary.
    pub engine_build: EngineBuildV1,
    /// Every retained raw stage-profile sample, in execution order.
    pub samples: Vec<ProfileSampleV1>,
    /// Canonical result produced by every warm-up and retained sample.
    pub result: ResultWireV1,
}
