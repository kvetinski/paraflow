//! Engine-side Day 6 scalar stage profiler.
//!
//! The Day 5 fused benchmark remains the baseline. This module executes a
//! separate materialized pass for normalize, score, filter, and aggregate so
//! each stage has one coarse timing boundary. The resulting topology and its
//! observer cost are explicit in the wire result.

use std::{
    error::Error,
    fmt,
    io::{self, Read, Write},
    time::Instant,
};

use paraflow_contracts::{ResultV1, Validate, ValidationErrors, WorkloadSpec};
use paraflow_protocol::{
    ExecutionV1, HexU64, MAX_FRAME_BYTES, ResultWireV1, SCALAR_BACKEND_V1,
    benchmark::{
        BENCHMARK_CLOCK_V1, BENCHMARK_COMPARISON_V1, BENCHMARK_ORACLE_V1, BENCHMARK_TIME_UNIT_V1,
        BenchmarkCorrectnessV1,
    },
    profile::{
        PROFILE_ENGINE_RESULT_SCHEMA_V1, PROFILE_OBSERVER_V1, PROFILE_REQUEST_SCHEMA_V1,
        PROFILE_TOPOLOGY_V1, ProfileEngineResultV1, ProfileRequestV1, ProfileSampleV1,
        ProfileTimingV1,
    },
};

use crate::{
    benchmark::{
        MAX_SAMPLE_ITERATIONS, MAX_WARMUP_ITERATIONS, current_engine_build, results_equal_exact,
        valid_identifier, valid_name,
    },
    generation::DatasetGenerator,
    scalar::ScalarOracle,
};

/// A profile request could not be decoded, validated, or completed.
#[derive(Debug)]
pub enum ProfileError {
    /// Reading the request failed.
    Read(io::Error),
    /// The request exceeded the shared bounded JSON payload size.
    RequestTooLarge {
        /// Maximum accepted bytes.
        maximum_bytes: usize,
    },
    /// The request JSON shape is invalid.
    InvalidRequestJson(serde_json::Error),
    /// A request-level invariant is invalid.
    InvalidRequest(String),
    /// The embedded workload has invalid JSON shape.
    InvalidWorkloadJson(serde_json::Error),
    /// The embedded workload violates semantic invariants.
    InvalidWorkload(ValidationErrors),
    /// Deterministic generation failed.
    Generation(crate::generation::GenerationError),
    /// Scalar profiling failed.
    Scalar(crate::scalar::ScalarError),
    /// A profiled result did not match the frozen scalar oracle.
    CorrectnessMismatch {
        /// Iteration kind and zero-based index.
        iteration: String,
    },
    /// Stage durations could not be added without overflowing `u64`.
    StageDurationOverflow,
    /// Monotonic timing boundaries violated their declared nesting.
    TimingInvariant(&'static str),
    /// A monotonic duration could not fit the wire representation.
    DurationOverflow,
    /// The response could not be serialized.
    Serialize(serde_json::Error),
    /// The response exceeded the shared bounded JSON payload size.
    ResponseTooLarge {
        /// Actual serialized bytes.
        actual_bytes: usize,
        /// Maximum accepted bytes.
        maximum_bytes: usize,
    },
    /// Writing the result failed.
    Write(io::Error),
}

impl fmt::Display for ProfileError {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Read(error) => write!(formatter, "read profile request: {error}"),
            Self::RequestTooLarge { maximum_bytes } => {
                write!(
                    formatter,
                    "profile request exceeds the {maximum_bytes}-byte limit"
                )
            }
            Self::InvalidRequestJson(error) => {
                write!(formatter, "invalid profile request JSON: {error}")
            }
            Self::InvalidRequest(message) => {
                write!(formatter, "invalid profile request: {message}")
            }
            Self::InvalidWorkloadJson(error) => {
                write!(formatter, "invalid embedded workload JSON: {error}")
            }
            Self::InvalidWorkload(errors) => {
                write!(formatter, "invalid embedded workload: {errors}")
            }
            Self::Generation(error) => write!(formatter, "generate materialized batch: {error}"),
            Self::Scalar(error) => write!(formatter, "execute scalar stage profile: {error}"),
            Self::CorrectnessMismatch { iteration } => write!(
                formatter,
                "profiled scalar result mismatched the oracle during {iteration}"
            ),
            Self::StageDurationOverflow => {
                formatter.write_str("sum of stage durations exceeds the u64 nanosecond domain")
            }
            Self::TimingInvariant(message) => {
                write!(formatter, "profile timing invariant failed: {message}")
            }
            Self::DurationOverflow => {
                formatter.write_str("measured duration exceeds the u64 nanosecond domain")
            }
            Self::Serialize(error) => write!(formatter, "serialize profile result: {error}"),
            Self::ResponseTooLarge {
                actual_bytes,
                maximum_bytes,
            } => write!(
                formatter,
                "profile response is {actual_bytes} bytes; maximum is {maximum_bytes}"
            ),
            Self::Write(error) => write!(formatter, "write profile result: {error}"),
        }
    }
}

