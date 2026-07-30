package benchmark

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kvetinski/paraflow/labctl-go/internal/buildinfo"
	"github.com/kvetinski/paraflow/labctl-go/internal/doctor"
	"github.com/kvetinski/paraflow/labctl-go/internal/jsoncheck"
	"github.com/kvetinski/paraflow/labctl-go/internal/protocol"
)

const EvidenceVerificationSchema = "paraflow.evidence-verification/v1"

// VerifyOptions identifies an immutable evidence artifact and the repository
// content needed to replay all of its deterministic checks.
type VerifyOptions struct {
	EvidencePath   string
	RepositoryRoot string
	EnginePath     string
	ReadFile       func(string) ([]byte, error)
}

// EvidenceVerification is a machine-readable receipt for one successful
// offline verification. It deliberately contains no fresh timing data.
type EvidenceVerification struct {
	SchemaVersion                string `json:"schema_version"`
	Status                       string `json:"status"`
	EvidencePath                 string `json:"evidence_path"`
	EvidenceSHA256               string `json:"evidence_sha256"`
	EvidenceSchema               string `json:"evidence_schema"`
	SuiteName                    string `json:"suite_name"`
	ExperimentCount              int    `json:"experiment_count"`
	RetainedSampleCount          int    `json:"retained_sample_count"`
	RepositoryIdentitiesVerified int    `json:"repository_identities_verified"`
	EngineArtifactVerified       bool   `json:"engine_artifact_verified"`
}

// VerifyEvidence strictly decodes an existing Day 5 capture or Day 6 profile
// report, re-hashes repository inputs, and derives every summary and analysis
// field again from retained raw samples.
func VerifyEvidence(options VerifyOptions) (EvidenceVerification, error) {
	if strings.TrimSpace(options.EvidencePath) == "" {
		return EvidenceVerification{}, errors.New("evidence path must not be empty")
	}
	if strings.TrimSpace(options.RepositoryRoot) == "" {
		return EvidenceVerification{}, errors.New("repository root must not be empty")
	}

	readFile := options.ReadFile
	if readFile == nil {
		readFile = os.ReadFile
	}
	root, err := filepath.Abs(options.RepositoryRoot)
	if err != nil {
		return EvidenceVerification{}, fmt.Errorf(
			"resolve repository root %q: %w",
			options.RepositoryRoot,
			err,
		)
	}
	evidenceBytes, err := readFile(options.EvidencePath)
	if err != nil {
		return EvidenceVerification{}, fmt.Errorf(
			"read evidence %q: %w",
			options.EvidencePath,
			err,
		)
	}

	var discriminator struct {
		SchemaVersion string `json:"schema_version"`
	}
	if err := jsoncheck.Decode(evidenceBytes, &discriminator, false); err != nil {
		return EvidenceVerification{}, fmt.Errorf("decode evidence discriminator: %w", err)
	}

	var result EvidenceVerification
	switch discriminator.SchemaVersion {
	case CaptureSchema:
		var capture Capture
		if err := jsoncheck.Decode(evidenceBytes, &capture, true); err != nil {
			return EvidenceVerification{}, fmt.Errorf("decode benchmark capture: %w", err)
		}
		result, err = verifyCapture(root, capture, readFile)
	case ScalarProfileReportSchema:
		var report ScalarProfileReport
		if err := jsoncheck.Decode(evidenceBytes, &report, true); err != nil {
			return EvidenceVerification{}, fmt.Errorf("decode scalar profile report: %w", err)
		}
		result, err = verifyProfileReport(root, report, readFile)
	default:
		return EvidenceVerification{}, fmt.Errorf(
			"unsupported evidence schema_version %q",
			discriminator.SchemaVersion,
		)
	}
	if err != nil {
		return EvidenceVerification{}, err
	}

	result.SchemaVersion = EvidenceVerificationSchema
	result.Status = "passed"
	result.EvidencePath = identityPath(root, options.EvidencePath)
	result.EvidenceSHA256 = hashBytes(evidenceBytes)
	result.EvidenceSchema = discriminator.SchemaVersion
	if options.EnginePath != "" {
		actual, err := hashFile(options.EnginePath)
		if err != nil {
			return EvidenceVerification{}, fmt.Errorf(
				"hash engine artifact %q: %w",
				options.EnginePath,
				err,
			)
		}
		expected, err := evidenceEngineSHA(evidenceBytes, discriminator.SchemaVersion)
		if err != nil {
			return EvidenceVerification{}, err
		}
		if actual != expected {
			return EvidenceVerification{}, fmt.Errorf(
				"engine artifact SHA-256 mismatch: got %s, want %s",
				actual,
				expected,
			)
		}
		result.EngineArtifactVerified = true
	}
	return result, nil
}

