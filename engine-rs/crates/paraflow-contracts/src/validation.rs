use std::{error::Error, fmt};

use crate::{DatasetSpec, DistributionSpec, WORKLOAD_SCHEMA, WorkloadSpec};

/// Maximum probability in basis points.
pub const BASIS_POINTS_MAX: u16 = 10_000;

/// Upper bound that prevents accidentally allocating unbounded histograms.
pub const MAX_CATEGORIES: u32 = 65_536;

/// Largest integer every supported JSON tool can represent without rounding.
pub const MAX_SAFE_JSON_INTEGER: u64 = (1_u64 << 53) - 1;

const MAX_NAME_CHARS: usize = 120;

/// Stable category for a workload validation failure.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ValidationCode {
    /// The manifest targets an unsupported schema.
    UnsupportedSchema,
    /// The workload name is blank.
    BlankName,
    /// The workload name exceeds the contract limit.
    NameTooLong,
    /// A manifest integer exceeds the cross-language lossless JSON range.
    UnsafeJsonInteger,
    /// Category count is zero.
    EmptyCategories,
    /// Category count exceeds the contract limit.
    TooManyCategories,
    /// Raw feature bounds do not form a non-empty range.
    InvalidFeatureRange,
    /// A probability is outside `[0, 10_000]` basis points.
    InvalidProbability,
    /// The configured hotspot category does not exist.
    InvalidHotspotCategory,
    /// A floating-point parameter is NaN or infinite.
    NonFiniteNumber,
    /// A normalization scale is not positive.
    InvalidScale,
    /// A normalization clamp is not positive.
    InvalidClip,
}

impl fmt::Display for ValidationCode {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        let code = match self {
            Self::UnsupportedSchema => "unsupported_schema",
            Self::BlankName => "blank_name",
            Self::NameTooLong => "name_too_long",
            Self::UnsafeJsonInteger => "unsafe_json_integer",
            Self::EmptyCategories => "empty_categories",
            Self::TooManyCategories => "too_many_categories",
            Self::InvalidFeatureRange => "invalid_feature_range",
            Self::InvalidProbability => "invalid_probability",
            Self::InvalidHotspotCategory => "invalid_hotspot_category",
            Self::NonFiniteNumber => "non_finite_number",
            Self::InvalidScale => "invalid_scale",
            Self::InvalidClip => "invalid_clip",
        };
        formatter.write_str(code)
    }
}

/// One semantic problem in a workload manifest.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ValidationIssue {
    /// Machine-stable issue category.
    pub code: ValidationCode,
    /// Manifest path containing the problem.
    pub path: &'static str,
    /// Human-readable explanation.
    pub message: String,
}

impl ValidationIssue {
    fn new(code: ValidationCode, path: &'static str, message: impl Into<String>) -> Self {
        Self {
            code,
            path,
            message: message.into(),
        }
    }
}

/// All semantic problems discovered in one validation pass.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ValidationErrors {
    issues: Vec<ValidationIssue>,
}

impl ValidationErrors {
    fn new(issues: Vec<ValidationIssue>) -> Self {
        Self { issues }
    }

    /// Return every discovered problem in deterministic order.
    #[must_use]
    pub fn issues(&self) -> &[ValidationIssue] {
        &self.issues
    }

    /// Return the number of discovered problems.
    #[must_use]
    pub fn len(&self) -> usize {
        self.issues.len()
    }

    /// Return whether no problems were discovered.
    #[must_use]
    pub fn is_empty(&self) -> bool {
        self.issues.is_empty()
    }
}

impl fmt::Display for ValidationErrors {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        writeln!(formatter, "{} validation error(s):", self.issues.len())?;
        for issue in &self.issues {
            writeln!(
                formatter,
                "- {} [{}]: {}",
                issue.path, issue.code, issue.message
            )?;
        }
        Ok(())
    }
}

impl Error for ValidationErrors {}

/// Semantic validation independent of JSON parsing.
pub trait Validate {
    /// Check every invariant and return all discovered issues.
    fn validate(&self) -> Result<(), ValidationErrors>;
}

impl Validate for WorkloadSpec {
    fn validate(&self) -> Result<(), ValidationErrors> {
        let mut issues = Vec::new();

        validate_identity(self, &mut issues);
        validate_dataset(&self.dataset, &mut issues);
        validate_pipeline(self, &mut issues);

        finish_validation(issues)
    }
}

impl Validate for DatasetSpec {
    fn validate(&self) -> Result<(), ValidationErrors> {
        let mut issues = Vec::new();
        validate_dataset(self, &mut issues);
        finish_validation(issues)
    }
}

fn finish_validation(issues: Vec<ValidationIssue>) -> Result<(), ValidationErrors> {
    if issues.is_empty() {
        Ok(())
    } else {
        Err(ValidationErrors::new(issues))
    }
}

