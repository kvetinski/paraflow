//! Long-lived, sequential protocol server for the Day 4 worker boundary.

use std::{
    error::Error,
    fmt,
    io::{self, BufRead, Write},
};

use paraflow_contracts::{Validate, ValidationErrors, WorkloadSpec};

use paraflow_protocol::{
    CompletedResponseV1, ErrorResponseV1, ExecuteRequestV1, JobErrorCodeV1, JobErrorV1,
    JobRequestV1, MAX_FRAME_BYTES, RequestDecodeError, SCALAR_BACKEND_V1, ShutdownAckResponseV1,
    ValidationIssueWireV1, decode_request,
};

use serde::Serialize;

use crate::scalar::ScalarOracle;

const MAX_ERROR_MESSAGE_CHARS: usize = 1_024;

/// The worker stream could not continue safely.
#[derive(Debug)]
pub enum ServerError {
    /// Reading the next frame failed.
    Read(io::Error),
    /// A physical frame exceeded the shared protocol bound.
    FrameTooLarge {
        /// Maximum payload bytes accepted by protocol v1.
        maximum_bytes: usize,
    },
    /// A response violated the shared physical frame limit.
    ResponseTooLarge {
        /// Encoded JSON payload size.
        actual_bytes: usize,
        /// Maximum payload bytes accepted by protocol v1.
        maximum_bytes: usize,
    },
    /// A frame violated the strict request envelope.
    InvalidRequest(RequestDecodeError),
    /// A response could not be encoded.
    Serialize(serde_json::Error),
    /// Writing or flushing a response failed.
    Write(io::Error),
}

impl fmt::Display for ServerError {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Read(error) => write!(formatter, "read worker input: {error}"),
            Self::FrameTooLarge { maximum_bytes } => {
                write!(
                    formatter,
                    "protocol frame exceeds the {maximum_bytes}-byte limit"
                )
            }
            Self::ResponseTooLarge {
                actual_bytes,
                maximum_bytes,
            } => write!(
                formatter,
                "protocol response is {actual_bytes} bytes; maximum is {maximum_bytes}"
            ),
            Self::InvalidRequest(error) => write!(formatter, "invalid protocol request: {error}"),
            Self::Serialize(error) => write!(formatter, "serialize protocol response: {error}"),
            Self::Write(error) => write!(formatter, "write protocol response: {error}"),
        }
    }
}

impl Error for ServerError {
    fn source(&self) -> Option<&(dyn Error + 'static)> {
        match self {
            Self::Read(error) | Self::Write(error) => Some(error),
            Self::InvalidRequest(error) => Some(error),
            Self::Serialize(error) => Some(error),
            Self::FrameTooLarge { .. } | Self::ResponseTooLarge { .. } => None,
        }
    }
}

/// Serve protocol requests sequentially until clean EOF or acknowledged shutdown.
///
/// Protocol violations are fatal because the peer can no longer safely
/// correlate the stream. Valid jobs that select an unsupported backend or
/// contain an invalid workload receive a correlated error and leave the
/// process reusable.
pub fn serve(input: &mut impl BufRead, output: &mut impl Write) -> Result<(), ServerError> {
    let mut frame = Vec::new();

    while read_frame(input, &mut frame)? {
        let request = decode_request(&frame).map_err(ServerError::InvalidRequest)?;

        match request {
            JobRequestV1::Execute(request) => execute(request, output)?,
            JobRequestV1::Shutdown(request) => {
                write_response(output, &ShutdownAckResponseV1::new(request.request_id))?;
                return Ok(());
            }
        }
    }

    Ok(())
}

fn execute(request: ExecuteRequestV1, output: &mut impl Write) -> Result<(), ServerError> {
    let request_id = request.request_id;

    if request.job.execution.backend != SCALAR_BACKEND_V1 {
        let backend = request.job.execution.backend;
        let summary = bounded_message(format!(
            "backend {backend:?} is not supported by this worker"
        ));
        let issue_message =
            bounded_message(format!("expected {SCALAR_BACKEND_V1:?}, got {backend:?}"));
        return write_response(
            output,
            &ErrorResponseV1::new(
                request_id,
                JobErrorV1 {
                    code: JobErrorCodeV1::UnsupportedBackend,
                    message: summary,
                    issues: vec![ValidationIssueWireV1 {
                        code: "unsupported_backend".to_owned(),
                        path: "job.execution.backend".to_owned(),
                        message: issue_message,
                    }],
                },
            ),
        );
    }

    let workload = match serde_json::from_str::<WorkloadSpec>(request.job.workload.get()) {
        Ok(workload) => workload,
        Err(error) => {
            return write_response(
                output,
                &ErrorResponseV1::new(
                    request_id,
                    JobErrorV1 {
                        code: JobErrorCodeV1::InvalidWorkload,
                        message: bounded_message(format!(
                            "workload JSON shape is invalid: {error}"
                        )),
                        issues: Vec::new(),
                    },
                ),
            );
        }
    };

    if let Err(errors) = workload.validate() {
        return write_response(
            output,
            &ErrorResponseV1::new(request_id, invalid_workload_error(&errors)),
        );
    }

    let result = match ScalarOracle::try_new(&workload).and_then(|oracle| oracle.run_result()) {
        Ok(result) => result,
        Err(error) => {
            return write_response(
                output,
                &ErrorResponseV1::new(
                    request_id,
                    JobErrorV1 {
                        code: JobErrorCodeV1::ExecutionFailed,
                        message: bounded_message(format!("scalar execution failed: {error}")),
                        issues: Vec::new(),
                    },
                ),
            );
        }
    };

    write_response(
        output,
        &CompletedResponseV1::scalar(request_id, workload.name, result),
    )
}