func verifyCapture(
	root string,
	capture Capture,
	readFile func(string) ([]byte, error),
) (EvidenceVerification, error) {
	if capture.SchemaVersion != CaptureSchema {
		return EvidenceVerification{}, fmt.Errorf(
			"unexpected capture schema_version %q",
			capture.SchemaVersion,
		)
	}
	if err := verifyCommonEvidence(
		capture.StartedAt,
		capture.CompletedAt,
		capture.Suite,
		capture.Controller,
		capture.Environment,
		capture.EngineArtifact,
	); err != nil {
		return EvidenceVerification{}, err
	}

	suite, err := readVerifiedSuite(root, capture.Suite, readFile)
	if err != nil {
		return EvidenceVerification{}, err
	}
	if len(capture.Experiments) != len(suite.Scenarios) {
		return EvidenceVerification{}, fmt.Errorf(
			"experiment count is %d; suite declares %d scenarios",
			len(capture.Experiments),
			len(suite.Scenarios),
		)
	}

	var retained int
	var build *EngineBuild
	ids := make(map[string]struct{}, len(capture.Experiments))
	for index, experiment := range capture.Experiments {
		scenario := suite.Scenarios[index]
		label := fmt.Sprintf("scenario %q", scenario.Name)
		if experiment.ScenarioName != scenario.Name {
			return EvidenceVerification{}, fmt.Errorf(
				"%s: experiment scenario_name is %q",
				label,
				experiment.ScenarioName,
			)
		}
		workloadBytes, projection, err := readVerifiedWorkload(
			root,
			scenario.Workload,
			readFile,
		)
		if err != nil {
			return EvidenceVerification{}, fmt.Errorf("%s: %w", label, err)
		}
		expectedWorkload := WorkloadIdentity{
			Path:          scenario.Workload,
			SHA256:        hashBytes(workloadBytes),
			SchemaVersion: projection.SchemaVersion,
			Name:          projection.Name,
			RecordCount:   projection.RecordCount,
			CategoryCount: projection.CategoryCount,
		}
		if experiment.Workload != expectedWorkload {
			return EvidenceVerification{}, fmt.Errorf(
				"%s: stored workload identity differs from repository content",
				label,
			)
		}

		engineResult := experiment.EngineResult
		if _, duplicate := ids[engineResult.ExperimentID]; duplicate {
			return EvidenceVerification{}, fmt.Errorf(
				"%s: duplicate experiment_id %q",
				label,
				engineResult.ExperimentID,
			)
		}
		ids[engineResult.ExperimentID] = struct{}{}
		request := Request{
			SchemaVersion: RequestSchema,
			ExperimentID:  engineResult.ExperimentID,
			ScenarioName:  scenario.Name,
			Execution:     scenario.Execution,
			Sampling:      scenario.Sampling,
			Workload:      json.RawMessage(workloadBytes),
		}
		if err := validateEngineResult(engineResult, request, projection); err != nil {
			return EvidenceVerification{}, fmt.Errorf(
				"%s: validate raw engine result: %w",
				label,
				err,
			)
		}
		if err := verifyEngineIdentity(engineResult.EngineBuild, capture.Controller); err != nil {
			return EvidenceVerification{}, fmt.Errorf("%s: %w", label, err)
		}
		if err := requireStableEngineBuild(&build, engineResult.EngineBuild); err != nil {
			return EvidenceVerification{}, fmt.Errorf("%s: %w", label, err)
		}
		if experiment.OrchestrationTotalNS.Uint64() <
			engineResult.Timing.ExperimentTotalNS.Uint64() {
			return EvidenceVerification{}, fmt.Errorf(
				"%s: orchestration_total_ns is smaller than experiment_total_ns",
				label,
			)
		}
		summary, err := Summarize(engineResult.Samples)
		if err != nil {
			return EvidenceVerification{}, fmt.Errorf("%s: summarize samples: %w", label, err)
		}
		if experiment.Summary != summary {
			return EvidenceVerification{}, fmt.Errorf(
				"%s: stored summary differs from retained raw samples",
				label,
			)
		}
		retained += len(engineResult.Samples)
	}

	return EvidenceVerification{
		SuiteName:                    suite.Name,
		ExperimentCount:              len(capture.Experiments),
		RetainedSampleCount:          retained,
		RepositoryIdentitiesVerified: 1 + len(capture.Experiments),
	}, nil
}

