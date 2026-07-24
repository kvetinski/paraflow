use std::{
    fs,
    path::{Path, PathBuf},
    process,
    sync::atomic::{AtomicU64, Ordering},
};

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

#[test]
fn validate_command_accepts_a_valid_file() {
    let path = write_temp_manifest("valid", VALID_WORKLOAD);
    let mut stdout = Vec::new();
    let mut stderr = Vec::new();

    let exit_code = run(
        &["validate".to_owned(), path.display().to_string()],
        &mut stdout,
        &mut stderr,
    );

    remove_temp_manifest(&path);
    assert_eq!(exit_code, 0);
    assert!(stderr.is_empty());
    assert!(
        String::from_utf8(stdout)
            .expect("output must be UTF-8")
            .contains("valid workload: smoke-uniform-v1")
    );
}

#[test]
fn validate_command_distinguishes_missing_and_invalid_files() {
    let missing = unique_temp_path("missing");
    let mut stdout = Vec::new();
    let mut stderr = Vec::new();

    let missing_exit = run(
        &["validate".to_owned(), missing.display().to_string()],
        &mut stdout,
        &mut stderr,
    );
    assert_eq!(missing_exit, 3);
    assert!(stdout.is_empty());
    assert!(String::from_utf8_lossy(&stderr).contains("cannot read workload manifest"));

    let invalid = write_temp_manifest("malformed", "{ not-json }");
    stdout.clear();
    stderr.clear();
    let invalid_exit = run(
        &["validate".to_owned(), invalid.display().to_string()],
        &mut stdout,
        &mut stderr,
    );
    remove_temp_manifest(&invalid);

    assert_eq!(invalid_exit, 4);
    assert!(stdout.is_empty());
    assert!(String::from_utf8_lossy(&stderr).contains("JSON error"));
}

#[test]
fn commands_reject_trailing_arguments() {
    for command in ["help", "version", "contract"] {
        let mut stdout = Vec::new();
        let mut stderr = Vec::new();
        let exit_code = run(
            &[command.to_owned(), "unexpected".to_owned()],
            &mut stdout,
            &mut stderr,
        );

        assert_eq!(exit_code, 2, "{command} must reject trailing arguments");
        assert!(stdout.is_empty());
        assert!(String::from_utf8_lossy(&stderr).contains("accepts no arguments"));
    }
}

#[cfg(unix)]
#[test]
fn validate_command_accepts_a_non_utf8_manifest_path() {
    use std::{ffi::OsString, os::unix::ffi::OsStringExt};
    let counter = TEMP_COUNTER.fetch_add(1, Ordering::Relaxed);
    let file_name = OsString::from_vec(
        format!("paraflow-non-utf8-{}-{counter}-", process::id())
            .bytes()
            .chain([0x80])
            .chain(".json".bytes())
            .collect(),
    );
    let path = std::env::temp_dir().join(file_name);
    fs::write(&path, VALID_WORKLOAD).expect("temporary manifest must be writable");
    let args = [OsString::from("validate"), path.clone().into_os_string()];
    let mut stdout = Vec::new();
    let mut stderr = Vec::new();

    let exit_code = run(&args, &mut stdout, &mut stderr);

    remove_temp_manifest(&path);
    assert_eq!(exit_code, 0);
    assert!(stderr.is_empty());
}

static TEMP_COUNTER: AtomicU64 = AtomicU64::new(0);

fn unique_temp_path(label: &str) -> PathBuf {
    let counter = TEMP_COUNTER.fetch_add(1, Ordering::Relaxed);
    std::env::temp_dir().join(format!("paraflow-{label}-{}-{counter}.json", process::id()))
}

fn write_temp_manifest(label: &str, source: &str) -> PathBuf {
    let path = unique_temp_path(label);
    fs::write(&path, source).expect("temporary manifest must be writable");
    path
}

fn remove_temp_manifest(path: &Path) {
    fs::remove_file(path).expect("temporary manifest must be removable");
}
