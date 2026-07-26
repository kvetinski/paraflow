use paraflow_contracts::ResultV1;

use crate::generation::mix_v1;

use super::{AcceptedRecord, CompactedIdCapture, OutputBuffer, ScalarError, ScalarRunOutput};

pub(super) struct Accumulator {
    result: ResultV1,
    compacted_ids: Option<Vec<u64>>,
    category_count: u32,
}

impl Accumulator {
    pub(super) fn try_new(
        category_count: u32,
        capture: CompactedIdCapture,
    ) -> Result<Self, ScalarError> {
        let histogram_length = usize::try_from(category_count).map_err(|_| {
            ScalarError::LengthExceedsAddressSpace {
                buffer: OutputBuffer::CategoryHistogram,
                requested_items: u64::from(category_count),
            }
        })?;
        let mut category_histogram = Vec::new();
        category_histogram
            .try_reserve_exact(histogram_length)
            .map_err(|source| ScalarError::AllocationFailed {
                buffer: OutputBuffer::CategoryHistogram,
                requested_items: u64::from(category_count),
                source,
            })?;
        category_histogram.resize(histogram_length, 0);

        let compacted_ids = match capture {
            CompactedIdCapture::Omit => None,
            CompactedIdCapture::Collect => Some(Vec::new()),
        };

        Ok(Self {
            result: ResultV1 {
                accepted_count: 0,
                score_sum: 0.0,
                category_histogram,
                accepted_id_sum: 0,
                accepted_id_xor: 0,
            },
            compacted_ids,
            category_count,
        })
    }

    pub(super) fn accept(&mut self, record: AcceptedRecord) -> Result<(), ScalarError> {
        let category = usize::try_from(record.category()).map_err(|_| {
            ScalarError::InvalidGeneratedCategory {
                record_id: record.id(),
                category: record.category(),
                category_count: self.category_count,
            }
        })?;
        if category >= self.result.category_histogram.len() {
            return Err(ScalarError::InvalidGeneratedCategory {
                record_id: record.id(),
                category: record.category(),
                category_count: self.category_count,
            });
        }

        if let Some(ids) = self.compacted_ids.as_mut() {
            if ids.len() == ids.capacity() {
                let requested_items = u64::try_from(ids.len())
                    .unwrap_or(u64::MAX)
                    .saturating_add(1);
                ids.try_reserve(1)
                    .map_err(|source| ScalarError::AllocationFailed {
                        buffer: OutputBuffer::CompactedIds,
                        requested_items,
                        source,
                    })?;
            }
            ids.push(record.id());
        }

        self.result.accepted_count = self.result.accepted_count.wrapping_add(1);
        self.result.score_sum += f64::from(record.score());
        self.result.category_histogram[category] =
            self.result.category_histogram[category].wrapping_add(1);
        self.result.accepted_id_sum = self.result.accepted_id_sum.wrapping_add(record.id());
        self.result.accepted_id_xor ^= mix_v1(record.id());

        Ok(())
    }

    pub(super) fn finish(self) -> ScalarRunOutput {
        ScalarRunOutput {
            result: self.result,
            compacted_ids: self.compacted_ids,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn empty_output_has_the_canonical_identity_and_requested_histogram_width() {
        let output = Accumulator::try_new(3, CompactedIdCapture::Collect)
            .expect("small output must allocate")
            .finish();

        assert_eq!(output.result.accepted_count, 0);
        assert_eq!(output.result.score_sum.to_bits(), 0.0_f64.to_bits());
        assert_eq!(output.result.category_histogram, [0, 0, 0]);
        assert_eq!(output.result.accepted_id_sum, 0);
        assert_eq!(output.result.accepted_id_xor, 0);
        assert_eq!(output.compacted_ids, Some(Vec::new()));
    }

    #[test]
    fn aggregation_uses_stable_f64_addition_histograms_and_mixed_ids() {
        let mut accumulator = Accumulator::try_new(3, CompactedIdCapture::Collect)
            .expect("small output must allocate");

        for record in [
            AcceptedRecord::from_parts(0, 1, 1.5),
            AcceptedRecord::from_parts(1, 0, -0.25),
            AcceptedRecord::from_parts(2, 1, 2.0),
        ] {
            accumulator.accept(record).expect("category must be valid");
        }
        let output = accumulator.finish();

        assert_eq!(output.result.accepted_count, 3);
        assert_eq!(output.result.score_sum.to_bits(), 3.25_f64.to_bits());
        assert_eq!(output.result.category_histogram, [1, 2, 0]);
        assert_eq!(output.result.accepted_id_sum, 3);
        assert_eq!(output.result.accepted_id_xor, 0xe472_b00b_ee88_c7a0);
        assert_eq!(output.compacted_ids, Some(vec![0, 1, 2]));
    }

    #[test]
    fn integer_aggregation_wraps_and_histogram_access_is_checked() {
        let mut accumulator =
            Accumulator::try_new(1, CompactedIdCapture::Omit).expect("small output must allocate");
        accumulator.result.accepted_count = u64::MAX;
        accumulator.result.category_histogram[0] = u64::MAX;
        accumulator.result.accepted_id_sum = u64::MAX;

        accumulator
            .accept(AcceptedRecord::from_parts(1, 0, 1.0))
            .expect("valid category must aggregate");

        assert_eq!(accumulator.result.accepted_count, 0);
        assert_eq!(accumulator.result.category_histogram, [0]);
        assert_eq!(accumulator.result.accepted_id_sum, 0);

        let error = accumulator
            .accept(AcceptedRecord::from_parts(2, 1, 1.0))
            .expect_err("invalid category must not index the histogram");
        assert!(matches!(
            error,
            ScalarError::InvalidGeneratedCategory {
                record_id: 2,
                category: 1,
                category_count: 1
            }
        ));
    }

    #[test]
    fn score_sum_uses_f64_and_preserves_stable_order() {
        let mut accumulator =
            Accumulator::try_new(1, CompactedIdCapture::Omit).expect("small output must allocate");

        for (id, score) in [(0, f32::MAX), (1, 1.0_f32), (2, -f32::MAX)] {
            accumulator
                .accept(AcceptedRecord::from_parts(id, 0, score))
                .expect("valid category must aggregate");
        }

        assert_eq!(accumulator.result.score_sum.to_bits(), 0.0_f64.to_bits());

        let reordered = f64::from(f32::MAX) + f64::from(-f32::MAX) + 1.0;
        assert_eq!(reordered.to_bits(), 1.0_f64.to_bits());
    }

    #[test]
    fn positive_infinite_accepted_score_produces_positive_infinite_sum() {
        let mut accumulator =
            Accumulator::try_new(1, CompactedIdCapture::Omit).expect("small output must allocate");
        accumulator
            .accept(AcceptedRecord::from_parts(0, 0, f32::INFINITY))
            .expect("valid category must aggregate");
        assert!(accumulator.result.score_sum.is_infinite());
        assert!(accumulator.result.score_sum.is_sign_positive());
    }
}
