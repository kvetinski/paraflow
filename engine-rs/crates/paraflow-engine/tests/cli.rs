use paraflow_engine::{parse_manifest, run};

const VALID_WORKLOAD: &str = include_str!("../../../../workloads/smoke-uniform-v1.json");

#[test]
fn contract_command_reports_stable_pipeline() {
    let mut stdout = Vec::new();
    let mut stderr = Vec::new();

    let exit_code = run(&["contract".to_owned()], &mut stdout, &mut stderr);

    assert_eq!(exit_code, 0);
    assert!(stderr.is_empty());
    let output = String::from_utf8(stdout).expect("output must be UTF-8");
    assert!(output.contains("paraflow.workload/v1"));
    assert!(output.contains("generate -> normalize -> score -> filter -> aggregate"));
}

#[test]
fn valid_manifest_is_accepted() {
    let spec = parse_manifest(VALID_WORKLOAD).expect("fixture must be valid");
    assert_eq!(spec.name, "smoke-uniform-v1");
}

#[test]
fn semantically_invalid_manifest_is_rejected() {
    let invalid = VALID_WORKLOAD.replace(
        "\"schema_version\": \"paraflow.workload/v1\"",
        "\"schema_version\": \"paraflow.workload/v999\"",
    );

    let error = parse_manifest(&invalid).expect_err("unsupported schema must fail");
    assert!(error.to_string().contains("unsupported_schema"));
}

#[test]
fn missing_validate_path_is_a_usage_error() {
    let mut stdout = Vec::new();
    let mut stderr = Vec::new();

    let exit_code = run(&["validate".to_owned()], &mut stdout, &mut stderr);

    assert_eq!(exit_code, 2);
    assert!(stdout.is_empty());
    assert!(
        String::from_utf8(stderr)
            .expect("error must be UTF-8")
            .contains("requires exactly one manifest path")
    );
}
