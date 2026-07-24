use serde::{Deserialize, Serialize};

/// A complete logical workload, independent of any execution backend.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct WorkloadSpec {
    /// Optional editor hint pointing at the repository JSON Schema.
    #[serde(rename = "$schema", default, skip_serializing_if = "Option::is_none")]
    pub json_schema: Option<String>,
    /// Versioned identifier for the manifest schema.
    pub schema_version: String,
    /// Human-readable workload identity used in reports.
    pub name: String,
    /// Deterministic input-generation configuration.
    pub dataset: DatasetSpec,
    /// Parameters for the stable logical pipeline.
    pub pipeline: PipelineSpec,
}

/// Configuration for deterministic logical records.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct DatasetSpec {
    /// Number of records to generate. Zero is valid for correctness tests.
    pub record_count: u64,
    /// Root seed from which every record field is derived.
    pub seed: u64,
    /// Counter-based generator algorithm.
    pub generator: GeneratorAlgorithm,
    /// Category distribution used by the generator.
    pub distribution: DistributionSpec,
    /// Inclusive lower bound for both raw integer features.
    pub feature_min: i32,
    /// Exclusive upper bound for both raw integer features.
    pub feature_max: i32,
    /// Number of logical categories.
    pub category_count: u32,
    /// Probability that bit zero is set in `flags`, in basis points.
    pub flag_probability_bps: u16,
}

/// A schedule-independent record generator.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum GeneratorAlgorithm {
    /// SplitMix64 keyed by seed, record index, and field number.
    #[serde(rename = "splitmix64-v1")]
    SplitMix64V1,
}

/// The requested category distribution.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(tag = "kind", rename_all = "kebab-case", deny_unknown_fields)]
pub enum DistributionSpec {
    /// All categories are selected with equal probability.
    Uniform,
    /// One category receives a configured share of records.
    Hotspot {
        /// Category receiving the configured hot share.
        category: u32,
        /// Hot category probability in basis points.
        probability_bps: u16,
    },
}

/// Parameters for the stable five-stage pipeline.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct PipelineSpec {
    /// Raw-feature normalization.
    pub normalize: NormalizeSpec,
    /// Per-record score calculation.
    pub score: ScoreSpec,
    /// Score-based selection and stable compaction.
    pub filter: FilterSpec,
    /// Final reduction configuration.
    pub aggregate: AggregateSpec,
}

/// Parameters for converting raw integer features to bounded floats.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct NormalizeSpec {
    /// Offset applied to feature A before scaling.
    pub offset_a: f32,
    /// Positive scale applied to feature A.
    pub scale_a: f32,
    /// Offset applied to feature B before scaling.
    pub offset_b: f32,
    /// Positive scale applied to feature B.
    pub scale_b: f32,
    /// Positive symmetric clamp bound.
    pub clip: f32,
}

/// Parameters for calculating a score from normalized features.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct ScoreSpec {
    /// Multiplier for normalized feature A.
    pub weight_a: f32,
    /// Multiplier for normalized feature B.
    pub weight_b: f32,
    /// Constant added to every score.
    pub bias: f32,
    /// Mask that enables `flag_bonus` when all selected bits are set.
    pub flag_mask: u32,
    /// Conditional score contribution controlled by `flag_mask`.
    pub flag_bonus: f32,
}

/// Parameters for selection and stable compaction.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct FilterSpec {
    /// Inclusive score threshold for accepted records.
    pub min_score: f32,
}

/// Parameters for final aggregation.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct AggregateSpec {
    /// Histogram dimension.
    pub histogram: HistogramSpec,
}

/// Supported histogram dimensions.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum HistogramSpec {
    /// Produce one count per logical category.
    Category,
}

/// Logical record semantics shared by all future physical layouts.
///
/// This is not a C ABI or a promise that records are stored in array-of-struct
/// form. Week 4 will compare physical layouts without changing these fields.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct LogicalRecord {
    /// Stable record identifier equal to the input position.
    pub id: u64,
    /// Logical category in `[0, category_count)`.
    pub category: u32,
    /// First raw integer feature.
    pub feature_a: i32,
    /// Second raw integer feature.
    pub feature_b: i32,
    /// Bit field used by scoring.
    pub flags: u32,
}
