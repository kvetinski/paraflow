//! Deterministic reference generation for `paraflow.workload/v1`.
//!
//! Random access is the fundamental operation. The lazy iterator and range
//! materializer are conveniences built on top of it, not a frozen physical
//! layout or ABI.

mod splitmix64_v1;

use std::{collections::TryReserveError, error::Error, fmt, iter::FusedIterator, ops::Range};

use paraflow_contracts::{
    DatasetSpec, DistributionSpec, GeneratorAlgorithm, LogicalRecord, Validate, ValidationErrors,
    BASIS_POINTS_MAX,
};

pub use splitmix64_v1::{mix_v1, sample_v1};

/// Stable field identifiers from the workload-v1 generator contract.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[repr(u64)]
pub enum FieldId {
    /// Category selection and hotspot decision.
    CategorySelector = 0,
    /// First raw feature.
    FeatureA = 1,
    /// Second raw feature.
    FeatureB = 2,
    /// Bit-zero flag decision.
    Flags = 3,
    /// Non-hotspot category selection.
    HotspotFallback = 4,
}

impl FieldId {
    const fn value(self) -> u64 {
        self as u64
    }
}

/// A validated, stateless view of one dataset specification.
///
/// Constructing the generator validates every invariant needed to avoid
/// undefined ranges or modulo-by-zero. Individual records can then be generated
/// independently in any visit order.
#[derive(Debug, Clone, Copy)]
pub struct DatasetGenerator<'a> {
    dataset: &'a DatasetSpec,
}

impl<'a> DatasetGenerator<'a> {
    /// Validate a dataset once and prepare deterministic record generation.
    pub fn try_new(dataset: &'a DatasetSpec) -> Result<Self, ValidationErrors> {
        dataset.validate()?;
        Ok(Self { dataset })
    }

    /// Return the immutable dataset contract used by this generator.
    #[must_use]
    pub const fn dataset(&self) -> &'a DatasetSpec {
        self.dataset
    }

    /// Generate one record by absolute input index.
    ///
    /// Returns `None` when `index` is outside the configured record count.
    #[must_use]
    pub fn record_at(&self, index: u64) -> Option<LogicalRecord> {
        (index < self.dataset.record_count).then(|| self.record_at_valid_index(index))
    }

    /// Lazily visit all logical records in stable input order.
    #[must_use]
    pub const fn records(self) -> Records<'a> {
        Records {
            generator: self,
            next_index: 0,
        }
    }

    /// Materialize an absolute index range as the current scalar reference
    /// representation.
    ///
    /// The returned `Vec` is an engine convenience, not part of the workload
    /// contract. Later AoS and SoA backends may materialize the same indices
    /// differently.
    pub fn generate_range(&self, range: Range<u64>) -> Result<Vec<LogicalRecord>, GenerationError> {
        if range.start > range.end || range.end > self.dataset.record_count {
            return Err(GenerationError::InvalidRange {
                start: range.start,
                end: range.end,
                record_count: self.dataset.record_count,
            });
        }

        let length_u64 = range.end - range.start;
        let length = usize::try_from(length_u64).map_err(|_| {
            GenerationError::LengthExceedsAddressSpace {
                requested_records: length_u64,
            }
        })?;
        let mut records = Vec::new();
        records
            .try_reserve_exact(length)
            .map_err(|source| GenerationError::AllocationFailed {
                requested_records: length_u64,
                source,
            })?;
        records.extend(range.map(|index| self.record_at_valid_index(index)));
        Ok(records)
    }

    /// Materialize every configured record in stable input order.
    pub fn generate_all(&self) -> Result<Vec<LogicalRecord>, GenerationError> {
        self.generate_range(0..self.dataset.record_count)
    }

    fn record_at_valid_index(&self, index: u64) -> LogicalRecord {
        debug_assert!(index < self.dataset.record_count);

        LogicalRecord {
            id: index,
            category: self.category(index),
            feature_a: self.feature(index, FieldId::FeatureA),
            feature_b: self.feature(index, FieldId::FeatureB),
            flags: self.flags(index),
        }
    }

    fn feature(&self, index: u64, field: FieldId) -> i32 {
        let minimum = i64::from(self.dataset.feature_min);
        let maximum = i64::from(self.dataset.feature_max);
        let span = u64::try_from(maximum - minimum)
            .expect("validated feature bounds must form a positive u64 span");
        let offset = self.sample(index, field) % span;
        i32::try_from(minimum + i64::try_from(offset).expect("an i32 feature span must fit in i64"))
            .expect("the generated feature must remain inside the validated i32 range")
    }

    fn category(&self, index: u64) -> u32 {
        let selector = self.sample(index, FieldId::CategorySelector);

        match self.dataset.distribution {
            DistributionSpec::Uniform => {
                u32::try_from(selector % u64::from(self.dataset.category_count))
                    .expect("modulo category_count must fit in u32")
            }
            DistributionSpec::Hotspot {
                category,
                probability_bps,
            } => {
                if self.dataset.category_count == 1 {
                    return 0;
                }
                if selector % u64::from(BASIS_POINTS_MAX) < u64::from(probability_bps) {
                    return category;
                }

                let fallback = self.sample(index, FieldId::HotspotFallback);
                let slot = u32::try_from(fallback % u64::from(self.dataset.category_count - 1))
                    .expect("fallback category slot must fit in u32");
                slot + u32::from(slot >= category)
            }
        }
    }

    fn flags(&self, index: u64) -> u32 {
        let selected = self.sample(index, FieldId::Flags) % u64::from(BASIS_POINTS_MAX)
            < u64::from(self.dataset.flag_probability_bps);
        u32::from(selected)
    }

    fn sample(&self, index: u64, field: FieldId) -> u64 {
        match self.dataset.generator {
            GeneratorAlgorithm::SplitMix64V1 => sample_v1(self.dataset.seed, index, field.value()),
        }
    }
}

