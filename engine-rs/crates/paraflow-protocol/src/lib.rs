//! Strict, lossless, versioned messages for the ParaFlow worker boundary.
//!
//! The protocol describes process communication. It does not change workload
//! meaning, promise an in-memory ABI, or introduce measurement semantics.

#![forbid(unsafe_code)]

pub mod benchmark;
mod hex;

use std::{error::Error, fmt};

pub use hex::HexU64;
use paraflow_contracts::ResultV1;
use serde::{Deserialize, Serialize};
use serde_json::value::RawValue;

/// Version accepted for Day 4 execution requests.
pub const JOB_SCHEMA_V1: &str = "paraflow.job/v1";
/// Version emitted for Day 4 execution responses.
pub const JOB_RESULT_SCHEMA_V1: &str = "paraflow.job-result/v1";
/// Version used by the logical result wire representation.
pub const RESULT_SCHEMA_V1: &str = "paraflow.result/v1";
/// Only execution backend available at the Day 4 boundary.
pub const SCALAR_BACKEND_V1: &str = "scalar";
/// Largest accepted request or response frame, excluding its newline.
pub const MAX_FRAME_BYTES: usize = 4 * 1024 * 1024;

/// Execution settings that remain separate from workload semantics.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ExecutionV1 {
    /// Requested implementation family.
    pub backend: String,
}

impl ExecutionV1 {
    /// Select the Day 4 scalar backend.
    #[must_use]
    pub fn scalar() -> Self {
        Self {
            backend: SCALAR_BACKEND_V1.to_owned(),
        }
    }
}

/// Payload of an execute request.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ExecuteJobV1 {
    /// Execution settings that do not alter workload meaning.
    pub execution: ExecutionV1,
    /// Complete self-contained workload object.
    pub workload: Box<RawValue>,
}

/// Strict execute request envelope.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ExecuteRequestV1 {
    /// Request schema identifier.
    pub schema_version: String,
    /// Opaque session-local correlation identifier.
    pub request_id: String,
    /// Must be `execute`.
    pub kind: String,
    /// Work submitted to the Rust process.
    pub job: ExecuteJobV1,
}

/// Strict graceful-shutdown request envelope.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ShutdownRequestV1 {
    /// Request schema identifier.
    pub schema_version: String,
    /// Opaque session-local correlation identifier.
    pub request_id: String,
    /// Must be `shutdown`.
    pub kind: String,
}

/// A decoded and validated job request.
#[derive(Debug, Clone)]
pub enum JobRequestV1 {
    /// Execute one self-contained workload.
    Execute(ExecuteRequestV1),
    /// Flush the final acknowledgment and exit cleanly.
    Shutdown(ShutdownRequestV1),
}

impl JobRequestV1 {
    /// Correlation identifier carried by this request.
    #[must_use]
    pub fn request_id(&self) -> &str {
        match self {
            Self::Execute(request) => &request.request_id,
            Self::Shutdown(request) => &request.request_id,
        }
    }
}

#[derive(Debug, Deserialize)]
struct RequestHeader {
    schema_version: String,
    request_id: String,
    kind: String,
    #[serde(default, rename = "job")]
    _job: Option<Box<RawValue>>,
}

/// A request frame is malformed or violates the v1 envelope contract.
#[derive(Debug)]
pub enum RequestDecodeError {
    /// The frame is not one complete JSON value.
    MalformedJson(serde_json::Error),
    /// The JSON value does not contain the required common fields.
    InvalidHeader(serde_json::Error),
    /// The peer uses an unsupported request schema.
    UnsupportedSchema(String),
    /// The request identifier cannot be safely correlated.
    InvalidRequestId,
    /// The requested backend is not a valid protocol identifier.
    InvalidBackendIdentifier,
    /// The request kind is unknown.
    UnsupportedKind(String),
    /// Kind-specific fields are missing, unknown, or invalid.
    InvalidPayload(serde_json::Error),
}

