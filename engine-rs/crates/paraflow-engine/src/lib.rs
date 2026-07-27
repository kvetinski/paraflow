//! Deterministic generation, scalar correctness, and command-line entry points
//! for the ParaFlow engine.

#![forbid(unsafe_code)]

pub mod generation;
pub mod scalar;
pub mod server;

use std::{
    error::Error,
    ffi::OsStr,
    fmt, fs,
    io::{self, BufRead, Write},
    path::Path,
};

use paraflow_contracts::{
    PIPELINE_STAGES, Validate, ValidationErrors, WORKLOAD_SCHEMA, WorkloadSpec,
};

use crate::scalar::ScalarOracle;

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
pub fn run<S: AsRef<OsStr>>(args: &[S], stdout: &mut impl Write, stderr: &mut impl Write) -> i32 {
    run_with_input(args, &mut io::empty(), stdout, stderr)
}

/// Run the engine command with an injected input stream.
///
/// Regular one-shot commands ignore `stdin`; the long-lived `serve` command
/// consumes versioned protocol frames from it.
pub fn run_with_input<S: AsRef<OsStr>>(
    args: &[S],
    stdin: &mut impl BufRead,
    stdout: &mut impl Write,
    stderr: &mut impl Write,
) -> i32 {
    let Some(command) = args.first() else {
        return write_help(stdout);
    };
    let Some(command) = command.as_ref().to_str() else {
        return write_usage_error(stderr, "command must be valid UTF-8");
    };

    match command {
        "help" | "--help" | "-h" if args.len() == 1 => write_help(stdout),
        "help" | "--help" | "-h" => write_usage_error(stderr, "help accepts no arguments"),
        "version" | "--version" | "-V" if args.len() == 1 => write_output(
            stdout,
            &format!("paraflow-engine {}", env!("CARGO_PKG_VERSION")),
        ),
        "version" | "--version" | "-V" => write_usage_error(stderr, "version accepts no arguments"),
        "contract" if args.len() == 1 => write_contract(stdout),
        "contract" => write_usage_error(stderr, "contract accepts no arguments"),
        "validate" if args.len() == 2 => validate_file(Path::new(args[1].as_ref()), stdout, stderr),
        "validate" => write_usage_error(stderr, "validate requires exactly one manifest path"),
        "oracle" if args.len() == 2 => oracle_file(Path::new(args[1].as_ref()), stdout, stderr),
        "oracle" => write_usage_error(stderr, "oracle requires exactly one manifest path"),
        "serve" if args.len() == 1 => match server::serve(stdin, stdout) {
            Ok(()) => 0,
            Err(server_error) => write_error(stderr, 1, &format!("worker failed: {server_error}")),
        },
        "serve" => write_usage_error(stderr, "serve accepts no arguments"),
        command => write_usage_error(stderr, &format!("unknown command: {command}")),
    }
}

fn write_help(output: &mut impl Write) -> i32 {
    let help = "\
ParaFlow execution engine

Usage:
  paraflow-engine contract
  paraflow-engine validate <workload.json>
  paraflow-engine oracle <workload.json>
  paraflow-engine serve
  paraflow-engine version
  paraflow-engine help

The serve command is a long-lived NDJSON worker for labctl.";
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

fn validate_file(path: &Path, output: &mut impl Write, error: &mut impl Write) -> i32 {
    let spec = match load_manifest(path, error) {
        Ok(spec) => spec,
        Err(exit_code) => return exit_code,
    };

    write_output(
        output,
        &format!("valid workload: {} ({})", spec.name, spec.schema_version),
    )
}

fn oracle_file(path: &Path, output: &mut impl Write, error: &mut impl Write) -> i32 {
    let spec = match load_manifest(path, error) {
        Ok(spec) => spec,
        Err(exit_code) => return exit_code,
    };
    let result = match ScalarOracle::try_new(&spec).and_then(|oracle| oracle.run_result()) {
        Ok(result) => result,
        Err(oracle_error) => {
            return write_error(
                error,
                5,
                &format!(
                    "scalar oracle failed for workload {:?}: {oracle_error}",
                    spec.name
                ),
            );
        }
    };
    let message = format!(
        "\
workload: {:?}
accepted_count: {}
score_sum: {}
category_histogram: {:?}
accepted_id_sum: 0x{:016x}
accepted_id_xor: 0x{:016x}",
        spec.name,
        result.accepted_count,
        result.score_sum,
        result.category_histogram,
        result.accepted_id_sum,
        result.accepted_id_xor,
    );
    write_output(output, &message)
}

fn load_manifest(path: &Path, error: &mut impl Write) -> Result<WorkloadSpec, i32> {
    let source = match fs::read_to_string(path) {
        Ok(source) => source,
        Err(read_error) => {
            return Err(write_error(
                error,
                3,
                &format!(
                    "cannot read workload manifest {:?}: {read_error}",
                    path.as_os_str()
                ),
            ));
        }
    };

    match parse_manifest(&source) {
        Ok(spec) => Ok(spec),
        Err(manifest_error) => Err(write_error(
            error,
            4,
            &format!(
                "invalid workload manifest {:?}: {manifest_error}",
                path.as_os_str()
            ),
        )),
    }
}

fn write_usage_error(error: &mut impl Write, message: &str) -> i32 {
    write_error(
        error,
        2,
        &format!("{message}\nrun 'paraflow-engine help' for usage"),
    )
}

fn write_error(error: &mut impl Write, exit_code: i32, message: &str) -> i32 {
    if writeln!(error, "error: {message}").is_err() {
        return 1;
    }
    exit_code
}

fn write_output(output: &mut impl Write, message: &str) -> i32 {
    if writeln!(output, "{message}").is_err() {
        return 1;
    }
    0
}
