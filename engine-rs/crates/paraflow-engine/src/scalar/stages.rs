use paraflow_contracts::{FilterSpec, LogicalRecord, NormalizeSpec, ScoreSpec};

/// Logical output of the scalar normalization stage.
///
/// This value is testable stage evidence, not a physical buffer or ABI.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct NormalizedRecord {
    id: u64,
    category: u32,
    flags: u32,
    normalized_a: f32,
    normalized_b: f32,
}

impl NormalizedRecord {
    /// Stable input identifier.
    #[must_use]
    pub const fn id(self) -> u64 {
        self.id
    }

    /// Logical category copied from the generated record.
    #[must_use]
    pub const fn category(self) -> u32 {
        self.category
    }

    /// Flags copied from the generated record.
    #[must_use]
    pub const fn flags(self) -> u32 {
        self.flags
    }

    /// Normalized feature A.
    #[must_use]
    pub const fn normalized_a(self) -> f32 {
        self.normalized_a
    }

    /// Normalized feature B.
    #[must_use]
    pub const fn normalized_b(self) -> f32 {
        self.normalized_b
    }
}

/// Logical output of the scalar scoring stage.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct ScoredRecord {
    id: u64,
    category: u32,
    score: f32,
}

impl ScoredRecord {
    /// Stable input identifier.
    #[must_use]
    pub const fn id(self) -> u64 {
        self.id
    }

    /// Logical category copied through the pipeline.
    #[must_use]
    pub const fn category(self) -> u32 {
        self.category
    }

    /// Scalar score calculated with the frozen operation order.
    #[must_use]
    pub const fn score(self) -> f32 {
        self.score
    }
}

/// A scored record that passed the inclusive scalar filter.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct AcceptedRecord(ScoredRecord);

impl AcceptedRecord {
    /// Stable input identifier.
    #[must_use]
    pub const fn id(self) -> u64 {
        self.0.id
    }

    /// Logical category copied through the pipeline.
    #[must_use]
    pub const fn category(self) -> u32 {
        self.0.category
    }

    /// Accepted scalar score.
    #[must_use]
    pub const fn score(self) -> f32 {
        self.0.score
    }

    #[cfg(test)]
    pub(super) const fn from_parts(id: u64, category: u32, score: f32) -> Self {
        Self(ScoredRecord {
            id,
            category,
            score,
        })
    }
}

/// Normalize one generated record with the exact workload-v1 operation order.
///
/// The parameters are expected to come from a validated workload.
///
/// # Panics
///
/// Panics when `parameters.clip` is negative or NaN. `WorkloadSpec::validate`
/// rejects both cases, so prepared [`super::ScalarOracle`] runs cannot panic
/// here.
#[must_use]
pub fn normalize_record(record: LogicalRecord, parameters: &NormalizeSpec) -> NormalizedRecord {
    let feature_a = record.feature_a as f32;
    let shifted_a = feature_a + parameters.offset_a;
    let scaled_a = shifted_a * parameters.scale_a;
    let normalized_a = scaled_a.clamp(-parameters.clip, parameters.clip);

    let feature_b = record.feature_b as f32;
    let shifted_b = feature_b + parameters.offset_b;
    let scaled_b = shifted_b * parameters.scale_b;
    let normalized_b = scaled_b.clamp(-parameters.clip, parameters.clip);

    NormalizedRecord {
        id: record.id,
        category: record.category,
        flags: record.flags,
        normalized_a,
        normalized_b,
    }
}

/// Score one normalized record with explicit, non-fused `f32` operations.
#[must_use]
pub fn score_record(record: NormalizedRecord, parameters: &ScoreSpec) -> ScoredRecord {
    let weighted_a = record.normalized_a * parameters.weight_a;
    let weighted_b = record.normalized_b * parameters.weight_b;
    let weighted_sum = weighted_a + weighted_b;
    let mut score = weighted_sum + parameters.bias;

    if record.flags & parameters.flag_mask == parameters.flag_mask {
        score += parameters.flag_bonus;
    }

    ScoredRecord {
        id: record.id,
        category: record.category,
        score,
    }
}