func verifyProfileReport(
	root string,
	report ScalarProfileReport,
	readFile func(string) ([]byte, error),
) (EvidenceVerification, error) {
	if report.SchemaVersion != ScalarProfileReportSchema {
		return EvidenceVerification{}, fmt.Errorf(
			"unexpected profile report schema_version %q",
			report.SchemaVersion,
		)
	}
	if err := verifyCommonEvidence(
		report.StartedAt,
		report.CompletedAt,
		report.Suite,
		report.Controller,
		report.Environment,
		report.EngineArtifact,
	); err != nil {
		return EvidenceVerification{}, err
	}

	suite, err := readVerifiedSuite(root, report.Suite, readFile)
	if err != nil {
		return EvidenceVerification{}, err
	}
	if len(report.Experiments) != len(suite.Scenarios) {
		return EvidenceVerification{}, fmt.Errorf(
			"experiment count is %d; suite declares %d scenarios",
			len(report.Experiments),
			len(suite.Scenarios),
		)
	}

	var retained int
	var build *EngineBuild
	ids := make(map[string]struct{}, len(report.Experiments)*2)
	for index, experiment := range report.Experiments {
		scenario := suite.Scenarios[index]
		label := fmt.Sprintf("scenario %q", scenario.Name)
		if experiment.ScenarioName != scenario.Name {
			return EvidenceVerification{}, fmt.Errorf(
				"%s: experiment scenario_name is %q",
				label,
				experiment.ScenarioName,
			)
		}
		workloadBytes, projection, err := readVerifiedWorkload(
			root,
			scenario.Workload,
			readFile,
		)
		if err != nil {
			return EvidenceVerification{}, fmt.Errorf("%s: %w", label, err)
		}
		expectedWorkload := ProfileWorkloadIdentity{
			Path:          scenario.Workload,
			SHA256:        hashBytes(workloadBytes),
			SchemaVersion: projection.SchemaVersion,
			Name:          projection.Name,
			RecordCount:   projection.RecordCount,
			CategoryCount: projection.CategoryCount,
			Distribution:  projection.Distribution,
		}
		if experiment.Workload != expectedWorkload {
			return EvidenceVerification{}, fmt.Errorf(
				"%s: stored workload identity differs from repository content",
				label,
			)
		}

		baseline := experiment.Baseline.EngineResult
		profile := experiment.StageProfile.EngineResult
		for _, id := range []string{baseline.ExperimentID, profile.ExperimentID} {
			if _, duplicate := ids[id]; duplicate {
				return EvidenceVerification{}, fmt.Errorf(
					"%s: duplicate experiment_id %q",
					label,
					id,
				)
			}
			ids[id] = struct{}{}
		}
		baselineRequest := Request{
			SchemaVersion: RequestSchema,
			ExperimentID:  baseline.ExperimentID,
			ScenarioName:  scenario.Name,
			Execution:     scenario.Execution,
			Sampling:      scenario.Sampling,
			Workload:      json.RawMessage(workloadBytes),
		}
		if err := validateEngineResult(baseline, baselineRequest, projection); err != nil {
			return EvidenceVerification{}, fmt.Errorf(
				"%s: validate fused baseline: %w",
				label,
				err,
			)
		}
		profileRequest := ProfileRequest{
			SchemaVersion: ProfileRequestSchema,
			ExperimentID:  profile.ExperimentID,
			ScenarioName:  scenario.Name,
			Execution:     scenario.Execution,
			Sampling:      scenario.Sampling,
			Workload:      json.RawMessage(workloadBytes),
		}
		if err := validateProfileEngineResult(profile, profileRequest, projection); err != nil {
			return EvidenceVerification{}, fmt.Errorf(
				"%s: validate stage profile: %w",
				label,
				err,
			)
		}
		if baseline.EngineBuild != profile.EngineBuild {
			return EvidenceVerification{}, fmt.Errorf(
				"%s: baseline and profile engine build identities differ",
				label,
			)
		}
		if err := verifyEngineIdentity(baseline.EngineBuild, report.Controller); err != nil {
			return EvidenceVerification{}, fmt.Errorf("%s: %w", label, err)
		}
		if err := requireStableEngineBuild(&build, baseline.EngineBuild); err != nil {
			return EvidenceVerification{}, fmt.Errorf("%s: %w", label, err)
		}
		if experiment.Baseline.OrchestrationTotalNS.Uint64() <
			baseline.Timing.ExperimentTotalNS.Uint64() {
			return EvidenceVerification{}, fmt.Errorf(
				"%s: baseline orchestration_total_ns is smaller than experiment_total_ns",
				label,
			)
		}
		if experiment.StageProfile.OrchestrationTotalNS.Uint64() <
			profile.Timing.ExperimentTotalNS.Uint64() {
			return EvidenceVerification{}, fmt.Errorf(
				"%s: profile orchestration_total_ns is smaller than experiment_total_ns",
				label,
			)
		}

		baselineSummary, err := Summarize(baseline.Samples)
		if err != nil {
			return EvidenceVerification{}, fmt.Errorf(
				"%s: summarize fused samples: %w",
				label,
				err,
			)
		}
		if experiment.Baseline.Summary != baselineSummary {
			return EvidenceVerification{}, fmt.Errorf(
				"%s: stored fused summary differs from retained raw samples",
				label,
			)
		}
		profileSummary, err := SummarizeProfile(profile.Samples)
		if err != nil {
			return EvidenceVerification{}, fmt.Errorf(
				"%s: summarize profile samples: %w",
				label,
				err,
			)
		}
		if experiment.StageProfile.Summary != profileSummary {
			return EvidenceVerification{}, fmt.Errorf(
				"%s: stored profile summary differs from retained raw samples",
				label,
			)
		}
		analysis, err := AnalyzeScenario(
			projection,
			baseline,
			baselineSummary,
			profile,
			profileSummary,
		)
		if err != nil {
			return EvidenceVerification{}, fmt.Errorf(
				"%s: recompute analysis: %w",
				label,
				err,
			)
		}
		if experiment.Analysis != analysis {
			return EvidenceVerification{}, fmt.Errorf(
				"%s: stored analysis differs from retained raw samples",
				label,
			)
		}
		retained += len(baseline.Samples) + len(profile.Samples)
	}

	return EvidenceVerification{
		SuiteName:                    suite.Name,
		ExperimentCount:              len(report.Experiments),
		RetainedSampleCount:          retained,
		RepositoryIdentitiesVerified: 1 + len(report.Experiments),
	}, nil
}

