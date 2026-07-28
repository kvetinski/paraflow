// Package benchmark orchestrates reproducible ParaFlow benchmark suites.
//
// The Rust engine owns the measured compute path. This package owns suite
// loading, process orchestration, strict response validation, descriptive
// statistics, machine/source identity, and durable raw-result persistence.
package benchmark

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/kvetinski/paraflow/labctl-go/internal/buildinfo"
	"github.com/kvetinski/paraflow/labctl-go/internal/doctor"
)

const (
	SuiteSchema        = "paraflow.benchmark-suite/v1"
	RequestSchema      = "paraflow.benchmark-request/v1"
	EngineResultSchema = "paraflow.benchmark-engine-result/v1"
	CaptureSchema      = "paraflow.benchmark-capture/v1"

	BenchmarkClock      = "std::time::Instant"
	BenchmarkTimeUnit   = "nanoseconds"
	BenchmarkOracle     = "rust-scalar-v1"
	BenchmarkComparison = "exact"

	maxWarmupIterations = uint32(1_000)
	maxSampleIterations = uint32(10_000)
	maxScenarios        = 64
	maxNameRunes        = 120
)

// Nanoseconds is a duration encoded as one canonical full-width hexadecimal
// u64 string. JSON numbers are intentionally avoided so every consumer keeps
// exact timing values.
type Nanoseconds uint64

// Uint64 returns the underlying nanosecond count.
func (value Nanoseconds) Uint64() uint64 {
	return uint64(value)
}

// MarshalJSON emits 0x followed by exactly sixteen lowercase hex digits.
func (value Nanoseconds) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"0x%016x\"", uint64(value))), nil
}

// UnmarshalJSON accepts only the canonical full-width lowercase form.
func (value *Nanoseconds) UnmarshalJSON(data []byte) error {
	var encoded string
	if err := json.Unmarshal(data, &encoded); err != nil {
		return errors.New("nanoseconds must be a JSON string")
	}
	parsed, err := parseCanonicalHex(encoded)
	if err != nil {
		return fmt.Errorf("invalid nanoseconds: %w", err)
	}
	*value = Nanoseconds(parsed)
	return nil
}

// Execution selects one implementation without changing workload semantics.
type Execution struct {
	Backend string `json:"backend"`
}

// Sampling controls warm-up and retained sample counts.
type Sampling struct {
	WarmupIterations uint32 `json:"warmup_iterations"`
	SampleIterations uint32 `json:"sample_iterations"`
}

// Suite is one versioned collection of benchmark scenarios.
type Suite struct {
	SchemaURI     *string    `json:"$schema,omitempty"`
	SchemaVersion string     `json:"schema_version"`
	Name          string     `json:"name"`
	Description   string     `json:"description,omitempty"`
	Scenarios     []Scenario `json:"scenarios"`
}

// Scenario links one workload to execution and measurement settings.
type Scenario struct {
	Name      string    `json:"name"`
	Workload  string    `json:"workload"`
	Execution Execution `json:"execution"`
	Sampling  Sampling  `json:"sampling"`
}

// Validate checks suite invariants not delegated to the Rust workload parser.
func (suite Suite) Validate() error {
	if suite.SchemaVersion != SuiteSchema {
		return fmt.Errorf("unsupported suite schema_version %q", suite.SchemaVersion)
	}
	if !validName(suite.Name) {
		return errors.New("suite name must contain 1..120 characters and non-whitespace text")
	}
	if suite.Description != "" {
		if len([]rune(suite.Description)) > 500 || strings.TrimSpace(suite.Description) == "" {
			return errors.New("suite description must contain 1..500 characters and non-whitespace text")
		}
	}
	if len(suite.Scenarios) == 0 || len(suite.Scenarios) > maxScenarios {
		return fmt.Errorf("suite must contain 1..%d scenarios", maxScenarios)
	}

	seen := make(map[string]struct{}, len(suite.Scenarios))
	for index, scenario := range suite.Scenarios {
		if err := scenario.Validate(); err != nil {
			return fmt.Errorf("scenario %d: %w", index, err)
		}
		if _, duplicate := seen[scenario.Name]; duplicate {
			return fmt.Errorf("scenario %d duplicates name %q", index, scenario.Name)
		}
		seen[scenario.Name] = struct{}{}
	}
	return nil
}

// Validate checks one scenario's execution and measurement settings.
func (scenario Scenario) Validate() error {
	if !validName(scenario.Name) {
		return errors.New("name must contain 1..120 characters and non-whitespace text")
	}
	if !validRepositoryJSONPath(scenario.Workload) {
		return fmt.Errorf("workload path %q must be a normalized repository-relative .json path", scenario.Workload)
	}
	if scenario.Execution.Backend != "scalar" {
		return fmt.Errorf("unsupported backend %q; expected scalar", scenario.Execution.Backend)
	}
	if scenario.Sampling.WarmupIterations > maxWarmupIterations {
		return fmt.Errorf(
			"warmup_iterations %d exceeds maximum %d",
			scenario.Sampling.WarmupIterations,
			maxWarmupIterations,
		)
	}
	if scenario.Sampling.SampleIterations == 0 ||
		scenario.Sampling.SampleIterations > maxSampleIterations {
		return fmt.Errorf(
			"sample_iterations must be in 1..=%d",
			maxSampleIterations,
		)
	}
	return nil
}

// Request is one self-contained experiment sent to a fresh Rust process.
type Request struct {
	SchemaVersion string          `json:"schema_version"`
	ExperimentID  string          `json:"experiment_id"`
	ScenarioName  string          `json:"scenario_name"`
	Execution     Execution       `json:"execution"`
	Sampling      Sampling        `json:"sampling"`
	Workload      json.RawMessage `json:"workload"`
}