impl fmt::Display for RequestDecodeError {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::MalformedJson(error) => write!(formatter, "malformed JSON frame: {error}"),
            Self::InvalidHeader(error) => write!(formatter, "invalid request header: {error}"),
            Self::UnsupportedSchema(schema) => {
                write!(formatter, "unsupported request schema {schema:?}")
            }
            Self::InvalidRequestId => {
                formatter.write_str("request_id must match [A-Za-z0-9][A-Za-z0-9._:-]{0,63}")
            }
            Self::InvalidBackendIdentifier => {
                formatter.write_str("backend must match [A-Za-z][A-Za-z0-9._-]{0,63}")
            }
            Self::UnsupportedKind(kind) => write!(formatter, "unsupported request kind {kind:?}"),
            Self::InvalidPayload(error) => write!(formatter, "invalid request payload: {error}"),
        }
    }
}

impl Error for RequestDecodeError {
    fn source(&self) -> Option<&(dyn Error + 'static)> {
        match self {
            Self::MalformedJson(error)
            | Self::InvalidHeader(error)
            | Self::InvalidPayload(error) => Some(error),
            Self::UnsupportedSchema(_)
            | Self::InvalidRequestId
            | Self::InvalidBackendIdentifier
            | Self::UnsupportedKind(_) => None,
        }
    }
}

/// Strictly decode one JSON frame and validate its common v1 fields.
pub fn decode_request(frame: &[u8]) -> Result<JobRequestV1, RequestDecodeError> {
    let raw = serde_json::from_slice::<Box<RawValue>>(frame)
        .map_err(RequestDecodeError::MalformedJson)?;
    let header = serde_json::from_str::<RequestHeader>(raw.get())
        .map_err(RequestDecodeError::InvalidHeader)?;

    if header.schema_version != JOB_SCHEMA_V1 {
        return Err(RequestDecodeError::UnsupportedSchema(header.schema_version));
    }
    if !is_valid_request_id(&header.request_id) {
        return Err(RequestDecodeError::InvalidRequestId);
    }

    match header.kind.as_str() {
        "execute" => {
            let request = serde_json::from_str::<ExecuteRequestV1>(raw.get())
                .map_err(RequestDecodeError::InvalidPayload)?;
            if !is_valid_backend_identifier(&request.job.execution.backend) {
                return Err(RequestDecodeError::InvalidBackendIdentifier);
            }
            Ok(JobRequestV1::Execute(request))
        }
        "shutdown" => {
            let request = serde_json::from_str::<ShutdownRequestV1>(raw.get())
                .map_err(RequestDecodeError::InvalidPayload)?;
            Ok(JobRequestV1::Shutdown(request))
        }
        _ => Err(RequestDecodeError::UnsupportedKind(header.kind)),
    }
}

fn is_valid_backend_identifier(value: &str) -> bool {
    let bytes = value.as_bytes();
    let Some(first) = bytes.first() else {
        return false;
    };
    bytes.len() <= 64
        && first.is_ascii_alphabetic()
        && bytes
            .iter()
            .skip(1)
            .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'.' | b'_' | b'-'))
}

fn is_valid_request_id(value: &str) -> bool {
    let bytes = value.as_bytes();
    let Some(first) = bytes.first() else {
        return false;
    };
    bytes.len() <= 64
        && first.is_ascii_alphanumeric()
        && bytes
            .iter()
            .skip(1)
            .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'.' | b'_' | b':' | b'-'))
}

/// Marker serialized for exact binary64 bit transport.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum FloatEncodingV1 {
    /// IEEE-754 binary64 encoded through its raw `u64` bits.
    #[serde(rename = "ieee754-binary64")]
    Ieee754Binary64,
}

/// Lossless floating-point representation, including positive infinity.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct Float64BitsV1 {
    /// Bit encoding used by this value.
    pub encoding: FloatEncodingV1,
    /// Raw IEEE-754 binary64 bits.
    pub bits: HexU64,
}

