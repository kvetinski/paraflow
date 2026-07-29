//! Typed, stable-order Rust scalar correctness oracle.
//!
//! Stage values remain explicit for conformance testing, while orchestration
//! streams one record at a time to avoid freezing intermediate buffer layouts.

mod aggregate;
mod stages;

use std::{
    collections::TryReserveError,
    error::Error,
    fmt,
    time::{Duration, Instant},
};

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

/// Boundary timings from the Day 6 materialized stage-pass profiler.
///
/// These durations describe a diagnostic topology with explicit intermediate
/// buffers. They are not a decomposition of the fused Day 5 scalar loop.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) struct ScalarStageDurations {
    pub(crate) normalize: Duration,
    pub(crate) score: Duration,
    pub(crate) filter: Duration,
    pub(crate) aggregate: Duration,
}

/// Result and raw stage timings from one diagnostic scalar profile pass.
#[derive(Debug, Clone, PartialEq)]
pub(crate) struct ProfiledScalarOutput {
    pub(crate) result: ResultV1,
    pub(crate) stages: ScalarStageDurations,
}

/// Fallibly allocated output owned by the scalar oracle.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum OutputBuffer {
    /// One wrapping counter per logical category.
    CategoryHistogram,
    /// Optional stable accepted-ID diagnostics.
    CompactedIds,
    /// Day 6 diagnostic normalized-record stage buffer.
    NormalizedRecords,
    /// Day 6 diagnostic scored-record stage buffer.
    ScoredRecords,
    /// Day 6 diagnostic stable accepted-record stage buffer.
    AcceptedRecords,
}

impl fmt::Display for OutputBuffer {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::CategoryHistogram => formatter.write_str("category histogram"),
            Self::CompactedIds => formatter.write_str("compacted IDs"),
            Self::NormalizedRecords => formatter.write_str("normalized records"),
            Self::ScoredRecords => formatter.write_str("scored records"),
            Self::AcceptedRecords => formatter.write_str("accepted records"),
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

    /// Execute an explicit materialized pass for each scalar pipeline stage.
    ///
    /// This is profiling-only code. It preserves workload semantics and stable
    /// order, but intentionally changes physical execution topology so each
    /// stage has one coarse timer rather than one timer per record. Callers must
    /// report it as `materialized-stage-passes-v1`, never as the fused baseline.
    pub(crate) fn run_materialized_profiled(
        &self,
        records: &[LogicalRecord],
    ) -> Result<ProfiledScalarOutput, ScalarError> {
        let normalize_started = Instant::now();
        let mut normalized = try_stage_buffer(records.len(), OutputBuffer::NormalizedRecords)?;
        normalized.extend(
            records
                .iter()
                .copied()
                .map(|record| normalize_record(record, &self.pipeline.normalize)),
        );
        let normalize = normalize_started.elapsed();

        let score_started = Instant::now();
        let mut scored = try_stage_buffer(normalized.len(), OutputBuffer::ScoredRecords)?;
        scored.extend(
            normalized
                .iter()
                .copied()
                .map(|record| score_record(record, &self.pipeline.score)),
        );
        let score = score_started.elapsed();

        let filter_started = Instant::now();
        let mut accepted = try_stage_buffer(scored.len(), OutputBuffer::AcceptedRecords)?;
        accepted.extend(
            scored
                .iter()
                .copied()
                .filter_map(|record| filter_record(record, &self.pipeline.filter)),
        );
        let filter = filter_started.elapsed();

        let aggregate_started = Instant::now();
        let mut accumulator = Accumulator::try_new(self.category_count, CompactedIdCapture::Omit)?;
        for record in accepted.iter().copied() {
            accumulator.accept(record)?;
        }
        let result = accumulator.finish().result;
        let aggregate = aggregate_started.elapsed();

        Ok(ProfiledScalarOutput {
            result,
            stages: ScalarStageDurations {
                normalize,
                score,
                filter,
                aggregate,
            },
        })
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

fn try_stage_buffer<T>(
    requested_items: usize,
    buffer: OutputBuffer,
) -> Result<Vec<T>, ScalarError> {
    let mut values = Vec::new();
    values
        .try_reserve_exact(requested_items)
        .map_err(|source| ScalarError::AllocationFailed {
            buffer,
            requested_items: u64::try_from(requested_items).unwrap_or(u64::MAX),
            source,
        })?;
    Ok(values)
}

#[cfg(test)]
mod profile_tests {
    use super::*;

    fn workload() -> WorkloadSpec {
        serde_json::from_str(include_str!("../../../../../workloads/edge-scalar-v1.json"))
            .expect("checked-in workload must decode")
    }

    #[test]
    fn materialized_stage_passes_match_the_fused_scalar_result_exactly() {
        let workload = workload();
        let oracle = ScalarOracle::try_new(&workload).expect("workload must be valid");
        let records = DatasetGenerator::try_new(&workload.dataset)
            .expect("dataset must be valid")
            .generate_all()
            .expect("small dataset must materialize");

        let fused = oracle
            .run_materialized_result(&records)
            .expect("fused path must run");
        let profiled = oracle
            .run_materialized_profiled(&records)
            .expect("profile path must run");

        assert_eq!(profiled.result.accepted_count, fused.accepted_count);
        assert_eq!(
            profiled.result.score_sum.to_bits(),
            fused.score_sum.to_bits()
        );
        assert_eq!(profiled.result.category_histogram, fused.category_histogram);
        assert_eq!(profiled.result.accepted_id_sum, fused.accepted_id_sum);
        assert_eq!(profiled.result.accepted_id_xor, fused.accepted_id_xor);
    }

    #[test]
    fn empty_materialized_stage_passes_preserve_identity_results() {
        let mut workload = workload();
        workload.dataset.record_count = 0;
        let oracle = ScalarOracle::try_new(&workload).expect("empty workload must be valid");

        let profiled = oracle
            .run_materialized_profiled(&[])
            .expect("empty profile must run");

        assert_eq!(profiled.result.accepted_count, 0);
        assert_eq!(profiled.result.score_sum.to_bits(), 0.0_f64.to_bits());
        assert_eq!(
            profiled.result.category_histogram,
            vec![0; workload.dataset.category_count as usize]
        );
    }
}