/// Lazy stable-order iteration over one validated dataset.
#[derive(Debug, Clone)]
pub struct Records<'a> {
    generator: DatasetGenerator<'a>,
    next_index: u64,
}

impl Iterator for Records<'_> {
    type Item = LogicalRecord;

    fn next(&mut self) -> Option<Self::Item> {
        let record = self.generator.record_at(self.next_index)?;
        self.next_index += 1;
        Some(record)
    }

    fn size_hint(&self) -> (usize, Option<usize>) {
        let remaining = self
            .generator
            .dataset
            .record_count
            .saturating_sub(self.next_index);
        match usize::try_from(remaining) {
            Ok(exact) => (exact, Some(exact)),
            Err(_) => (usize::MAX, None),
        }
    }
}

impl FusedIterator for Records<'_> {}

/// Failure to materialize a requested reference batch.
#[derive(Debug)]
pub enum GenerationError {
    /// The range is reversed or extends beyond the configured record count.
    InvalidRange {
        /// Inclusive range start.
        start: u64,
        /// Exclusive range end.
        end: u64,
        /// Dataset record count.
        record_count: u64,
    },
    /// The requested number of records cannot be indexed by this process.
    LengthExceedsAddressSpace {
        /// Number of requested records.
        requested_records: u64,
    },
    /// The process could not reserve storage for the requested records.
    AllocationFailed {
        /// Number of requested records.
        requested_records: u64,
        /// Allocation failure reported by the standard library.
        source: TryReserveError,
    },
}

impl fmt::Display for GenerationError {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::InvalidRange {
                start,
                end,
                record_count,
            } => write!(
                formatter,
                "generation range {start}..{end} is invalid for dataset domain 0..{record_count}"
            ),
            Self::LengthExceedsAddressSpace { requested_records } => write!(
                formatter,
                "cannot address a materialized batch of {requested_records} records on this process"
            ),
            Self::AllocationFailed {
                requested_records,
                source,
            } => write!(
                formatter,
                "cannot allocate a materialized batch of {requested_records} records: {source}"
            ),
        }
    }
}

impl Error for GenerationError {
    fn source(&self) -> Option<&(dyn Error + 'static)> {
        match self {
            Self::AllocationFailed { source, .. } => Some(source),
            Self::InvalidRange { .. } | Self::LengthExceedsAddressSpace { .. } => None,
        }
    }
}
