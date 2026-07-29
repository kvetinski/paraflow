package benchmark

import (
	"encoding/json"
	"time"

	"github.com/kvetinski/paraflow/labctl-go/internal/buildinfo"
	"github.com/kvetinski/paraflow/labctl-go/internal/doctor"
)

const (
	ProfileRequestSchema      = "paraflow.profile-request/v1"
	ProfileEngineResultSchema = "paraflow.profile-engine-result/v1"
	ScalarProfileReportSchema = "paraflow.scalar-profile-report/v1"

	ProfileTopology = "materialized-stage-passes-v1"
	ProfileObserver = "boundary-timers-v1"
)

// ProfileRequest asks a fresh Rust process to collect diagnostic stage-pass
// samples for one workload.
type ProfileRequest struct {
	SchemaVersion string          `json:"schema_version"`
	ExperimentID  string          `json:"experiment_id"`
	ScenarioName  string          `json:"scenario_name"`
	Execution     Execution       `json:"execution"`
	Sampling      Sampling        `json:"sampling"`
	Workload      json.RawMessage `json:"workload"`
}

// ProfileSample is one retained raw diagnostic stage-pass measurement.
type ProfileSample struct {
	Ordinal        uint32      `json:"ordinal"`
	GenerationNS   Nanoseconds `json:"generation_ns"`
	NormalizeNS    Nanoseconds `json:"normalize_ns"`
	ScoreNS        Nanoseconds `json:"score_ns"`
	FilterNS       Nanoseconds `json:"filter_ns"`
	AggregateNS    Nanoseconds `json:"aggregate_ns"`
	StageSumNS     Nanoseconds `json:"stage_sum_ns"`
	ProfileTotalNS Nanoseconds `json:"profile_total_ns"`
}

// ProfileTiming defines the diagnostic topology and clock used by Rust.
type ProfileTiming struct {
	Clock                 string      `json:"clock"`
	Unit                  string      `json:"unit"`
	ProcessStartInSamples bool        `json:"process_start_in_samples"`
	Topology              string      `json:"topology"`
	Observer              string      `json:"observer"`
	ExperimentTotalNS     Nanoseconds `json:"experiment_total_ns"`
}

// ProfileEngineResult is the strict raw response from one Rust profile process.
type ProfileEngineResult struct {
	SchemaVersion string          `json:"schema_version"`
	ExperimentID  string          `json:"experiment_id"`
	ScenarioName  string          `json:"scenario_name"`
	WorkloadName  string          `json:"workload_name"`
	Execution     Execution       `json:"execution"`
	Sampling      Sampling        `json:"sampling"`
	Timing        ProfileTiming   `json:"timing"`
	Correctness   Correctness     `json:"correctness"`
	EngineBuild   EngineBuild     `json:"engine_build"`
	Samples       []ProfileSample `json:"samples"`
	Result        json.RawMessage `json:"result"`
}

// ProfileSummary derives descriptive statistics for every declared stage and
// for the complete diagnostic boundary.
type ProfileSummary struct {
	Generation     Statistics `json:"generation"`
	Normalize      Statistics `json:"normalize"`
	Score          Statistics `json:"score"`
	Filter         Statistics `json:"filter"`
	Aggregate      Statistics `json:"aggregate"`
	StageSum       Statistics `json:"stage_sum"`
	PipelineStages Statistics `json:"pipeline_stages"`
	ProfileTotal   Statistics `json:"profile_total"`
}

// ProfileWorkloadIdentity records the exact workload and the dimensions needed
// to interpret per-record and distribution-sensitive costs.
type ProfileWorkloadIdentity struct {
	Path          string `json:"path"`
	SHA256        string `json:"sha256"`
	SchemaVersion string `json:"schema_version"`
	Name          string `json:"name"`
	RecordCount   uint64 `json:"record_count"`
	CategoryCount uint64 `json:"category_count"`
	Distribution  string `json:"distribution"`
}

// StageSharesBPS apportions the sum of stage medians across all five observed
// stages. Values always sum to exactly 10,000 basis points.
type StageSharesBPS struct {
	Generation uint32 `json:"generation"`
	Normalize  uint32 `json:"normalize"`
	Score      uint32 `json:"score"`
	Filter     uint32 `json:"filter"`
	Aggregate  uint32 `json:"aggregate"`
}

// ScenarioProfileAnalysis contains integer-only, reproducible interpretation
// of one fused baseline and one diagnostic stage profile.
type ScenarioProfileAnalysis struct {
	AcceptedCount                           uint64         `json:"accepted_count"`
	SelectivityBPS                          uint32         `json:"selectivity_bps"`
	StageMedianSumNS                        Nanoseconds    `json:"stage_median_sum_ns"`
	StagePipelineMedianSumNS                Nanoseconds    `json:"stage_pipeline_median_sum_ns"`
	StageShareBPS                           StageSharesBPS `json:"stage_share_bps"`
	DominantStage                           string         `json:"dominant_stage"`
	DominantPipelineStage                   string         `json:"dominant_pipeline_stage"`
	FusedPipelineMedianNSPerRecordMilli     uint64         `json:"fused_pipeline_median_ns_per_record_milli"`
	StagePassPipelineMedianNSPerRecordMilli uint64         `json:"stage_pass_pipeline_median_ns_per_record_milli"`
	StagePassToFusedPipelineRatioMilli      uint64         `json:"stage_pass_to_fused_pipeline_ratio_milli"`
	FusedPipelineMedianAbsoluteDeviationBPS uint32         `json:"fused_pipeline_median_absolute_deviation_bps"`
	ProfileTotalMedianAbsoluteDeviationBPS  uint32         `json:"profile_total_median_absolute_deviation_bps"`
}

// BaselineEvidence retains the unchanged Day 5 fused measurements used as the
// denominator for one scenario analysis.
type BaselineEvidence struct {
	OrchestrationTotalNS Nanoseconds  `json:"orchestration_total_ns"`
	EngineResult         EngineResult `json:"engine_result"`
	Summary              Summary      `json:"summary"`
}

// StageProfileEvidence retains the Day 6 diagnostic stage-pass measurements.
type StageProfileEvidence struct {
	OrchestrationTotalNS Nanoseconds         `json:"orchestration_total_ns"`
	EngineResult         ProfileEngineResult `json:"engine_result"`
	Summary              ProfileSummary      `json:"summary"`
}

// ProfileExperiment pairs unchanged fused baseline evidence with a diagnostic
// stage profile for one exact workload.
type ProfileExperiment struct {
	ScenarioName string                  `json:"scenario_name"`
	Workload     ProfileWorkloadIdentity `json:"workload"`
	Baseline     BaselineEvidence        `json:"baseline"`
	StageProfile StageProfileEvidence    `json:"stage_profile"`
	Analysis     ScenarioProfileAnalysis `json:"analysis"`
}

// ScalarProfileReport is the complete immutable Day 6 evidence artifact.
type ScalarProfileReport struct {
	SchemaVersion  string              `json:"schema_version"`
	StartedAt      time.Time           `json:"started_at"`
	CompletedAt    time.Time           `json:"completed_at"`
	Suite          SuiteIdentity       `json:"suite"`
	Controller     buildinfo.Info      `json:"controller"`
	Environment    doctor.Report       `json:"environment"`
	EngineArtifact Artifact            `json:"engine_artifact"`
	Experiments    []ProfileExperiment `json:"experiments"`
}
