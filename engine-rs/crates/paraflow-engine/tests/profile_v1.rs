use std::io::Cursor;

use paraflow_engine::{
    profile::{self, ProfileError, execute},
    run_with_input,
};
use paraflow_protocol::profile::{
    PROFILE_ENGINE_RESULT_SCHEMA_V1, PROFILE_OBSERVER_V1, PROFILE_REQUEST_SCHEMA_V1,
    PROFILE_TOPOLOGY_V1, ProfileEngineResultV1, ProfileRequestV1,
};
use serde::Deserialize;

const PROFILE_VECTORS: &str = include_str!("../../../../contracts/conformance/profile-v1.json");

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct ProfileVectors {
    schema_version: String,
    cases: Vec<ProfileVector>,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields)]
struct ProfileVector {
    name: String,
    request: ProfileRequestV1,
    engine_result: ProfileEngineResultV1,
}

#[test]
fn profile_wire_vectors_round_trip_strictly() {
    let vectors = vectors();
    assert_eq!(vectors.schema_version, "paraflow.profile-vectors/v1");
    assert!(!vectors.cases.is_empty());

    for vector in vectors.cases {
        assert_eq!(vector.name, "edge-scalar-v1");
        assert_eq!(vector.request.schema_version, PROFILE_REQUEST_SCHEMA_V1);
        assert_eq!(
            vector.engine_result.schema_version,
            PROFILE_ENGINE_RESULT_SCHEMA_V1
        );
        assert_eq!(
            vector.request.experiment_id,
            vector.engine_result.experiment_id
        );
        assert_eq!(
            vector.request.scenario_name,
            vector.engine_result.scenario_name
        );

        let request_json = serde_json::to_string(&vector.request).expect("serialize request");
        let decoded_request: ProfileRequestV1 =
            serde_json::from_str(&request_json).expect("decode request");
        assert_eq!(decoded_request.experiment_id, vector.request.experiment_id);

        let result_json =
            serde_json::to_string(&vector.engine_result).expect("serialize engine result");
        let decoded_result: ProfileEngineResultV1 =
            serde_json::from_str(&result_json).expect("decode engine result");
        assert_eq!(decoded_result, vector.engine_result);
    }
}

#[test]
fn one_process_profiles_every_stage_and_retains_raw_samples() {
    let mut request = request();
    request.sampling.warmup_iterations = 2;
    request.sampling.sample_iterations = 4;

    let result = execute(request).expect("profile request must complete");

    assert_eq!(result.samples.len(), 4);
    assert!(!result.timing.process_start_in_samples);
    assert_eq!(result.timing.topology, PROFILE_TOPOLOGY_V1);
    assert_eq!(result.timing.observer, PROFILE_OBSERVER_V1);
    assert_eq!(result.correctness.status, "passed");
    assert_eq!(result.result.accepted_count.value(), 3);
    assert_eq!(
        result.result.score_sum.to_float().to_bits(),
        6.5_f64.to_bits()
    );

    let mut retained_total = 0_u64;
    for (ordinal, sample) in result.samples.iter().enumerate() {
        assert_eq!(
            usize::try_from(sample.ordinal).expect("u32 ordinal must fit in usize"),
            ordinal
        );
        let expected_sum = [
            sample.generation_ns.value(),
            sample.normalize_ns.value(),
            sample.score_ns.value(),
            sample.filter_ns.value(),
            sample.aggregate_ns.value(),
        ]
        .into_iter()
        .sum::<u64>();
        assert_eq!(sample.stage_sum_ns.value(), expected_sum);
        assert!(sample.profile_total_ns.value() >= expected_sum);
        retained_total = retained_total
            .checked_add(sample.profile_total_ns.value())
            .expect("test durations must not overflow");
    }
    assert!(result.timing.experiment_total_ns.value() >= retained_total);
}

#[test]
fn invalid_sampling_is_rejected_before_profile_execution() {
    let mut zero = request();
    zero.sampling.sample_iterations = 0;
    assert!(matches!(
        execute(zero),
        Err(ProfileError::InvalidRequest(_))
    ));

    let mut too_many = request();
    too_many.sampling.sample_iterations = paraflow_engine::benchmark::MAX_SAMPLE_ITERATIONS + 1;
    assert!(matches!(
        execute(too_many),
        Err(ProfileError::InvalidRequest(_))
    ));
}

#[test]
fn profile_command_reads_one_request_and_writes_one_result() {
    let mut request = request();
    request.sampling.warmup_iterations = 0;
    request.sampling.sample_iterations = 1;
    let input = serde_json::to_vec(&request).expect("serialize request");
    let mut stdin = Cursor::new(input);
    let mut stdout = Vec::new();
    let mut stderr = Vec::new();

    let exit_code = run_with_input(
        &["profile".to_owned()],
        &mut stdin,
        &mut stdout,
        &mut stderr,
    );

    assert_eq!(exit_code, 0, "stderr: {}", String::from_utf8_lossy(&stderr));
    assert!(stderr.is_empty());
    let result: ProfileEngineResultV1 =
        serde_json::from_slice(&stdout).expect("profile command result");
    assert_eq!(result.samples.len(), 1);
    assert_eq!(result.experiment_id, request.experiment_id);
}

#[test]
fn bounded_reader_rejects_an_oversized_profile_request() {
    let mut input = Cursor::new(vec![b' '; paraflow_protocol::MAX_FRAME_BYTES + 1]);
    let mut output = Vec::new();

    assert!(matches!(
        profile::run(&mut input, &mut output),
        Err(ProfileError::RequestTooLarge {
            maximum_bytes: paraflow_protocol::MAX_FRAME_BYTES
        })
    ));
    assert!(output.is_empty());
}

#[test]
fn exact_limit_profile_payload_accepts_crlf_and_rejects_one_extra_byte() {
    let mut request = request();
    request.sampling.warmup_iterations = 0;
    request.sampling.sample_iterations = 1;

    let encoded = serde_json::to_vec(&request).expect("serialize request");
    let mut exact = encoded.clone();
    exact.resize(paraflow_protocol::MAX_FRAME_BYTES, b' ');
    exact.extend_from_slice(b"\r\n");
    let mut output = Vec::new();
    profile::run(&mut Cursor::new(exact), &mut output)
        .expect("an exact-limit JSON payload plus CRLF must be accepted");
    let result: ProfileEngineResultV1 =
        serde_json::from_slice(&output).expect("decode exact-limit result");
    assert_eq!(result.samples.len(), 1);

    let mut oversized = encoded;
    oversized.resize(paraflow_protocol::MAX_FRAME_BYTES + 1, b' ');
    oversized.extend_from_slice(b"\r\n");
    assert!(matches!(
        profile::run(&mut Cursor::new(oversized), &mut Vec::new()),
        Err(ProfileError::RequestTooLarge {
            maximum_bytes: paraflow_protocol::MAX_FRAME_BYTES
        })
    ));
}

#[test]
fn unknown_profile_request_fields_are_rejected_by_strict_serde_shape() {
    let mut value: serde_json::Value = serde_json::from_str(PROFILE_VECTORS).expect("fixture JSON");
    let request = &mut value["cases"][0]["request"];
    request["unexpected"] = serde_json::Value::Bool(true);

    assert!(serde_json::from_value::<ProfileRequestV1>(request.clone()).is_err());
}

fn request() -> ProfileRequestV1 {
    vectors()
        .cases
        .into_iter()
        .next()
        .expect("profile fixture must contain a case")
        .request
}

fn vectors() -> ProfileVectors {
    serde_json::from_str(PROFILE_VECTORS).expect("profile vectors must parse")
}
