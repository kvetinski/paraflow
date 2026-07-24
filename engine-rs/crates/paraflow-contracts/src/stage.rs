use std::fmt;

/// A logical stage in the stable ParaFlow workload.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Stage {
    /// Create a deterministic logical record batch.
    Generate,
    /// Transform raw features into bounded floating-point values.
    Normalize,
    /// Calculate a score for each record.
    Score,
    /// Select and compact qualifying records.
    Filter,
    /// Reduce selected records into final results.
    Aggregate,
}

impl Stage {
    /// Return the stable contract spelling for this stage.
    #[must_use]
    pub const fn as_str(self) -> &'static str {
        match self {
            Self::Generate => "generate",
            Self::Normalize => "normalize",
            Self::Score => "score",
            Self::Filter => "filter",
            Self::Aggregate => "aggregate",
        }
    }
}

impl fmt::Display for Stage {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.write_str(self.as_str())
    }
}

/// The pipeline order all ParaFlow backends must preserve.
pub const PIPELINE_STAGES: [Stage; 5] = [
    Stage::Generate,
    Stage::Normalize,
    Stage::Score,
    Stage::Filter,
    Stage::Aggregate,
];
