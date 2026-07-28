//! Engine-side Day 5 benchmark harness.
//!
//! One Rust process performs correctness preflight, warm-ups, and every retained
//! sample. Process launch, request encoding, transport, and persistence remain
//! Go control-plane work and therefore cannot contaminate individual samples.

use std::{
    error::Error,
    fmt,
    io::{self, Read, Write},
    time::{Duration, Instant},
};

use paraflow_contracts::{ResultV1, Validate, ValidationErrors, WorkloadSpec};
use paraflow_protocol::{
    ExecutionV1, HexU64, MAX_FRAME_BYTES, ResultWireV1, SCALAR_BACKEND_V1,
    benchmark::{
        BENCHMARK_CLOCK_V1, BENCHMARK_COMPARISON_V1, BENCHMARK_ENGINE_RESULT_SCHEMA_V1,
        BENCHMARK_ORACLE_V1, BENCHMARK_REQUEST_SCHEMA_V1, BENCHMARK_TIME_UNIT_V1,
        BenchmarkCorrectnessV1, BenchmarkEngineResultV1, BenchmarkRequestV1, BenchmarkSampleV1,
        BenchmarkTimingV1, EngineBuildV1,
    },
};

use crate::{generation::DatasetGenerator, scalar::ScalarOracle};

/// Maximum untimed warm-up iterations accepted from one request.
pub const MAX_WARMUP_ITERATIONS: u32 = 1_000;
/// Maximum retained samples accepted from one request.
pub const MAX_SAMPLE_ITERATIONS: u32 = 10_000;

/// A benchmark request could not be decoded, validated, or completed.
#[derive(Debug)]
pub enum BenchmarkError {
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
    /// Scalar execution failed.
    Scalar(crate::scalar::ScalarError),
    /// A measured materialized result did not match the frozen scalar oracle.
    CorrectnessMismatch {
        /// Iteration kind and zero-based index.
        iteration: String,
    },
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

impl fmt::Display for BenchmarkError {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Read(error) => write!(formatter, "read benchmark request: {error}"),
            Self::RequestTooLarge { maximum_bytes } => write!(
                formatter,
                "benchmark request exceeds the {maximum_bytes}-byte limit"
            ),
            Self::InvalidRequestJson(error) => {
                write!(formatter, "invalid benchmark request JSON: {error}")
            }
            Self::InvalidRequest(message) => {
                write!(formatter, "invalid benchmark request: {message}")
            }
            Self::InvalidWorkloadJson(error) => {
                write!(formatter, "invalid embedded workload JSON: {error}")
            }
            Self::InvalidWorkload(errors) => {
                write!(formatter, "invalid embedded workload: {errors}")
            }
            Self::Generation(error) => write!(formatter, "generate materialized batch: {error}"),
            Self::Scalar(error) => write!(formatter, "execute scalar pipeline: {error}"),
            Self::CorrectnessMismatch { iteration } => write!(
                formatter,
                "materialized scalar result mismatched the oracle during {iteration}"
            ),
            Self::DurationOverflow => {
                formatter.write_str("measured duration exceeds the u64 nanosecond domain")
            }
            Self::Serialize(error) => write!(formatter, "serialize benchmark result: {error}"),
            Self::ResponseTooLarge {
                actual_bytes,
                maximum_bytes,
            } => write!(
                formatter,
                "benchmark response is {actual_bytes} bytes; maximum is {maximum_bytes}"
            ),
            Self::Write(error) => write!(formatter, "write benchmark result: {error}"),
        }
    }
}

impl Error for BenchmarkError {
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
            | Self::DurationOverflow
            | Self::ResponseTooLarge { .. } => None,
        }
    }
}

/// Read one bounded request, execute it, and write one JSON result.
pub fn run(input: &mut impl Read, output: &mut impl Write) -> Result<(), BenchmarkError> {
    let mut request_bytes = Vec::new();
    let read_limit = MAX_FRAME_BYTES + 3;
    input
        .take(u64::try_from(read_limit).expect("frame bound must fit in u64"))
        .read_to_end(&mut request_bytes)
        .map_err(BenchmarkError::Read)?;
    if request_bytes.len() > MAX_FRAME_BYTES + 2 {
        return Err(BenchmarkError::RequestTooLarge {
            maximum_bytes: MAX_FRAME_BYTES,
        });
    }
    strip_optional_line_ending(&mut request_bytes)?;
    if request_bytes.len() > MAX_FRAME_BYTES {
        return Err(BenchmarkError::RequestTooLarge {
            maximum_bytes: MAX_FRAME_BYTES,
        });
    }

    let request = serde_json::from_slice::<BenchmarkRequestV1>(&request_bytes)
        .map_err(BenchmarkError::InvalidRequestJson)?;
    let result = execute(request)?;
    let payload = serde_json::to_vec(&result).map_err(BenchmarkError::Serialize)?;
    if payload.len() > MAX_FRAME_BYTES {
        return Err(BenchmarkError::ResponseTooLarge {
            actual_bytes: payload.len(),
            maximum_bytes: MAX_FRAME_BYTES,
        });
    }
    output.write_all(&payload).map_err(BenchmarkError::Write)?;
    output.write_all(b"\n").map_err(BenchmarkError::Write)?;
    output.flush().map_err(BenchmarkError::Write)
}

