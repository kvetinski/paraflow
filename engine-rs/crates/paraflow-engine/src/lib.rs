//! Command-line entry points for the ParaFlow engine.
//!
//! The Day 1 executable can report its version and contract. Workload
//! execution is intentionally unavailable until the scalar oracle exists.

#![forbid(unsafe_code)]

use std::io::Write;

use paraflow_contracts::{PIPELINE_STAGES, WORKLOAD_SCHEMA};

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
        Some(command) => write_usage_error(stderr, &format!("unknown command: {command}")),
    }
}

fn write_help(output: &mut impl Write) -> i32 {
    let help = "\
ParaFlow execution engine

Usage:
  paraflow-engine contract
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
    if writeln!(
        error,
        "error: {message}\nrun 'paraflow-engine help' for usage"
    )
    .is_err()
    {
        return 1;
    }
    2
}

fn write_output(output: &mut impl Write, message: &str) -> i32 {
    if writeln!(output, "{message}").is_err() {
        return 1;
    }
    0
}
