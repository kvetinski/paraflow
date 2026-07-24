//! Stable logical contracts shared by ParaFlow components.
//!
//! This crate describes workload semantics. It deliberately does not describe
//! a physical memory layout, FFI representation, scheduler, or backend.

#![forbid(unsafe_code)]

mod stage;

pub use stage::{PIPELINE_STAGES, Stage};

/// The only workload schema accepted by the Day 1 foundation.
pub const WORKLOAD_SCHEMA: &str = "paraflow.workload/v1";
