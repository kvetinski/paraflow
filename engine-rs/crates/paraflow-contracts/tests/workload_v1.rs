use paraflow_contracts::{
    BASIS_POINTS_MAX, DistributionSpec, PIPELINE_STAGES, Validate, ValidationCode, WORKLOAD_SCHEMA,
    WorkloadSpec,
};

const VALID_WORKLOAD: &str = include_str!("../../../../workloads/smoke-uniform-v1.json");

fn valid_spec() -> WorkloadSpec {
    serde_json::from_str(VALID_WORKLOAD).expect("valid fixture must parse")
}

fn has_code(spec: &WorkloadSpec, expected: ValidationCode) -> bool {
    spec.validate()
        .expect_err("mutated spec must be invalid")
        .issues()
        .iter()
        .any(|issue| issue.code == expected)
}

#[test]
fn pipeline_order_is_stable() {
    let stages = PIPELINE_STAGES.map(|stage| stage.as_str());
    assert_eq!(
        stages,
        ["generate", "normalize", "score", "filter", "aggregate"]
    );
}

#[test]
fn valid_fixture_round_trips_and_validates() {
    let spec = valid_spec();

    assert_eq!(spec.schema_version, WORKLOAD_SCHEMA);
    assert_eq!(spec.name, "smoke-uniform-v1");
    assert_eq!(spec.dataset.record_count, 1_024);
    assert!(spec.validate().is_ok());

    let encoded = serde_json::to_string(&spec).expect("spec must serialize");
    let decoded =
        serde_json::from_str::<WorkloadSpec>(&encoded).expect("serialized spec must parse");
    assert_eq!(decoded, spec);
}

#[test]
fn unknown_manifest_fields_are_rejected() {
    let source = VALID_WORKLOAD.replacen(
        "\"name\": \"smoke-uniform-v1\",",
        "\"name\": \"smoke-uniform-v1\", \"unexpected\": true,",
        1,
    );

    let error =
        serde_json::from_str::<WorkloadSpec>(&source).expect_err("unknown fields must be rejected");
    assert!(error.to_string().contains("unknown field"));
}

#[test]
fn identity_errors_are_accumulated() {
    let mut spec = valid_spec();
    spec.schema_version = "paraflow.workload/v999".to_owned();
    spec.name = " ".to_owned();

    let errors = spec.validate().expect_err("identity must be invalid");
    let codes = errors
        .issues()
        .iter()
        .map(|issue| issue.code)
        .collect::<Vec<_>>();

    assert!(codes.contains(&ValidationCode::UnsupportedSchema));
    assert!(codes.contains(&ValidationCode::BlankName));
}

#[test]
fn invalid_dataset_bounds_are_rejected() {
    let mut spec = valid_spec();
    spec.dataset.category_count = 0;
    spec.dataset.feature_min = spec.dataset.feature_max;
    spec.dataset.flag_probability_bps = BASIS_POINTS_MAX + 1;

    assert!(has_code(&spec, ValidationCode::EmptyCategories));
    assert!(has_code(&spec, ValidationCode::InvalidFeatureRange));
    assert!(has_code(&spec, ValidationCode::InvalidProbability));
}

#[test]
fn invalid_hotspot_is_rejected() {
    let mut spec = valid_spec();
    spec.dataset.distribution = DistributionSpec::Hotspot {
        category: spec.dataset.category_count,
        probability_bps: BASIS_POINTS_MAX + 1,
    };

    assert!(has_code(&spec, ValidationCode::InvalidHotspotCategory));
    assert!(has_code(&spec, ValidationCode::InvalidProbability));
}

#[test]
fn non_finite_pipeline_values_are_rejected() {
    let mut spec = valid_spec();
    spec.pipeline.normalize.offset_a = f32::NAN;
    spec.pipeline.normalize.scale_a = 0.0;
    spec.pipeline.normalize.clip = f32::INFINITY;
    spec.pipeline.score.bias = f32::NEG_INFINITY;

    assert!(has_code(&spec, ValidationCode::NonFiniteNumber));
    assert!(has_code(&spec, ValidationCode::InvalidScale));
}

#[test]
fn zero_records_are_a_valid_correctness_workload() {
    let mut spec = valid_spec();
    spec.dataset.record_count = 0;

    assert!(spec.validate().is_ok());
}