fn invalid_workload_error(errors: &ValidationErrors) -> JobErrorV1 {
    JobErrorV1 {
        code: JobErrorCodeV1::InvalidWorkload,
        message: format!("workload has {} semantic validation issue(s)", errors.len()),
        issues: errors
            .issues()
            .iter()
            .map(|issue| ValidationIssueWireV1 {
                code: issue.code.to_string(),
                path: issue.path.to_owned(),
                message: bounded_message(&issue.message),
            })
            .collect(),
    }
}

fn bounded_message(message: impl AsRef<str>) -> String {
    message
        .as_ref()
        .chars()
        .take(MAX_ERROR_MESSAGE_CHARS)
        .collect()
}

fn write_response(output: &mut impl Write, response: &impl Serialize) -> Result<(), ServerError> {
    let payload = serde_json::to_vec(response).map_err(ServerError::Serialize)?;
    if payload.len() > MAX_FRAME_BYTES {
        return Err(ServerError::ResponseTooLarge {
            actual_bytes: payload.len(),
            maximum_bytes: MAX_FRAME_BYTES,
        });
    }
    output.write_all(&payload).map_err(ServerError::Write)?;
    output.write_all(b"\n").map_err(ServerError::Write)?;
    output.flush().map_err(ServerError::Write)
}

fn read_frame(input: &mut impl BufRead, frame: &mut Vec<u8>) -> Result<bool, ServerError> {
    frame.clear();

    loop {
        let available = input.fill_buf().map_err(ServerError::Read)?;
        if available.is_empty() {
            if frame.len() > MAX_FRAME_BYTES {
                return Err(ServerError::FrameTooLarge {
                    maximum_bytes: MAX_FRAME_BYTES,
                });
            }
            return Ok(!frame.is_empty());
        }

        let newline = available.iter().position(|byte| *byte == b'\n');
        let payload_length = newline.unwrap_or(available.len());
        let next_length = frame.len().saturating_add(payload_length);
        let possible_crlf_terminator = next_length == MAX_FRAME_BYTES + 1
            && if payload_length == 0 {
                frame.last() == Some(&b'\r')
            } else {
                available[payload_length - 1] == b'\r'
            };

        if next_length > MAX_FRAME_BYTES && !possible_crlf_terminator {
            return Err(ServerError::FrameTooLarge {
                maximum_bytes: MAX_FRAME_BYTES,
            });
        }

        frame.extend_from_slice(&available[..payload_length]);
        let consumed = payload_length + usize::from(newline.is_some());
        input.consume(consumed);

        if newline.is_some() {
            if frame.last() == Some(&b'\r') {
                frame.pop();
            }
            if frame.len() > MAX_FRAME_BYTES {
                return Err(ServerError::FrameTooLarge {
                    maximum_bytes: MAX_FRAME_BYTES,
                });
            }
            return Ok(true);
        }
    }
}

#[cfg(test)]
mod tests {
    use std::io::Cursor;

    use serde::Serialize;

    use super::*;

    #[test]
    fn frame_reader_accepts_lf_crlf_and_final_unterminated_data() {
        let mut input = Cursor::new(b"one\ntwo\r\nthree".to_vec());
        let mut frame = Vec::new();

        assert!(read_frame(&mut input, &mut frame).expect("first frame"));
        assert_eq!(frame, b"one");
        assert!(read_frame(&mut input, &mut frame).expect("second frame"));
        assert_eq!(frame, b"two");
        assert!(read_frame(&mut input, &mut frame).expect("third frame"));
        assert_eq!(frame, b"three");
        assert!(!read_frame(&mut input, &mut frame).expect("clean EOF"));
    }

    #[test]
    fn frame_reader_rejects_oversized_payload_before_unbounded_growth() {
        let oversized = vec![b'x'; MAX_FRAME_BYTES + 1];
        let mut input = Cursor::new(oversized);
        let mut frame = Vec::new();

        assert!(matches!(
            read_frame(&mut input, &mut frame),
            Err(ServerError::FrameTooLarge {
                maximum_bytes: MAX_FRAME_BYTES
            })
        ));
        assert!(frame.len() <= MAX_FRAME_BYTES + 1);
    }

    #[test]
    fn exact_limit_excludes_lf_and_crlf_terminators() {
        for terminator in [b"\n".as_slice(), b"\r\n".as_slice()] {
            let mut input = vec![b'x'; MAX_FRAME_BYTES];
            input.extend_from_slice(terminator);
            let mut frame = Vec::new();

            assert!(read_frame(&mut Cursor::new(input), &mut frame).expect("exact frame"));
            assert_eq!(frame.len(), MAX_FRAME_BYTES);
        }
    }

    #[test]
    fn correlated_error_messages_are_bounded_by_unicode_characters() {
        let message = "🦀".repeat(MAX_ERROR_MESSAGE_CHARS + 1);
        let bounded = bounded_message(&message);

        assert_eq!(bounded.chars().count(), MAX_ERROR_MESSAGE_CHARS);
        assert!(bounded.is_char_boundary(bounded.len()));
    }

    #[test]
    fn oversized_responses_are_rejected_before_any_bytes_are_written() {
        #[derive(Serialize)]
        struct OversizedResponse {
            payload: String,
        }

        let response = OversizedResponse {
            payload: "x".repeat(MAX_FRAME_BYTES),
        };
        let mut output = Vec::new();
        assert!(matches!(
            write_response(&mut output, &response),
            Err(ServerError::ResponseTooLarge {
                maximum_bytes: MAX_FRAME_BYTES,
                ..
            })
        ));
        assert!(output.is_empty());
    }
}