fn validate_identity(spec: &WorkloadSpec, issues: &mut Vec<ValidationIssue>) {
    if spec.schema_version != WORKLOAD_SCHEMA {
        issues.push(ValidationIssue::new(
            ValidationCode::UnsupportedSchema,
            "schema_version",
            format!(
                "expected {WORKLOAD_SCHEMA:?}, got {:?}",
                spec.schema_version
            ),
        ));
    }

    if spec.name.trim().is_empty() {
        issues.push(ValidationIssue::new(
            ValidationCode::BlankName,
            "name",
            "workload name must contain a non-whitespace character",
        ));
    } else if spec.name.chars().count() > MAX_NAME_CHARS {
        issues.push(ValidationIssue::new(
            ValidationCode::NameTooLong,
            "name",
            format!("workload name must not exceed {MAX_NAME_CHARS} characters"),
        ));
    }
}

fn validate_dataset(dataset: &DatasetSpec, issues: &mut Vec<ValidationIssue>) {
    validate_json_integer(issues, "dataset.record_count", dataset.record_count);
    validate_json_integer(issues, "dataset.seed", dataset.seed);

    if dataset.category_count == 0 {
        issues.push(ValidationIssue::new(
            ValidationCode::EmptyCategories,
            "dataset.category_count",
            "category_count must be greater than zero",
        ));
    } else if dataset.category_count > MAX_CATEGORIES {
        issues.push(ValidationIssue::new(
            ValidationCode::TooManyCategories,
            "dataset.category_count",
            format!("category_count must not exceed {MAX_CATEGORIES}"),
        ));
    }

    if dataset.feature_min >= dataset.feature_max {
        issues.push(ValidationIssue::new(
            ValidationCode::InvalidFeatureRange,
            "dataset.feature_min",
            "feature_min must be less than feature_max",
        ));
    }

    validate_probability(
        issues,
        "dataset.flag_probability_bps",
        dataset.flag_probability_bps,
    );

    if let DistributionSpec::Hotspot {
        category,
        probability_bps,
    } = dataset.distribution
    {
        validate_probability(
            issues,
            "dataset.distribution.probability_bps",
            probability_bps,
        );

        if dataset.category_count > 0 && category >= dataset.category_count {
            issues.push(ValidationIssue::new(
                ValidationCode::InvalidHotspotCategory,
                "dataset.distribution.category",
                format!(
                    "hotspot category {category} is outside category_count {}",
                    dataset.category_count
                ),
            ));
        }
    }
}

fn validate_json_integer(issues: &mut Vec<ValidationIssue>, path: &'static str, value: u64) {
    if value > MAX_SAFE_JSON_INTEGER {
        issues.push(ValidationIssue::new(
            ValidationCode::UnsafeJsonInteger,
            path,
            format!(
                "value must not exceed {MAX_SAFE_JSON_INTEGER} so every JSON consumer preserves it exactly"
            ),
        ));
    }
}

fn validate_pipeline(spec: &WorkloadSpec, issues: &mut Vec<ValidationIssue>) {
    let normalize = &spec.pipeline.normalize;
    validate_finite(issues, "pipeline.normalize.offset_a", normalize.offset_a);
    validate_positive_scale(issues, "pipeline.normalize.scale_a", normalize.scale_a);
    validate_finite(issues, "pipeline.normalize.offset_b", normalize.offset_b);
    validate_positive_scale(issues, "pipeline.normalize.scale_b", normalize.scale_b);

    if validate_finite(issues, "pipeline.normalize.clip", normalize.clip) && normalize.clip <= 0.0 {
        issues.push(ValidationIssue::new(
            ValidationCode::InvalidClip,
            "pipeline.normalize.clip",
            "clip must be greater than zero",
        ));
    }

    let score = &spec.pipeline.score;
    validate_finite(issues, "pipeline.score.weight_a", score.weight_a);
    validate_finite(issues, "pipeline.score.weight_b", score.weight_b);
    validate_finite(issues, "pipeline.score.bias", score.bias);
    validate_finite(issues, "pipeline.score.flag_bonus", score.flag_bonus);
    validate_finite(
        issues,
        "pipeline.filter.min_score",
        spec.pipeline.filter.min_score,
    );
}

fn validate_probability(
    issues: &mut Vec<ValidationIssue>,
    path: &'static str,
    probability_bps: u16,
) {
    if probability_bps > BASIS_POINTS_MAX {
        issues.push(ValidationIssue::new(
            ValidationCode::InvalidProbability,
            path,
            format!("probability must not exceed {BASIS_POINTS_MAX} basis points"),
        ));
    }
}

fn validate_positive_scale(issues: &mut Vec<ValidationIssue>, path: &'static str, value: f32) {
    if validate_finite(issues, path, value) && value <= 0.0 {
        issues.push(ValidationIssue::new(
            ValidationCode::InvalidScale,
            path,
            "normalization scale must be greater than zero",
        ));
    }
}

fn validate_finite(issues: &mut Vec<ValidationIssue>, path: &'static str, value: f32) -> bool {
    if value.is_finite() {
        true
    } else {
        issues.push(ValidationIssue::new(
            ValidationCode::NonFiniteNumber,
            path,
            "value must be finite",
        ));
        false
    }
}
