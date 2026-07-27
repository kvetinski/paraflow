use std::io::{self, Cursor, Write};

use paraflow_engine::{run_with_input, server};
use paraflow_protocol::{MAX_FRAME_BYTES, SCALAR_BACKEND_V1};
use serde::Deserialize;
use serde_json::{Value, json};

const EXECUTION_VECTORS: &str = include_str!("../../../../contracts/conformance/execution-v1.json");

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct ExecutionVectors {
    #[serde(rename = "$schema")]
    json_schema: String,
    schema_version: String,
    protocol_schema: String,
    result_schema: String,
    cases: Vec<ExecutionCase>,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct ExecutionCase {
    name: String,
    request: Value,
    expected_response: Value,
}

#[test]
fn one_server_process_matches_all_portable_exchange_vectors() {
    let vectors = vectors();
    assert_eq!(vectors.json_schema, "../execution-vectors-v1.schema.json");
    assert_eq!(vectors.schema_version, "paraflow.execution-vectors/v1");
    assert_eq!(vectors.protocol_schema, "paraflow.job/v1");
    assert_eq!(vectors.result_schema, "paraflow.result/v1");

    let shutdown = json!({
        "schema_version": "paraflow.job/v1",
        "request_id": "conformance.shutdown",
        "kind": "shutdown"
    });
    let mut input = vectors
        .cases
        .iter()
        .map(|fixture| fixture.request.to_string())
        .chain([shutdown.to_string()])
        .collect::<Vec<_>>()
        .join("\n");
    input.push('\n');

    let mut output = Vec::new();
    server::serve(&mut Cursor::new(input), &mut output).expect("server must complete");

    let responses = output
        .split(|byte| *byte == b'\n')
        .filter(|line| !line.is_empty())
        .map(|line| serde_json::from_slice::<Value>(line).expect("response must be JSON"))
        .collect::<Vec<_>>();
    assert_eq!(responses.len(), vectors.cases.len() + 1);

    for (fixture, response) in vectors.cases.iter().zip(&responses) {
        assert_eq!(
            response, &fixture.expected_response,
            "response mismatch for {}",
            fixture.name
        );
    }
    assert_eq!(
        responses.last(),
        Some(&json!({
            "schema_version": "paraflow.job-result/v1",
            "request_id": "conformance.shutdown",
            "kind": "shutdown_ack"
        }))
    );
}

#[test]
fn recoverable_job_errors_do_not_end_the_process() {
    let fixture = vectors()
        .cases
        .into_iter()
        .find(|case| case.name == "edge-empty-v1")
        .expect("empty fixture");
    let mut unsupported = fixture.request.clone();
    unsupported["request_id"] = json!("recovery.unsupported");
    unsupported["job"]["execution"]["backend"] = json!("not-available");

    let mut invalid = fixture.request.clone();
    invalid["request_id"] = json!("recovery.invalid");
    invalid["job"]["workload"]["dataset"]["category_count"] = json!(0);

    let mut invalid_numeric = fixture.request.clone();
    invalid_numeric["request_id"] = json!("recovery.invalid-number");
    let invalid_numeric = invalid_numeric
        .to_string()
        .replacen("\"clip\":1.0", "\"clip\":1e400", 1);
    assert!(
        invalid_numeric.contains("\"clip\":1e400"),
        "test must inject a syntactically valid out-of-range JSON number"
    );

    let shutdown = json!({
        "schema_version": "paraflow.job/v1",
        "request_id": "recovery.shutdown",
        "kind": "shutdown"
    });
    let input = format!(
        "{unsupported}\n{invalid}\n{invalid_numeric}\n{}\n{shutdown}\n",
        fixture.request
    );
    let mut output = Vec::new();

    server::serve(&mut Cursor::new(input), &mut output).expect("server must recover");

    let responses = output
        .split(|byte| *byte == b'\n')
        .filter(|line| !line.is_empty())
        .map(|line| serde_json::from_slice::<Value>(line).expect("valid response"))
        .collect::<Vec<_>>();
    assert_eq!(responses.len(), 5);
    assert_eq!(responses[0]["kind"], "error");
    assert_eq!(responses[0]["error"]["code"], "unsupported_backend");
    assert!(
        responses[0]["error"]["message"]
            .as_str()
            .expect("error message")
            .chars()
            .count()
            <= 1_024
    );
    assert!(
        responses[0]["error"]["issues"][0]["message"]
            .as_str()
            .expect("issue message")
            .chars()
            .count()
            <= 1_024
    );
    assert_eq!(responses[1]["kind"], "error");
    assert_eq!(responses[1]["error"]["code"], "invalid_workload");
    assert_eq!(responses[2]["kind"], "error");
    assert_eq!(responses[2]["error"]["code"], "invalid_workload");
    assert_eq!(responses[3], fixture.expected_response);
    assert_eq!(responses[4]["kind"], "shutdown_ack");
}

#[test]
fn malformed_unknown_and_oversized_frames_are_fatal() {
    let mut output = Vec::new();
    assert!(matches!(
        server::serve(&mut Cursor::new(b"{broken}\n"), &mut output),
        Err(server::ServerError::InvalidRequest(_))
    ));
    assert!(output.is_empty());

    let duplicate = br#"{
        "schema_version":"paraflow.job/v1",
        "request_id":"fatal.duplicate",
        "request_id":"fatal.duplicate-again",
        "kind":"shutdown"
    }"#;
    assert!(matches!(
        server::serve(&mut Cursor::new(duplicate), &mut Vec::new()),
        Err(server::ServerError::InvalidRequest(_))
    ));

    let invalid_utf8 = b"{\"schema_version\":\"paraflow.job/v1\",\xff}\n";
    assert!(matches!(
        server::serve(&mut Cursor::new(invalid_utf8), &mut Vec::new()),
        Err(server::ServerError::InvalidRequest(_))
    ));

    let unknown = json!({
        "schema_version": "paraflow.job/v1",
        "request_id": "fatal.unknown",
        "kind": "shutdown",
        "unexpected": true
    });
    assert!(matches!(
        server::serve(&mut Cursor::new(format!("{unknown}\n")), &mut Vec::new()),
        Err(server::ServerError::InvalidRequest(_))
    ));

    let mut invalid_backend = vectors().cases[0].request.clone();
    invalid_backend["request_id"] = json!("fatal.backend");
    invalid_backend["job"]["execution"]["backend"] = json!("x".repeat(65));
    assert!(matches!(
        server::serve(
            &mut Cursor::new(format!("{invalid_backend}\n")),
            &mut Vec::new()
        ),
        Err(server::ServerError::InvalidRequest(_))
    ));

    let oversized = vec![b'x'; MAX_FRAME_BYTES + 1];
    assert!(matches!(
        server::serve(&mut Cursor::new(oversized), &mut Vec::new()),
        Err(server::ServerError::FrameTooLarge { .. })
    ));
}