func verifyCommonEvidence(
	startedAt time.Time,
	completedAt time.Time,
	suiteIdentity SuiteIdentity,
	controller buildinfo.Info,
	environment doctor.Report,
	engine Artifact,
) error {
	if startedAt.IsZero() || completedAt.IsZero() {
		return errors.New("evidence timestamps must not be zero")
	}
	if completedAt.Before(startedAt) {
		return errors.New("evidence completion timestamp precedes its start")
	}
	if strings.TrimSpace(controller.Version) == "" ||
		strings.TrimSpace(controller.FullCommit) == "" ||
		!validSourceState(controller.SourceState) {
		return errors.New("controller build identity is incomplete")
	}
	if environment.SchemaVersion != "paraflow.environment/v3" {
		return fmt.Errorf(
			"unexpected environment schema_version %q",
			environment.SchemaVersion,
		)
	}
	if environment.Source != controller {
		return errors.New("environment source identity differs from controller identity")
	}
	if environment.Milestone != "day-05" &&
		environment.Milestone != "day-06" &&
		environment.Milestone != "day-07" {
		return fmt.Errorf(
			"unexpected evidence environment milestone %q",
			environment.Milestone,
		)
	}
	if !environment.Ready {
		return errors.New("environment was not ready when evidence was captured")
	}
	if environment.CapturedAt.IsZero() {
		return errors.New("environment capture timestamp must not be zero")
	}
	if strings.TrimSpace(environment.OS) == "" ||
		strings.TrimSpace(environment.Architecture) == "" ||
		strings.TrimSpace(environment.GoVersion) == "" ||
		environment.LogicalCPUs < 1 ||
		environment.GoMaxProcs < 1 ||
		len(environment.Tools) == 0 {
		return errors.New("environment identity is incomplete")
	}
	if !validRepositoryJSONPath(suiteIdentity.Path) {
		return fmt.Errorf("suite path %q is not a repository-relative JSON path", suiteIdentity.Path)
	}
	if !validSHA256(suiteIdentity.SHA256) {
		return errors.New("suite SHA-256 is not canonical lowercase hexadecimal")
	}
	if strings.TrimSpace(engine.Path) == "" || !validSHA256(engine.SHA256) {
		return errors.New("engine artifact identity is incomplete")
	}
	return nil
}