impl Float64BitsV1 {
    /// Encode one logical `f64` without JSON-number loss.
    #[must_use]
    pub fn from_float(value: f64) -> Self {
        Self {
            encoding: FloatEncodingV1::Ieee754Binary64,
            bits: HexU64::new(value.to_bits()),
        }
    }

    /// Recover the exact logical `f64`.
    #[must_use]
    pub fn to_float(self) -> f64 {
        f64::from_bits(self.bits.value())
    }
}

/// Lossless wire form of the canonical logical result.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ResultWireV1 {
    /// Logical result schema identifier.
    pub schema_version: String,
    /// Number of accepted records.
    pub accepted_count: HexU64,
    /// Stable scalar sum represented by its exact bits.
    pub score_sum: Float64BitsV1,
    /// One fixed-width wrapping count per category.
    pub category_histogram: Vec<HexU64>,
    /// Wrapping sum of accepted IDs.
    pub accepted_id_sum: HexU64,
    /// XOR of mixed accepted IDs.
    pub accepted_id_xor: HexU64,
}

impl From<ResultV1> for ResultWireV1 {
    fn from(result: ResultV1) -> Self {
        Self {
            schema_version: RESULT_SCHEMA_V1.to_owned(),
            accepted_count: result.accepted_count.into(),
            score_sum: Float64BitsV1::from_float(result.score_sum),
            category_histogram: result
                .category_histogram
                .into_iter()
                .map(HexU64::from)
                .collect(),
            accepted_id_sum: result.accepted_id_sum.into(),
            accepted_id_xor: result.accepted_id_xor.into(),
        }
    }
}

/// Successful execution response.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct CompletedResponseV1 {
    /// Response schema identifier.
    pub schema_version: String,
    /// Echoed request correlation identifier.
    pub request_id: String,
    /// Must be `completed`.
    pub kind: String,
    /// Echoed workload identity.
    pub workload_name: String,
    /// Execution settings actually used.
    pub execution: ExecutionV1,
    /// Lossless canonical result.
    pub result: ResultWireV1,
}

impl CompletedResponseV1 {
    /// Construct a completed scalar response.
    #[must_use]
    pub fn scalar(request_id: String, workload_name: String, result: ResultV1) -> Self {
        Self {
            schema_version: JOB_RESULT_SCHEMA_V1.to_owned(),
            request_id,
            kind: "completed".to_owned(),
            workload_name,
            execution: ExecutionV1::scalar(),
            result: result.into(),
        }
    }
}

/// Stable machine category for a recoverable job error.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum JobErrorCodeV1 {
    /// The embedded workload has invalid shape or semantics.
    InvalidWorkload,
    /// The process does not implement the requested execution backend.
    UnsupportedBackend,
    /// The selected backend could not complete a valid workload.
    ExecutionFailed,
}

/// One structured workload validation issue.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ValidationIssueWireV1 {
    /// Stable issue category.
    pub code: String,
    /// Workload path containing the issue.
    pub path: String,
    /// Human-readable explanation.
    pub message: String,
}

/// Body of a recoverable correlated job error.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct JobErrorV1 {
    /// Stable error category.
    pub code: JobErrorCodeV1,
    /// Human-readable summary.
    pub message: String,
    /// Structured workload issues when semantic validation ran.
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub issues: Vec<ValidationIssueWireV1>,
}

/// Recoverable correlated error response.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ErrorResponseV1 {
    /// Response schema identifier.
    pub schema_version: String,
    /// Echoed request correlation identifier.
    pub request_id: String,
    /// Must be `error`.
    pub kind: String,
    /// Structured failure.
    pub error: JobErrorV1,
}