impl Error for ProfileError {
    fn source(&self) -> Option<&(dyn Error + 'static)> {
        match self {
            Self::Read(error) | Self::Write(error) => Some(error),
            Self::InvalidRequestJson(error)
            | Self::InvalidWorkloadJson(error)
            | Self::Serialize(error) => Some(error),
            Self::InvalidWorkload(errors) => Some(errors),
            Self::Generation(error) => Some(error),
            Self::Scalar(error) => Some(error),
            Self::RequestTooLarge { .. }
            | Self::InvalidRequest(_)
            | Self::CorrectnessMismatch { .. }
            | Self::StageDurationOverflow
            | Self::TimingInvariant(_)
            | Self::DurationOverflow
            | Self::ResponseTooLarge { .. } => None,
        }
    }
}

/// Read one bounded request, execute it, and write one JSON profile result.
pub fn run(input: &mut impl Read, output: &mut impl Write) -> Result<(), ProfileError> {
    let mut request_bytes = Vec::new();
    let read_limit = MAX_FRAME_BYTES + 3;
    input
        .take(u64::try_from(read_limit).expect("frame bound must fit in u64"))
        .read_to_end(&mut request_bytes)
        .map_err(ProfileError::Read)?;
    if request_bytes.len() > MAX_FRAME_BYTES + 2 {
        return Err(ProfileError::RequestTooLarge {
            maximum_bytes: MAX_FRAME_BYTES,
        });
    }
    strip_optional_line_ending(&mut request_bytes)?;
    if request_bytes.len() > MAX_FRAME_BYTES {
        return Err(ProfileError::RequestTooLarge {
            maximum_bytes: MAX_FRAME_BYTES,
        });
    }

    let request = serde_json::from_slice::<ProfileRequestV1>(&request_bytes)
        .map_err(ProfileError::InvalidRequestJson)?;
    let result = execute(request)?;
    let payload = serde_json::to_vec(&result).map_err(ProfileError::Serialize)?;
    if payload.len() > MAX_FRAME_BYTES {
        return Err(ProfileError::ResponseTooLarge {
            actual_bytes: payload.len(),
            maximum_bytes: MAX_FRAME_BYTES,
        });
    }
    output.write_all(&payload).map_err(ProfileError::Write)?;
    output.write_all(b"\n").map_err(ProfileError::Write)?;
    output.flush().map_err(ProfileError::Write)
}

/// Execute one already-decoded scalar profile request.
pub fn execute(request: ProfileRequestV1) -> Result<ProfileEngineResultV1, ProfileError> {
    validate_request(&request)?;

    let workload = serde_json::from_str::<WorkloadSpec>(request.workload.get())
        .map_err(ProfileError::InvalidWorkloadJson)?;
    workload.validate().map_err(ProfileError::InvalidWorkload)?;

    let oracle = ScalarOracle::try_new(&workload).map_err(ProfileError::Scalar)?;
    let expected = oracle.run_result().map_err(ProfileError::Scalar)?;
    let generator =
        DatasetGenerator::try_new(&workload.dataset).map_err(ProfileError::InvalidWorkload)?;

    let experiment_started = Instant::now();
    for warmup in 0..request.sampling.warmup_iterations {
        run_iteration(&generator, &oracle, &expected, &format!("warm-up {warmup}"))?;
    }

    let sample_capacity = usize::try_from(request.sampling.sample_iterations)
        .expect("u32 sample count must fit in usize on supported targets");
    let mut samples = Vec::with_capacity(sample_capacity);
    for ordinal in 0..request.sampling.sample_iterations {
        let sample = run_iteration(&generator, &oracle, &expected, &format!("sample {ordinal}"))?;
        samples.push(ProfileSampleV1 { ordinal, ..sample });
    }
    let experiment_total_ns = duration_ns(experiment_started.elapsed())?;

    Ok(ProfileEngineResultV1 {
        schema_version: PROFILE_ENGINE_RESULT_SCHEMA_V1.to_owned(),
        experiment_id: request.experiment_id,
        scenario_name: request.scenario_name,
        workload_name: workload.name,
        execution: ExecutionV1::scalar(),
        sampling: request.sampling,
        timing: ProfileTimingV1 {
            clock: BENCHMARK_CLOCK_V1.to_owned(),
            unit: BENCHMARK_TIME_UNIT_V1.to_owned(),
            process_start_in_samples: false,
            topology: PROFILE_TOPOLOGY_V1.to_owned(),
            observer: PROFILE_OBSERVER_V1.to_owned(),
            experiment_total_ns: HexU64::new(experiment_total_ns),
        },
        correctness: BenchmarkCorrectnessV1 {
            status: "passed".to_owned(),
            oracle: BENCHMARK_ORACLE_V1.to_owned(),
            comparison: BENCHMARK_COMPARISON_V1.to_owned(),
        },
        engine_build: current_engine_build(),
        samples,
        result: ResultWireV1::from(expected),
    })
}

