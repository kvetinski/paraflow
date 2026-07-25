use std::{fs, path::Path};

use paraflow_contracts::{
    BASIS_POINTS_MAX, DistributionSpec, MAX_CATEGORIES, MAX_SAFE_JSON_INTEGER, PIPELINE_STAGES,
    Validate, ValidationCode, WORKLOAD_SCHEMA, WorkloadSpec,
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
fn manifest_integers_must_be_lossless_in_every_json_consumer() {
    let mut spec = valid_spec();
    spec.dataset.record_count = MAX_SAFE_JSON_INTEGER + 1;
    spec.dataset.seed = MAX_SAFE_JSON_INTEGER + 1;

    let errors = spec.validate().expect_err("unsafe JSON integers must fail");
    let unsafe_paths = errors
        .issues()
        .iter()
        .filter(|issue| issue.code == ValidationCode::UnsafeJsonInteger)
        .map(|issue| issue.path)
        .collect::<Vec<_>>();

    assert_eq!(
        unsafe_paths,
        ["dataset.record_count", "dataset.seed"],
        "both independently rounded fields must be reported"
    );
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

#[test]
fn every_validation_code_has_a_regression_case() {
    let mut cases = Vec::new();

    let mut unsupported_schema = valid_spec();
    unsupported_schema.schema_version = "paraflow.workload/v999".to_owned();
    cases.push((unsupported_schema, ValidationCode::UnsupportedSchema));

    let mut blank_name = valid_spec();
    blank_name.name = " \t".to_owned();
    cases.push((blank_name, ValidationCode::BlankName));

    let mut long_name = valid_spec();
    long_name.name = "x".repeat(121);
    cases.push((long_name, ValidationCode::NameTooLong));

    let mut unsafe_json_integer = valid_spec();
    unsafe_json_integer.dataset.seed = MAX_SAFE_JSON_INTEGER + 1;
    cases.push((unsafe_json_integer, ValidationCode::UnsafeJsonInteger));

    let mut empty_categories = valid_spec();
    empty_categories.dataset.category_count = 0;
    cases.push((empty_categories, ValidationCode::EmptyCategories));

    let mut too_many_categories = valid_spec();
    too_many_categories.dataset.category_count = MAX_CATEGORIES + 1;
    cases.push((too_many_categories, ValidationCode::TooManyCategories));

    let mut invalid_range = valid_spec();
    invalid_range.dataset.feature_max = invalid_range.dataset.feature_min;
    cases.push((invalid_range, ValidationCode::InvalidFeatureRange));

    let mut invalid_probability = valid_spec();
    invalid_probability.dataset.flag_probability_bps = BASIS_POINTS_MAX + 1;
    cases.push((invalid_probability, ValidationCode::InvalidProbability));

    let mut invalid_hotspot = valid_spec();
    invalid_hotspot.dataset.distribution = DistributionSpec::Hotspot {
        category: invalid_hotspot.dataset.category_count,
        probability_bps: 5_000,
    };
    cases.push((invalid_hotspot, ValidationCode::InvalidHotspotCategory));

    let mut non_finite = valid_spec();
    non_finite.pipeline.score.bias = f32::NAN;
    cases.push((non_finite, ValidationCode::NonFiniteNumber));

    let mut invalid_scale = valid_spec();
    invalid_scale.pipeline.normalize.scale_b = -1.0;
    cases.push((invalid_scale, ValidationCode::InvalidScale));

    let mut invalid_clip = valid_spec();
    invalid_clip.pipeline.normalize.clip = 0.0;
    cases.push((invalid_clip, ValidationCode::InvalidClip));

    for (spec, expected) in cases {
        assert!(
            has_code(&spec, expected),
            "missing regression coverage for {expected}"
        );
    }
}

#[test]
fn all_checked_in_workloads_parse_and_validate() {
    let workloads = Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("../../..")
        .join("workloads");
    let mut paths = fs::read_dir(&workloads)
        .expect("workloads directory must exist")
        .map(|entry| {
            entry
                .expect("workload directory entry must be readable")
                .path()
        })
        .filter(|path| {
            path.extension()
                .is_some_and(|extension| extension == "json")
        })
        .collect::<Vec<_>>();
    paths.sort();

    assert!(
        !paths.is_empty(),
        "at least one workload fixture is required"
    );

    for path in paths {
        let source = fs::read_to_string(&path).expect("workload fixture must be readable");
        let spec = serde_json::from_str::<WorkloadSpec>(&source)
            .unwrap_or_else(|error| panic!("{} must parse: {error}", path.display()));
        spec.validate()
            .unwrap_or_else(|error| panic!("{} must validate: {error}", path.display()));
    }
}