/// Apply the inclusive workload-v1 filter.
#[must_use]
pub fn filter_record(record: ScoredRecord, parameters: &FilterSpec) -> Option<AcceptedRecord> {
    (record.score >= parameters.min_score).then_some(AcceptedRecord(record))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn logical_record(feature_a: i32, feature_b: i32, flags: u32) -> LogicalRecord {
        LogicalRecord {
            id: 7,
            category: 2,
            feature_a,
            feature_b,
            flags,
        }
    }

    fn normalize_parameters() -> NormalizeSpec {
        NormalizeSpec {
            offset_a: 1.0,
            scale_a: 2.0,
            offset_b: -1.0,
            scale_b: 2.0,
            clip: 10.0,
        }
    }

    #[test]
    fn normalization_preserves_order_and_clamps_both_signs() {
        let normalized = normalize_record(logical_record(3, -10, 1), &normalize_parameters());

        assert_eq!(normalized.id(), 7);
        assert_eq!(normalized.category(), 2);
        assert_eq!(normalized.flags(), 1);
        assert_eq!(normalized.normalized_a().to_bits(), 8.0_f32.to_bits());
        assert_eq!(normalized.normalized_b().to_bits(), (-10.0_f32).to_bits());
    }

    #[test]
    fn integer_conversion_happens_before_offset_addition() {
        let parameters = NormalizeSpec {
            offset_a: -16_777_216.0,
            scale_a: 1.0,
            offset_b: 0.0,
            scale_b: 1.0,
            clip: 20_000_000.0,
        };

        let normalized = normalize_record(logical_record(16_777_217, 0, 0), &parameters);

        assert_eq!(normalized.normalized_a().to_bits(), 0.0_f32.to_bits());
    }

    #[test]
    fn normalization_clamps_finite_overflow() {
        let parameters = NormalizeSpec {
            offset_a: 0.0,
            scale_a: f32::MAX,
            offset_b: 0.0,
            scale_b: f32::MAX,
            clip: 8.0,
        };

        let normalized = normalize_record(logical_record(i32::MAX, i32::MIN, 0), &parameters);

        assert_eq!(normalized.normalized_a().to_bits(), 8.0_f32.to_bits());
        assert_eq!(normalized.normalized_b().to_bits(), (-8.0_f32).to_bits());
    }

    #[test]
    fn zero_flag_mask_intentionally_applies_the_bonus() {
        let normalized = NormalizedRecord {
            id: 0,
            category: 0,
            flags: 0,
            normalized_a: 2.5,
            normalized_b: -2.0,
        };
        let parameters = ScoreSpec {
            weight_a: 1.0,
            weight_b: 0.5,
            bias: 0.0,
            flag_mask: 0,
            flag_bonus: 0.5,
        };

        let scored = score_record(normalized, &parameters);

        assert_eq!(scored.score().to_bits(), 2.0_f32.to_bits());
    }

    #[test]
    fn scoring_preserves_the_declared_association() {
        let normalized = NormalizedRecord {
            id: 0,
            category: 0,
            flags: 0,
            normalized_a: 1.0e20,
            normalized_b: -1.0e20,
        };
        let parameters = ScoreSpec {
            weight_a: 1.0,
            weight_b: 1.0,
            bias: 1.0,
            flag_mask: 1,
            flag_bonus: 0.0,
        };

        let scored = score_record(normalized, &parameters);

        assert_eq!(scored.score().to_bits(), 1.0_f32.to_bits());
    }

    #[test]
    fn scoring_does_not_contract_multiply_and_add() {
        let normalized = NormalizedRecord {
            id: 0,
            category: 0,
            flags: 0,
            normalized_a: f32::from_bits(0x3f80_0001),
            normalized_b: 1.0,
        };
        let parameters = ScoreSpec {
            weight_a: f32::from_bits(0x3f80_0001),
            weight_b: f32::from_bits(0xbf80_0002),
            bias: 0.0,
            flag_mask: 1,
            flag_bonus: 0.0,
        };

        let scored = score_record(normalized, &parameters);

        assert_eq!(scored.score().to_bits(), 0x0000_0000);
    }

    #[test]
    fn finite_score_operands_can_produce_ieee_infinity_and_nan() {
        let parameters = ScoreSpec {
            weight_a: f32::MAX,
            weight_b: f32::MAX,
            bias: 0.0,
            flag_mask: 1,
            flag_bonus: 0.0,
        };
        let filter = FilterSpec { min_score: 0.0 };

        let positive_infinity = score_record(
            NormalizedRecord {
                id: 0,
                category: 0,
                flags: 0,
                normalized_a: f32::MAX,
                normalized_b: 0.0,
            },
            &parameters,
        );
        assert!(positive_infinity.score().is_infinite());
        assert!(positive_infinity.score().is_sign_positive());
        assert!(filter_record(positive_infinity, &filter).is_some());

        let not_a_number = score_record(
            NormalizedRecord {
                id: 1,
                category: 0,
                flags: 0,
                normalized_a: f32::MAX,
                normalized_b: -f32::MAX,
            },
            &parameters,
        );
        assert!(not_a_number.score().is_nan());
        assert!(filter_record(not_a_number, &filter).is_none());
    }

    #[test]
    fn filtering_is_inclusive_and_uses_ieee_comparisons() {
        let parameters = FilterSpec { min_score: 0.5 };

        let below = ScoredRecord {
            id: 0,
            category: 0,
            score: f32::from_bits(0x3eff_ffff),
        };
        let equal = ScoredRecord {
            id: 1,
            category: 0,
            score: 0.5,
        };
        let above = ScoredRecord {
            id: 2,
            category: 0,
            score: f32::from_bits(0x3f00_0001),
        };
        let not_a_number = ScoredRecord {
            id: 3,
            category: 0,
            score: f32::NAN,
        };
        let positive_infinity = ScoredRecord {
            id: 4,
            category: 0,
            score: f32::INFINITY,
        };
        let negative_infinity = ScoredRecord {
            id: 5,
            category: 0,
            score: f32::NEG_INFINITY,
        };

        assert!(filter_record(below, &parameters).is_none());
        assert!(filter_record(equal, &parameters).is_some());
        assert!(filter_record(above, &parameters).is_some());
        assert!(filter_record(not_a_number, &parameters).is_none());
        assert!(filter_record(positive_infinity, &parameters).is_some());
        assert!(filter_record(negative_infinity, &parameters).is_none());
    }
}
