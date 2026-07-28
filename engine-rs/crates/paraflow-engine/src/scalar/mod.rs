//! Typed, stable-order Rust scalar correctness oracle.
//!
//! Stage values remain explicit for conformance testing, while orchestration
//! streams one record at a time to avoid freezing intermediate buffer layouts.

mod aggregate;
mod stages;

use std::{collections::TryReserveError, error::Error, fmt};

use aggregate::Accumulator;
use paraflow_contracts::{
    LogicalRecord, PipelineSpec, ResultV1, Validate, ValidationErrors, WorkloadSpec,
};

use crate::generation::DatasetGenerator;

pub use stages::{
    AcceptedRecord, NormalizedRecord, ScoredRecord, filter_record, normalize_record, score_record,
};

/// Whether a scalar run should retain every accepted identifier.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub enum CompactedIdCapture {
    /// Produce only the canonical result.
    #[default]
    Omit,
    /// Also retain the complete stable accepted-ID sequence.
    Collect,
}

/// Options that change diagnostics without changing workload semantics.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub struct ScalarRunOptions {
    /// Optional stable-compaction diagnostic.
    pub compacted_ids: CompactedIdCapture,
}

/// Output of one scalar oracle run.
#[derive(Debug, Clone, PartialEq)]
pub struct ScalarRunOutput {
    /// Canonical workload-v1 result.
    pub result: ResultV1,
    /// Stable accepted IDs when explicitly requested.
    pub compacted_ids: Option<Vec<u64>>,
}

/// Fallibly allocated output owned by the scalar oracle.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum OutputBuffer {
    /// One wrapping counter per logical category.
    CategoryHistogram,
    /// Optional stable accepted-ID diagnostics.
    CompactedIds,
}

impl fmt::Display for OutputBuffer {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::CategoryHistogram => formatter.write_str("category histogram"),
            Self::CompactedIds => formatter.write_str("compacted IDs"),
        }
    }
}

/// A scalar oracle could not be prepared or completed.
#[derive(Debug)]
pub enum ScalarError {
    /// The workload violates one or more semantic invariants.
    InvalidWorkload(ValidationErrors),
    /// A requested output length cannot be indexed by this process.
    LengthExceedsAddressSpace {
        /// Output that could not be represented.
        buffer: OutputBuffer,
        /// Requested logical item count.
        requested_items: u64,
    },
    /// Storage could not be reserved for a scalar output.
    AllocationFailed {
        /// Output that could not be allocated.
        buffer: OutputBuffer,
        /// Requested logical item count.
        requested_items: u64,
        /// Allocation failure reported by the standard library.
        source: TryReserveError,
    },
    /// Deterministic generation violated the validated category domain.
    InvalidGeneratedCategory {
        /// Record exposing the invariant failure.
        record_id: u64,
        /// Generated category.
        category: u32,
        /// Valid category count.
        category_count: u32,
    },
}

impl fmt::Display for ScalarError {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::InvalidWorkload(errors) => write!(formatter, "{errors}"),
            Self::LengthExceedsAddressSpace {
                buffer,
                requested_items,
            } => write!(
                formatter,
                "cannot address {requested_items} item(s) for the {buffer} on this process"
            ),
            Self::AllocationFailed {
                buffer,
                requested_items,
                source,
            } => write!(
                formatter,
                "cannot allocate {requested_items} item(s) for the {buffer}: {source}"
            ),
            Self::InvalidGeneratedCategory {
                record_id,
                category,
                category_count,
            } => write!(
                formatter,
                "generated record {record_id} has category {category} outside 0..{category_count}"
            ),
        }
    }
}

impl Error for ScalarError {
    fn source(&self) -> Option<&(dyn Error + 'static)> {
        match self {
            Self::InvalidWorkload(errors) => Some(errors),
            Self::AllocationFailed { source, .. } => Some(source),
            Self::LengthExceedsAddressSpace { .. } | Self::InvalidGeneratedCategory { .. } => None,
        }
    }
}

/// Prepared scalar execution over one immutable, validated workload.
#[derive(Debug, Clone, Copy)]
pub struct ScalarOracle<'a> {
    generator: DatasetGenerator<'a>,
    pipeline: &'a PipelineSpec,
    category_count: u32,
}

impl<'a> ScalarOracle<'a> {
    /// Validate a complete workload and prepare scalar execution.
    pub fn try_new(workload: &'a WorkloadSpec) -> Result<Self, ScalarError> {
        workload.validate().map_err(ScalarError::InvalidWorkload)?;
        let generator =
            DatasetGenerator::try_new(&workload.dataset).map_err(ScalarError::InvalidWorkload)?;

        Ok(Self {
            generator,
            pipeline: &workload.pipeline,
            category_count: workload.dataset.category_count,
        })
    }

    /// Execute every stage in stable input order.
    pub fn run(&self, options: ScalarRunOptions) -> Result<ScalarRunOutput, ScalarError> {
        self.run_records(self.generator.records(), options)
    }

    /// Execute the canonical result path without retaining accepted IDs.
    pub fn run_result(&self) -> Result<ResultV1, ScalarError> {
        self.run(ScalarRunOptions::default())
            .map(|output| output.result)
    }

    /// Execute normalize through aggregate over a materialized logical batch.
    ///
    /// The Day 5 harness generates this batch from the same validated workload
    /// immediately before calling this method. Keeping the materialized path
    /// internal prevents it from becoming a public layout or ABI promise.
    pub(crate) fn run_materialized_result(
        &self,
        records: &[LogicalRecord],
    ) -> Result<ResultV1, ScalarError> {
        self.run_records(records.iter().copied(), ScalarRunOptions::default())
            .map(|output| output.result)
    }

    fn run_records(
        &self,
        records: impl IntoIterator<Item = LogicalRecord>,
        options: ScalarRunOptions,
    ) -> Result<ScalarRunOutput, ScalarError> {
        let mut accumulator = Accumulator::try_new(self.category_count, options.compacted_ids)?;

        for logical in records {
            let normalized = normalize_record(logical, &self.pipeline.normalize);
            let scored = score_record(normalized, &self.pipeline.score);
            if let Some(accepted) = filter_record(scored, &self.pipeline.filter) {
                accumulator.accept(accepted)?;
            }
        }

        Ok(accumulator.finish())
    }
}
