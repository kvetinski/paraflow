//! Command-line entry points for the ParaFlow engine.
//!
//! The Day 1 executable can report its version and contract. Workload
//! execution is intentionally unavailable until the scalar oracle exists.

#![forbid(unsafe_code)]

use std::{error::Error, fmt, fs, io::Write, path::Path};

use paraflow_contracts::{
    PIPELINE_STAGES, Validate, ValidationErrors, WORKLOAD_SCHEMA, WorkloadSpec,
};

/// A workload manifest could not be parsed or validated.
#[derive(Debug)]
pub enum ManifestError {
    /// JSON syntax or shape is invalid.
    Json(serde_json::Error),
    /// JSON is well-formed but violates semantic invariants.
    Validation(ValidationErrors),
}

impl fmt::Display for ManifestError {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Json(error) => write!(formatter, "JSON error: {error}"),
            Self::Validation(errors) => write!(formatter, "{errors}"),
        }
    }
}

impl Error for ManifestError {
    fn source(&self) -> Option<&(dyn Error + 'static)> {
        match self {
            Self::Json(error) => Some(error),
            Self::Validation(errors) => Some(errors),
        }
    }
}

/// Parse and semantically validate one workload manifest.
pub fn parse_manifest(source: &str) -> Result<WorkloadSpec, ManifestError> {
    let spec = serde_json::from_str::<WorkloadSpec>(source).map_err(ManifestError::Json)?;
    spec.validate().map_err(ManifestError::Validation)?;
    Ok(spec)
}

/// Run the engine command and return a process-compatible exit code.
pub fn run(args: &[String], stdout: &mut impl Write, stderr: &mut impl Write) -> i32 {
    match args.first().map(String::as_str) {
        None | Some("help") | Some("--help") | Some("-h") => write_help(stdout),
        Some("version") | Some("--version") | Some("-V") => write_output(
            stdout,
            &format!("paraflow-engine {}", env!("CARGO_PKG_VERSION")),
        ),
        Some("contract") if args.len() == 1 => write_contract(stdout),
        Some("contract") => write_usage_error(stderr, "contract accepts no arguments"),
        Some("validate") if args.len() == 2 => validate_file(&args[1], stdout, stderr),
        Some("validate") => {
            write_usage_error(stderr, "validate requires exactly one manifest path")
        }
        Some(command) => write_usage_error(stderr, &format!("unknown command: {command}")),
    }
}

fn write_help(output: &mut impl Write) -> i32 {
    let help = "\
ParaFlow execution engine

Usage:
  paraflow-engine contract
  paraflow-engine validate <workload.json>
  paraflow-engine version
  paraflow-engine help

Execution is introduced after the scalar correctness oracle is implemented.";
    write_output(output, help)
}

fn write_contract(output: &mut impl Write) -> i32 {
    let stages = PIPELINE_STAGES
        .iter()
        .map(ToString::to_string)
        .collect::<Vec<_>>()
        .join(" -> ");
    let message = format!("schema: {WORKLOAD_SCHEMA}\npipeline: {stages}");
    write_output(output, &message)
}

fn write_usage_error(error: &mut impl Write, message: &str) -> i32 {
    write_error(
        error,
        2,
        &format!("{message}\nrun 'paraflow-engine help' for usage"),
    )
}

fn write_output(output: &mut impl Write, message: &str) -> i32 {
    if writeln!(output, "{message}").is_err() {
        return 1;
    }
    0
}

fn validate_file(path: &str, output: &mut impl Write, error: &mut impl Write) -> i32 {
    let source = match fs::read_to_string(Path::new(path)) {
        Ok(source) => source,
        Err(read_error) => {
            return write_error(
                error,
                3,
                &format!("cannot read workload manifest {path:?}: {read_error}"),
            );
        }
    };

    match parse_manifest(&source) {
        Ok(spec) => write_output(
            output,
            &format!("valid workload: {} ({})", spec.name, spec.schema_version),
        ),
        Err(manifest_error) => write_error(
            error,
            4,
            &format!("invalid workload manifest {path:?}: {manifest_error}"),
        ),
    }
}

fn write_error(error: &mut impl Write, exit_code: i32, message: &str) -> i32 {
    if writeln!(error, "error: {message}").is_err() {
        return 1;
    }
    exit_code
}