#[test]
fn final_unterminated_request_and_crlf_are_supported() {
    let request = vectors().cases[0].request.clone();
    let mut output = Vec::new();
    server::serve(
        &mut Cursor::new(format!("{request}\r\n{request}")),
        &mut output,
    )
    .expect("both physical endings must work");

    assert_eq!(
        output
            .split(|byte| *byte == b'\n')
            .filter(|line| !line.is_empty())
            .count(),
        2
    );
}

#[test]
fn write_failure_is_fatal() {
    let request = vectors().cases[0].request.to_string();
    let error = server::serve(
        &mut Cursor::new(request),
        &mut FailingWriter { remaining: 8 },
    )
    .expect_err("writer failure must stop the server");

    assert!(matches!(
        error,
        server::ServerError::Write(_) | server::ServerError::Serialize(_)
    ));
}

#[test]
fn maximum_histogram_response_fits_the_shared_frame_bound() {
    let response = paraflow_protocol::CompletedResponseV1::scalar(
        "maximum.categories".to_owned(),
        "maximum-categories".to_owned(),
        paraflow_contracts::ResultV1 {
            accepted_count: 0,
            score_sum: 0.0,
            category_histogram: vec![0; 65_536],
            accepted_id_sum: 0,
            accepted_id_xor: 0,
        },
    );
    let encoded = serde_json::to_vec(&response).expect("serialize maximum response");

    assert!(encoded.len() > 64 * 1024);
    assert!(encoded.len() <= MAX_FRAME_BYTES);
    assert_eq!(response.execution.backend, SCALAR_BACKEND_V1);
}

#[test]
fn serve_cli_uses_injected_streams_and_rejects_arguments() {
    let shutdown = json!({
        "schema_version": "paraflow.job/v1",
        "request_id": "cli.shutdown",
        "kind": "shutdown"
    });
    let mut stdin = Cursor::new(format!("{shutdown}\n"));
    let mut stdout = Vec::new();
    let mut stderr = Vec::new();

    let exit_code = run_with_input(&["serve".to_owned()], &mut stdin, &mut stdout, &mut stderr);
    assert_eq!(exit_code, 0);
    assert!(stderr.is_empty());
    assert_eq!(
        serde_json::from_slice::<Value>(&stdout).expect("ack response")["kind"],
        "shutdown_ack"
    );

    let exit_code = run_with_input(
        &["serve".to_owned(), "unexpected".to_owned()],
        &mut Cursor::new(Vec::new()),
        &mut Vec::new(),
        &mut stderr,
    );
    assert_eq!(exit_code, 2);
}

struct FailingWriter {
    remaining: usize,
}

impl Write for FailingWriter {
    fn write(&mut self, buffer: &[u8]) -> io::Result<usize> {
        if self.remaining == 0 {
            return Err(io::Error::new(
                io::ErrorKind::BrokenPipe,
                "injected failure",
            ));
        }
        let written = self.remaining.min(buffer.len());
        self.remaining -= written;
        Ok(written)
    }

    fn flush(&mut self) -> io::Result<()> {
        Ok(())
    }
}

fn vectors() -> ExecutionVectors {
    serde_json::from_str(EXECUTION_VECTORS).expect("execution vectors must parse")
}
