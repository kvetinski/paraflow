//! Stable logical contracts shared by ParaFlow components.
//!
//! This crate describes workload semantics. It deliberately does not describe
//! a physical memory layout, FFI representation, scheduler, or backend.

#![forbid(unsafe_code)]

mod stage;
mod validation;
mod workload;

pub use stage::{PIPELINE_STAGES, Stage};

pub use validation::{
    BASIS_POINTS_MAX, MAX_CATEGORIES, MAX_SAFE_JSON_INTEGER, Validate, ValidationCode,
    ValidationErrors, ValidationIssue,
};

pub use workload::{
    AggregateSpec, DatasetSpec, DistributionSpec, FilterSpec, GeneratorAlgorithm, HistogramSpec,
    LogicalRecord, NormalizeSpec, PipelineSpec, ScoreSpec, WorkloadSpec,
};

/// The only workload schema accepted by the Day 1 foundation.
pub const WORKLOAD_SCHEMA: &str = "paraflow.workload/v1";