func readVerifiedSuite(
	root string,
	identity SuiteIdentity,
	readFile func(string) ([]byte, error),
) (Suite, error) {
	suitePath, err := resolveVerifiedRepositoryFile(root, identity.Path)
	if err != nil {
		return Suite{}, fmt.Errorf("resolve suite identity: %w", err)
	}
	suiteBytes, err := readFile(suitePath)
	if err != nil {
		return Suite{}, fmt.Errorf("read suite %q: %w", identity.Path, err)
	}
	if hashBytes(suiteBytes) != identity.SHA256 {
		return Suite{}, fmt.Errorf("suite %q SHA-256 mismatch", identity.Path)
	}
	var suite Suite
	if err := jsoncheck.Decode(suiteBytes, &suite, true); err != nil {
		return Suite{}, fmt.Errorf("decode suite %q: %w", identity.Path, err)
	}
	if err := suite.Validate(); err != nil {
		return Suite{}, fmt.Errorf("validate suite %q: %w", identity.Path, err)
	}
	if suite.SchemaVersion != identity.SchemaVersion || suite.Name != identity.Name {
		return Suite{}, fmt.Errorf("suite %q identity fields differ", identity.Path)
	}
	return suite, nil
}

func readVerifiedWorkload(
	root string,
	repositoryPath string,
	readFile func(string) ([]byte, error),
) ([]byte, protocol.WorkloadProjection, error) {
	workloadPath, err := resolveVerifiedRepositoryFile(root, repositoryPath)
	if err != nil {
		return nil, protocol.WorkloadProjection{}, err
	}
	workloadBytes, err := readFile(workloadPath)
	if err != nil {
		return nil, protocol.WorkloadProjection{}, fmt.Errorf(
			"read workload %q: %w",
			repositoryPath,
			err,
		)
	}
	projection, err := protocol.ProjectWorkload(workloadBytes)
	if err != nil {
		return nil, protocol.WorkloadProjection{}, fmt.Errorf(
			"inspect workload %q: %w",
			repositoryPath,
			err,
		)
	}
	if projection.SchemaVersion != "paraflow.workload/v1" {
		return nil, protocol.WorkloadProjection{}, fmt.Errorf(
			"workload %q uses unsupported schema_version %q",
			repositoryPath,
			projection.SchemaVersion,
		)
	}
	return workloadBytes, projection, nil
}

func resolveVerifiedRepositoryFile(root, repositoryPath string) (string, error) {
	candidate, err := resolveRepositoryPath(root, repositoryPath)
	if err != nil {
		return "", err
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve repository root symlinks: %w", err)
	}
	resolvedCandidate, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve repository path %q symlinks: %w", repositoryPath, err)
	}
	relative, err := filepath.Rel(resolvedRoot, resolvedCandidate)
	if err != nil {
		return "", fmt.Errorf(
			"compare resolved repository path %q with root: %w",
			repositoryPath,
			err,
		)
	}
	if relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf(
			"repository path %q resolves outside the repository root",
			repositoryPath,
		)
	}
	return resolvedCandidate, nil
}

func verifyEngineIdentity(engine EngineBuild, controller buildinfo.Info) error {
	if err := validateSourceAlignment(engine, controller); err != nil {
		return err
	}
	if engine.Version != controller.Version {
		return fmt.Errorf(
			"engine version %q does not match controller version %q",
			engine.Version,
			controller.Version,
		)
	}
	return nil
}

func requireStableEngineBuild(expected **EngineBuild, actual EngineBuild) error {
	if *expected == nil {
		copy := actual
		*expected = &copy
		return nil
	}
	if **expected != actual {
		return errors.New("engine build identity changed between scenarios")
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, digit := range value {
		if !((digit >= '0' && digit <= '9') || (digit >= 'a' && digit <= 'f')) {
			return false
		}
	}
	return true
}

func evidenceEngineSHA(data []byte, schema string) (string, error) {
	var identity struct {
		EngineArtifact Artifact `json:"engine_artifact"`
	}
	if err := jsoncheck.Decode(data, &identity, false); err != nil {
		return "", fmt.Errorf("decode %s engine identity: %w", schema, err)
	}
	if !validSHA256(identity.EngineArtifact.SHA256) {
		return "", errors.New("evidence engine artifact SHA-256 is invalid")
	}
	return identity.EngineArtifact.SHA256, nil
}