// Sample is one retained raw engine-side measurement.
type Sample struct {
	Ordinal       uint32      `json:"ordinal"`
	GenerationNS  Nanoseconds `json:"generation_ns"`
	PipelineNS    Nanoseconds `json:"pipeline_ns"`
	EngineTotalNS Nanoseconds `json:"engine_total_ns"`
}

// Timing defines the clock and complete engine experiment boundary.
type Timing struct {
	Clock                 string      `json:"clock"`
	Unit                  string      `json:"unit"`
	ProcessStartInSamples bool        `json:"process_start_in_samples"`
	ExperimentTotalNS     Nanoseconds `json:"experiment_total_ns"`
}

// Correctness records the reference and comparison policy used by the engine.
type Correctness struct {
	Status     string `json:"status"`
	Oracle     string `json:"oracle"`
	Comparison string `json:"comparison"`
}

// EngineBuild identifies the measured Rust binary.
type EngineBuild struct {
	Version      string `json:"version"`
	Profile      string `json:"profile"`
	Target       string `json:"target"`
	Rustc        string `json:"rustc"`
	SourceCommit string `json:"source_commit"`
	SourceState  string `json:"source_state"`
}

// EngineResult is the strict raw response from one Rust benchmark process.
type EngineResult struct {
	SchemaVersion string          `json:"schema_version"`
	ExperimentID  string          `json:"experiment_id"`
	ScenarioName  string          `json:"scenario_name"`
	WorkloadName  string          `json:"workload_name"`
	Execution     Execution       `json:"execution"`
	Sampling      Sampling        `json:"sampling"`
	Timing        Timing          `json:"timing"`
	Correctness   Correctness     `json:"correctness"`
	EngineBuild   EngineBuild     `json:"engine_build"`
	Samples       []Sample        `json:"samples"`
	Result        json.RawMessage `json:"result"`
}

// Statistics is a compact descriptive summary derived from retained raw data.
type Statistics struct {
	Count                     int         `json:"count"`
	MinimumNS                 Nanoseconds `json:"minimum_ns"`
	MedianNS                  Nanoseconds `json:"median_ns"`
	MedianAbsoluteDeviationNS Nanoseconds `json:"median_absolute_deviation_ns"`
	MaximumNS                 Nanoseconds `json:"maximum_ns"`
}

// Summary groups descriptive statistics for each declared engine boundary.
type Summary struct {
	Generation  Statistics `json:"generation"`
	Pipeline    Statistics `json:"pipeline"`
	EngineTotal Statistics `json:"engine_total"`
}

// Artifact identifies one file by path and exact SHA-256 digest.
type Artifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// SuiteIdentity identifies the exact suite bytes used by one capture.
type SuiteIdentity struct {
	Path          string `json:"path"`
	SHA256        string `json:"sha256"`
	SchemaVersion string `json:"schema_version"`
	Name          string `json:"name"`
}

// WorkloadIdentity records the exact workload bytes and projected dimensions.
type WorkloadIdentity struct {
	Path          string `json:"path"`
	SHA256        string `json:"sha256"`
	SchemaVersion string `json:"schema_version"`
	Name          string `json:"name"`
	RecordCount   uint64 `json:"record_count"`
	CategoryCount uint64 `json:"category_count"`
}

// Experiment is one fully validated scenario result inside a capture.
type Experiment struct {
	ScenarioName         string           `json:"scenario_name"`
	Workload             WorkloadIdentity `json:"workload"`
	OrchestrationTotalNS Nanoseconds      `json:"orchestration_total_ns"`
	EngineResult         EngineResult     `json:"engine_result"`
	Summary              Summary          `json:"summary"`
}

// Capture is the durable Day 5 benchmark evidence artifact.
type Capture struct {
	SchemaVersion  string         `json:"schema_version"`
	StartedAt      time.Time      `json:"started_at"`
	CompletedAt    time.Time      `json:"completed_at"`
	Suite          SuiteIdentity  `json:"suite"`
	Controller     buildinfo.Info `json:"controller"`
	Environment    doctor.Report  `json:"environment"`
	EngineArtifact Artifact       `json:"engine_artifact"`
	Experiments    []Experiment   `json:"experiments"`
}

func parseCanonicalHex(encoded string) (uint64, error) {
	if len(encoded) != 18 || !strings.HasPrefix(encoded, "0x") {
		return 0, errors.New("must use 0x followed by exactly 16 lowercase hex digits")
	}
	for _, digit := range encoded[2:] {
		if !((digit >= '0' && digit <= '9') || (digit >= 'a' && digit <= 'f')) {
			return 0, errors.New("must use 0x followed by exactly 16 lowercase hex digits")
		}
	}
	value, err := strconv.ParseUint(encoded[2:], 16, 64)
	if err != nil {
		return 0, fmt.Errorf("parse hexadecimal value: %w", err)
	}
	return value, nil
}

func validName(value string) bool {
	length := len([]rune(value))
	return length > 0 && length <= maxNameRunes && strings.TrimSpace(value) != ""
}

func validRepositoryJSONPath(value string) bool {
	if value == "" || len([]rune(value)) > 512 || strings.Contains(value, "\\") || strings.HasPrefix(value, "/") {
		return false
	}
	if path.Clean(value) != value || path.Ext(value) != ".json" {
		return false
	}
	for _, component := range strings.Split(value, "/") {
		if component == ".." || component == "." || component == "" {
			return false
		}
	}
	return true
}

func rawPresent(value json.RawMessage) bool {
	trimmed := bytes.TrimSpace(value)
	return len(trimmed) != 0 && !bytes.Equal(trimmed, []byte("null"))
}