fn validate_request(request: &ProfileRequestV1) -> Result<(), ProfileError> {
    if request.schema_version != PROFILE_REQUEST_SCHEMA_V1 {
        return Err(ProfileError::InvalidRequest(format!(
            "unsupported schema_version {:?}",
            request.schema_version
        )));
    }
    if !valid_identifier(&request.experiment_id, 64) {
        return Err(ProfileError::InvalidRequest(
            "experiment_id must match [a-z0-9][a-z0-9._:-]{0,63}".to_owned(),
        ));
    }
    if !valid_name(&request.scenario_name, 120) {
        return Err(ProfileError::InvalidRequest(
            "scenario_name must contain 1..120 non-whitespace characters".to_owned(),
        ));
    }
    if request.execution.backend != SCALAR_BACKEND_V1 {
        return Err(ProfileError::InvalidRequest(format!(
            "backend {:?} is unsupported; expected {:?}",
            request.execution.backend, SCALAR_BACKEND_V1
        )));
    }
    if request.sampling.warmup_iterations > MAX_WARMUP_ITERATIONS {
        return Err(ProfileError::InvalidRequest(format!(
            "warmup_iterations {} exceeds maximum {MAX_WARMUP_ITERATIONS}",
            request.sampling.warmup_iterations
        )));
    }
    if request.sampling.sample_iterations == 0
        || request.sampling.sample_iterations > MAX_SAMPLE_ITERATIONS
    {
        return Err(ProfileError::InvalidRequest(format!(
            "sample_iterations must be in 1..={MAX_SAMPLE_ITERATIONS}"
        )));
    }
    Ok(())
}

fn run_iteration(
    generator: &DatasetGenerator<'_>,
    oracle: &ScalarOracle<'_>,
    expected: &ResultV1,
    iteration: &str,
) -> Result<ProfileSampleV1, ProfileError> {
    let profile_started = Instant::now();

    let generation_started = Instant::now();
    let records = generator.generate_all().map_err(ProfileError::Generation)?;
    let generation_ns = duration_ns(generation_started.elapsed())?;

    let profiled = oracle
        .run_materialized_profiled(&records)
        .map_err(ProfileError::Scalar)?;
    let normalize_ns = duration_ns(profiled.stages.normalize)?;
    let score_ns = duration_ns(profiled.stages.score)?;
    let filter_ns = duration_ns(profiled.stages.filter)?;
    let aggregate_ns = duration_ns(profiled.stages.aggregate)?;
    let stage_sum_ns = [
        generation_ns,
        normalize_ns,
        score_ns,
        filter_ns,
        aggregate_ns,
    ]
    .into_iter()
    .try_fold(0_u64, |total, duration| total.checked_add(duration))
    .ok_or(ProfileError::StageDurationOverflow)?;

    if !results_equal_exact(&profiled.result, expected) {
        return Err(ProfileError::CorrectnessMismatch {
            iteration: iteration.to_owned(),
        });
    }

    drop(profiled);
    drop(records);
    let profile_total_ns = duration_ns(profile_started.elapsed())?;
    if profile_total_ns < stage_sum_ns {
        return Err(ProfileError::TimingInvariant(
            "profile_total_ns was smaller than the sum of stage boundaries",
        ));
    }

    Ok(ProfileSampleV1 {
        ordinal: 0,
        generation_ns: HexU64::new(generation_ns),
        normalize_ns: HexU64::new(normalize_ns),
        score_ns: HexU64::new(score_ns),
        filter_ns: HexU64::new(filter_ns),
        aggregate_ns: HexU64::new(aggregate_ns),
        stage_sum_ns: HexU64::new(stage_sum_ns),
        profile_total_ns: HexU64::new(profile_total_ns),
    })
}

fn strip_optional_line_ending(payload: &mut Vec<u8>) -> Result<(), ProfileError> {
    if payload.last() == Some(&b'\n') {
        payload.pop();
        if payload.last() == Some(&b'\r') {
            payload.pop();
        }
    }
    if payload.last() == Some(&b'\r') || payload.last() == Some(&b'\n') {
        return Err(ProfileError::InvalidRequest(
            "request may use only one optional LF or CRLF terminator".to_owned(),
        ));
    }
    Ok(())
}

fn duration_ns(duration: std::time::Duration) -> Result<u64, ProfileError> {
    u64::try_from(duration.as_nanos()).map_err(|_| ProfileError::DurationOverflow)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn profile_payload_accepts_only_one_lf_or_crlf_terminator() {
        let mut lf = b"{}\n".to_vec();
        strip_optional_line_ending(&mut lf).expect("LF must be accepted");
        assert_eq!(lf, b"{}");

        let mut crlf = b"{}\r\n".to_vec();
        strip_optional_line_ending(&mut crlf).expect("CRLF must be accepted");
        assert_eq!(crlf, b"{}");

        for mut invalid in [b"{}\r".to_vec(), b"{}\n\n".to_vec()] {
            assert!(matches!(
                strip_optional_line_ending(&mut invalid),
                Err(ProfileError::InvalidRequest(_))
            ));
        }
    }
}