/// Execute one already-decoded benchmark request.
pub fn execute(request: BenchmarkRequestV1) -> Result<BenchmarkEngineResultV1, BenchmarkError> {
    validate_request(&request)?;

    let workload = serde_json::from_str::<WorkloadSpec>(request.workload.get())
        .map_err(BenchmarkError::InvalidWorkloadJson)?;
    workload
        .validate()
        .map_err(BenchmarkError::InvalidWorkload)?;

    let oracle = ScalarOracle::try_new(&workload).map_err(BenchmarkError::Scalar)?;
    let expected = oracle.run_result().map_err(BenchmarkError::Scalar)?;
    let generator =
        DatasetGenerator::try_new(&workload.dataset).map_err(BenchmarkError::InvalidWorkload)?;

    let experiment_started = Instant::now();
    for warmup in 0..request.sampling.warmup_iterations {
        let iteration = format!("warm-up {warmup}");
        run_iteration(&generator, &oracle, &expected, &iteration)?;
    }

    let sample_capacity = usize::try_from(request.sampling.sample_iterations)
        .expect("u32 sample count must fit in usize on supported targets");
    let mut samples = Vec::with_capacity(sample_capacity);
    for ordinal in 0..request.sampling.sample_iterations {
        let iteration = format!("sample {ordinal}");
        let sample = run_iteration(&generator, &oracle, &expected, &iteration)?;
        samples.push(BenchmarkSampleV1 { ordinal, ..sample });
    }
    let experiment_total_ns = duration_ns(experiment_started.elapsed())?;

    Ok(BenchmarkEngineResultV1 {
        schema_version: BENCHMARK_ENGINE_RESULT_SCHEMA_V1.to_owned(),
        experiment_id: request.experiment_id,
        scenario_name: request.scenario_name,
        workload_name: workload.name,
        execution: ExecutionV1::scalar(),
        sampling: request.sampling,
        timing: BenchmarkTimingV1 {
            clock: BENCHMARK_CLOCK_V1.to_owned(),
            unit: BENCHMARK_TIME_UNIT_V1.to_owned(),
            process_start_in_samples: false,
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

fn validate_request(request: &BenchmarkRequestV1) -> Result<(), BenchmarkError> {
    if request.schema_version != BENCHMARK_REQUEST_SCHEMA_V1 {
        return Err(BenchmarkError::InvalidRequest(format!(
            "unsupported schema_version {:?}",
            request.schema_version
        )));
    }
    if !valid_identifier(&request.experiment_id, 64) {
        return Err(BenchmarkError::InvalidRequest(
            "experiment_id must match [a-z0-9][a-z0-9._:-]{0,63}".to_owned(),
        ));
    }
    if !valid_name(&request.scenario_name, 120) {
        return Err(BenchmarkError::InvalidRequest(
            "scenario_name must contain 1..120 non-whitespace characters".to_owned(),
        ));
    }
    if request.execution.backend != SCALAR_BACKEND_V1 {
        return Err(BenchmarkError::InvalidRequest(format!(
            "backend {:?} is unsupported; expected {:?}",
            request.execution.backend, SCALAR_BACKEND_V1
        )));
    }
    if request.sampling.warmup_iterations > MAX_WARMUP_ITERATIONS {
        return Err(BenchmarkError::InvalidRequest(format!(
            "warmup_iterations {} exceeds maximum {MAX_WARMUP_ITERATIONS}",
            request.sampling.warmup_iterations
        )));
    }
    if request.sampling.sample_iterations == 0
        || request.sampling.sample_iterations > MAX_SAMPLE_ITERATIONS
    {
        return Err(BenchmarkError::InvalidRequest(format!(
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
) -> Result<BenchmarkSampleV1, BenchmarkError> {
    let engine_started = Instant::now();

    let generation_started = Instant::now();
    let records = generator
        .generate_all()
        .map_err(BenchmarkError::Generation)?;
    let generation_ns = duration_ns(generation_started.elapsed())?;

    let pipeline_started = Instant::now();
    let result = oracle
        .run_materialized_result(&records)
        .map_err(BenchmarkError::Scalar)?;
    let pipeline_ns = duration_ns(pipeline_started.elapsed())?;

    if !results_equal_exact(&result, expected) {
        return Err(BenchmarkError::CorrectnessMismatch {
            iteration: iteration.to_owned(),
        });
    }

    drop(result);
    drop(records);
    let engine_total_ns = duration_ns(engine_started.elapsed())?;

    Ok(BenchmarkSampleV1 {
        ordinal: 0,
        generation_ns: HexU64::new(generation_ns),
        pipeline_ns: HexU64::new(pipeline_ns),
        engine_total_ns: HexU64::new(engine_total_ns),
    })
}

fn strip_optional_line_ending(payload: &mut Vec<u8>) -> Result<(), BenchmarkError> {
    if payload.last() == Some(&b'\n') {
        payload.pop();
        if payload.last() == Some(&b'\r') {
            payload.pop();
        }
    }
    if payload.last() == Some(&b'\r') || payload.last() == Some(&b'\n') {
        return Err(BenchmarkError::InvalidRequest(
            "request may use only one optional LF or CRLF terminator".to_owned(),
        ));
    }
    Ok(())
}

fn duration_ns(duration: Duration) -> Result<u64, BenchmarkError> {
    u64::try_from(duration.as_nanos()).map_err(|_| BenchmarkError::DurationOverflow)
}

fn results_equal_exact(actual: &ResultV1, expected: &ResultV1) -> bool {
    actual.accepted_count == expected.accepted_count
        && actual.score_sum.to_bits() == expected.score_sum.to_bits()
        && actual.category_histogram == expected.category_histogram
        && actual.accepted_id_sum == expected.accepted_id_sum
        && actual.accepted_id_xor == expected.accepted_id_xor
}

fn valid_identifier(value: &str, maximum_bytes: usize) -> bool {
    let bytes = value.as_bytes();
    let Some(first) = bytes.first() else {
        return false;
    };
    bytes.len() <= maximum_bytes
        && (first.is_ascii_lowercase() || first.is_ascii_digit())
        && bytes.iter().skip(1).all(|byte| {
            byte.is_ascii_lowercase()
                || byte.is_ascii_digit()
                || matches!(byte, b'.' | b'_' | b':' | b'-')
        })
}

fn valid_name(value: &str, maximum_characters: usize) -> bool {
    let length = value.chars().count();
    length != 0
        && length <= maximum_characters
        && value.chars().any(|character| !character.is_whitespace())
}

fn current_engine_build() -> EngineBuildV1 {
    EngineBuildV1 {
        version: env!("CARGO_PKG_VERSION").to_owned(),
        profile: env!("PARAFLOW_BUILD_PROFILE").to_owned(),
        target: env!("PARAFLOW_BUILD_TARGET").to_owned(),
        rustc: env!("PARAFLOW_BUILD_RUSTC").to_owned(),
        source_commit: env!("PARAFLOW_BUILD_GIT_COMMIT").to_owned(),
        source_state: env!("PARAFLOW_BUILD_GIT_STATE").to_owned(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn identifiers_use_the_portable_lowercase_alphabet() {
        let maximum = "a".repeat(64);
        for valid in ["0", "day05.001", "suite_case-1", maximum.as_str()] {
            assert!(valid_identifier(valid, 64), "{valid:?} must be valid");
        }

        let too_long = "a".repeat(65);
        for invalid in ["", "Upper", "-leading", "contains space", too_long.as_str()] {
            assert!(
                !valid_identifier(invalid, 64),
                "{invalid:?} must be invalid"
            );
        }
    }

    #[test]
    fn request_payload_accepts_only_one_lf_or_crlf_terminator() {
        let mut lf = b"{}\n".to_vec();
        strip_optional_line_ending(&mut lf).expect("LF must be accepted");
        assert_eq!(lf, b"{}");

        let mut crlf = b"{}\r\n".to_vec();
        strip_optional_line_ending(&mut crlf).expect("CRLF must be accepted");
        assert_eq!(crlf, b"{}");

        for mut invalid in [b"{}\r".to_vec(), b"{}\n\n".to_vec()] {
            assert!(matches!(
                strip_optional_line_ending(&mut invalid),
                Err(BenchmarkError::InvalidRequest(_))
            ));
        }
    }

    #[test]
    fn exact_result_comparison_preserves_signed_zero_bits() {
        let positive = ResultV1 {
            accepted_count: 0,
            score_sum: 0.0,
            category_histogram: vec![0],
            accepted_id_sum: 0,
            accepted_id_xor: 0,
        };
        let mut negative = positive.clone();
        negative.score_sum = -0.0;

        assert!(!results_equal_exact(&positive, &negative));
    }
}
