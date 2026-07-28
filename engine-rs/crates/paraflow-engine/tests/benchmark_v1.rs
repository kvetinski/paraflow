use std::io::Cursor;

use paraflow_engine::{
    benchmark::{self, execute, BenchmarkError, MAX_SAMPLE_ITERATIONS},
    run_with_input,
};
use paraflow_protocol::benchmark::{
    BenchmarkEngineResultV1, BenchmarkRequestV1, BENCHMARK_ENGINE_RESULT_SCHEMA_V1,
    BENCHMARK_REQUEST_SCHEMA_V1,
};
use serde::Deserialize;

const BENCHMARK_VECTORS: &str =
    include_str!("../../../../contracts/conformance/benchmark-v1.json");

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct BenchmarkVectors {
    schema_version: String,
    cases: Vec<BenchmarkVector>,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct BenchmarkVector {
    name: String,
    request: BenchmarkRequestV1,
    engine_result: BenchmarkEngineResultV1,
}

#[test]
fn benchmark_wire_vectors_round_trip_strictly() {
    let vectors = vectors();
    assert_eq!(vectors.schema_version, "paraflow.benchmark-vectors/v1");
    assert!(!vectors.cases.is_empty());

    for vector in vectors.cases {
        assert_eq!(vector.request.schema_version, BENCHMARK_REQUEST_SCHEMA_V1);
        assert_eq!(
            vector.engine_result.schema_version,
            BENCHMARK_ENGINE_RESULT_SCHEMA_V1
        );
        assert_eq!(vector.request.experiment_id, vector.engine_result.experiment_id);
        assert_eq!(vector.request.scenario_name, vector.engine_result.scenario_name);
        assert_eq!(vector.name, "edge-scalar-v1");

        let request_json = serde_json::to_string(&vector.request).expect("serialize request");
        let decoded_request: BenchmarkRequestV1 =
            serde_json::from_str(&request_json).expect("decode request");
        assert_eq!(decoded_request.experiment_id, vector.request.experiment_id);

        let result_json =
            serde_json::to_string(&vector.engine_result).expect("serialize engine result");
        let decoded_result: BenchmarkEngineResultV1 =
            serde_json::from_str(&result_json).expect("decode engine result");
        assert_eq!(decoded_result, vector.engine_result);
    }
}

#[test]
fn one_process_runs_warmups_and_retains_every_raw_sample() {
    let mut request = request();
    request.sampling.warmup_iterations = 2;
    request.sampling.sample_iterations = 4;

    let result = execute(request).expect("benchmark request must complete");

    assert_eq!(result.samples.len(), 4);
    assert!(!result.timing.process_start_in_samples);
    assert_eq!(result.correctness.status, "passed");
    assert_eq!(result.correctness.oracle, "rust-scalar-v1");
    assert_eq!(result.correctness.comparison, "exact");
    assert_eq!(result.result.accepted_count.value(), 3);
    assert_eq!(result.result.score_sum.to_float().to_bits(), 6.5_f64.to_bits());

    let mut retained_total = 0_u64;
    for (ordinal, sample) in result.samples.iter().enumerate() {
        assert_eq!(
            usize::try_from(sample.ordinal).expect("u32 ordinal must fit in usize"),
            ordinal
        );
        let generation = sample.generation_ns.value();
        let pipeline = sample.pipeline_ns.value();
        let engine_total = sample.engine_total_ns.value();
        assert!(engine_total >= generation);
        assert!(engine_total - generation >= pipeline);
        retained_total = retained_total
            .checked_add(engine_total)
            .expect("test durations must not overflow");
    }
    assert!(result.timing.experiment_total_ns.value() >= retained_total);
}

#[test]
fn invalid_sampling_is_rejected_before_workload_execution() {
    let mut zero = request();
    zero.sampling.sample_iterations = 0;
    assert!(matches!(execute(zero), Err(BenchmarkError::InvalidRequest(_))));

    let mut too_many = request();
    too_many.sampling.sample_iterations = MAX_SAMPLE_ITERATIONS + 1;
    assert!(matches!(
        execute(too_many),
        Err(BenchmarkError::InvalidRequest(_))
    ));
}

#[test]
fn benchmark_command_reads_one_request_and_writes_one_result() {
    let mut request = request();
    request.sampling.warmup_iterations = 0;
    request.sampling.sample_iterations = 1;
    let input = serde_json::to_vec(&request).expect("serialize request");
    let mut stdin = Cursor::new(input);
    let mut stdout = Vec::new();
    let mut stderr = Vec::new();

    let exit_code = run_with_input(
        &["benchmark".to_owned()],
        &mut stdin,
        &mut stdout,
        &mut stderr,
    );

    assert_eq!(exit_code, 0, "stderr: {}", String::from_utf8_lossy(&stderr));
    assert!(stderr.is_empty());
    let result: BenchmarkEngineResultV1 =
        serde_json::from_slice(&stdout).expect("benchmark command result");
    assert_eq!(result.samples.len(), 1);
    assert_eq!(result.experiment_id, request.experiment_id);
}

#[test]
fn bounded_reader_rejects_an_oversized_request() {
    let mut input = Cursor::new(vec![b' '; paraflow_protocol::MAX_FRAME_BYTES + 1]);
    let mut output = Vec::new();

    assert!(matches!(
        benchmark::run(&mut input, &mut output),
        Err(BenchmarkError::RequestTooLarge {
            maximum_bytes: paraflow_protocol::MAX_FRAME_BYTES
        })
    ));
    assert!(output.is_empty());
}

#[test]
fn exact_limit_request_payload_accepts_crlf_and_one_extra_payload_byte_is_rejected() {
    let mut request = request();
    request.sampling.warmup_iterations = 0;
    request.sampling.sample_iterations = 1;

    let encoded = serde_json::to_vec(&request).expect("serialize request");
    assert!(encoded.len() < paraflow_protocol::MAX_FRAME_BYTES);

    let mut exact = encoded.clone();
    exact.resize(paraflow_protocol::MAX_FRAME_BYTES, b' ');
    exact.extend_from_slice(b"\r\n");
    let mut exact_output = Vec::new();
    benchmark::run(&mut Cursor::new(exact), &mut exact_output)
        .expect("an exact-limit JSON payload plus CRLF must be accepted");
    let result: BenchmarkEngineResultV1 =
        serde_json::from_slice(&exact_output).expect("decode exact-limit result");
    assert_eq!(result.samples.len(), 1);

    let mut oversized = encoded;
    oversized.resize(paraflow_protocol::MAX_FRAME_BYTES + 1, b' ');
    oversized.extend_from_slice(b"\r\n");
    assert!(matches!(
        benchmark::run(&mut Cursor::new(oversized), &mut Vec::new()),
        Err(BenchmarkError::RequestTooLarge {
            maximum_bytes: paraflow_protocol::MAX_FRAME_BYTES
        })
    ));
}

#[test]
fn unknown_request_fields_are_rejected_by_strict_serde_shape() {
    let mut value: serde_json::Value =
        serde_json::from_str(BENCHMARK_VECTORS).expect("fixture JSON");
    let request = &mut value["cases"][0]["request"];
    request["unexpected"] = serde_json::Value::Bool(true);

    assert!(serde_json::from_value::<BenchmarkRequestV1>(request.clone()).is_err());
}

fn request() -> BenchmarkRequestV1 {
    vectors()
        .cases
        .into_iter()
        .next()
        .expect("benchmark fixture must contain a case")
        .request
}

fn vectors() -> BenchmarkVectors {
    serde_json::from_str(BENCHMARK_VECTORS).expect("benchmark vectors must parse")
}