impl ErrorResponseV1 {
    /// Construct one correlated recoverable job error.
    #[must_use]
    pub fn new(request_id: String, error: JobErrorV1) -> Self {
        Self {
            schema_version: JOB_RESULT_SCHEMA_V1.to_owned(),
            request_id,
            kind: "error".to_owned(),
            error,
        }
    }
}

/// Acknowledgment emitted immediately before a graceful server exit.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ShutdownAckResponseV1 {
    /// Response schema identifier.
    pub schema_version: String,
    /// Echoed request correlation identifier.
    pub request_id: String,
    /// Must be `shutdown_ack`.
    pub kind: String,
}

impl ShutdownAckResponseV1 {
    /// Construct a correlated graceful-shutdown acknowledgment.
    #[must_use]
    pub fn new(request_id: String) -> Self {
        Self {
            schema_version: JOB_RESULT_SCHEMA_V1.to_owned(),
            request_id,
            kind: "shutdown_ack".to_owned(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn request_ids_use_a_small_safe_ascii_alphabet() {
        for valid in ["a", "req-0001", "session_1:sample.2", &"a".repeat(64)] {
            assert!(is_valid_request_id(valid), "{valid:?} must be valid");
        }
        for invalid in [
            "",
            "-leading",
            "contains space",
            "snowman-\u{2603}",
            &"a".repeat(65),
        ] {
            assert!(!is_valid_request_id(invalid), "{invalid:?} must be invalid");
        }
    }

    #[test]
    fn request_union_is_strict() {
        let execute = serde_json::json!({
            "schema_version": JOB_SCHEMA_V1,
            "request_id": "req-1",
            "kind": "execute",
            "job": {
                "execution": {"backend": SCALAR_BACKEND_V1},
                "workload": {"schema_version": "paraflow.workload/v1"}
            }
        });
        assert!(matches!(
            decode_request(execute.to_string().as_bytes()),
            Ok(JobRequestV1::Execute(_))
        ));

        let mut unknown = execute;
        unknown["unexpected"] = serde_json::Value::Bool(true);
        assert!(matches!(
            decode_request(unknown.to_string().as_bytes()),
            Err(RequestDecodeError::InvalidPayload(_))
        ));
    }

    #[test]
    fn workload_is_preserved_raw_before_semantic_decoding() {
        let request = br#"{
            "schema_version":"paraflow.job/v1",
            "request_id":"raw-workload",
            "kind":"execute",
            "job":{
                "execution":{"backend":"scalar"},
                "workload":{"pipeline":{"normalize":{"clip":1e400}}}
            }
        }"#;

        let JobRequestV1::Execute(decoded) =
            decode_request(request).expect("outer envelope must remain correlatable")
        else {
            panic!("expected execute request");
        };
        assert!(decoded.job.workload.get().contains("1e400"));
    }

    #[test]
    fn backend_identifier_uses_the_schema_alphabet_and_bound() {
        for valid in ["scalar", "cpp.simd", "rust_task-v2", &"a".repeat(64)] {
            assert!(
                is_valid_backend_identifier(valid),
                "{valid:?} must be valid"
            );
        }
        for invalid in ["", "1scalar", "contains space", &"a".repeat(65)] {
            assert!(
                !is_valid_backend_identifier(invalid),
                "{invalid:?} must be invalid"
            );
        }
    }

    #[test]
    fn result_transport_preserves_full_width_and_infinity() {
        let wire = ResultWireV1::from(ResultV1 {
            accepted_count: u64::MAX,
            score_sum: f64::INFINITY,
            category_histogram: vec![0, u64::MAX],
            accepted_id_sum: u64::MAX,
            accepted_id_xor: 0xfeed_face_cafe_beef,
        });
        let encoded = serde_json::to_string(&wire).expect("serialize result");

        assert!(encoded.contains("\"accepted_count\":\"0xffffffffffffffff\""));
        assert!(encoded.contains("\"bits\":\"0x7ff0000000000000\""));
        assert_eq!(wire.score_sum.to_float(), f64::INFINITY);
    }
}
