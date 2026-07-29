package benchmark

import (
	"context"
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

// ProfileOptions configures one complete paired scalar baseline/profile report.
type ProfileOptions struct {
	EnginePath     string
	SuitePath      string
	OutputPath     string
	RepositoryRoot string
	Build          buildinfo.Info
	Probe          doctor.Probe
	ReadFile       func(string) ([]byte, error)
	BaselineRunner EngineRunner
	ProfileRunner  ProfileEngineRunner
	Now            func() time.Time
	Persist        func(string, ScalarProfileReport) error
}

// ExecuteProfile pairs the unchanged Day 5 fused baseline with a separate
// stage-pass profile for every scenario and persists one immutable report.
func ExecuteProfile(
	ctx context.Context,
	options ProfileOptions,
) (ScalarProfileReport, error) {
	if err := validateProfileOptions(options); err != nil {
		return ScalarProfileReport{}, err
	}
	if err := ctx.Err(); err != nil {
		return ScalarProfileReport{}, err
	}

	readFile := options.ReadFile
	if readFile == nil {
		readFile = os.ReadFile
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	probe := options.Probe
	if probe == nil {
		probe = doctor.CommandProbe
	}
	baselineRunner := options.BaselineRunner
	if baselineRunner == nil {
		baselineRunner = RunEngine
	}
	profileRunner := options.ProfileRunner
	if profileRunner == nil {
		profileRunner = RunProfileEngine
	}
	persist := options.Persist
	if persist == nil {
		persist = PersistScalarProfileReport
		if err := requireAbsent(options.OutputPath); err != nil {
			return ScalarProfileReport{}, err
		}
	}

	startedAt := now().UTC()
	suiteBytes, err := readFile(options.SuitePath)
	if err != nil {
		return ScalarProfileReport{}, fmt.Errorf(
			"read profile suite %q: %w",
			options.SuitePath,
			err,
		)
	}
	var suite Suite
	if err := jsoncheck.Decode(suiteBytes, &suite, true); err != nil {
		return ScalarProfileReport{}, fmt.Errorf(
			"decode profile suite %q: %w",
			options.SuitePath,
			err,
		)
	}
	if err := suite.Validate(); err != nil {
		return ScalarProfileReport{}, fmt.Errorf(
			"validate profile suite %q: %w",
			options.SuitePath,
			err,
		)
	}

	root, err := filepath.Abs(options.RepositoryRoot)
	if err != nil {
		return ScalarProfileReport{}, fmt.Errorf(
			"resolve repository root %q: %w",
			options.RepositoryRoot,
			err,
		)
	}
	engineHash, err := hashFile(options.EnginePath)
	if err != nil {
		return ScalarProfileReport{}, fmt.Errorf(
			"hash engine artifact %q: %w",
			options.EnginePath,
			err,
		)
	}

	environment := doctor.Check(ctx, probe, options.Build)
	if !environment.Ready {
		return ScalarProfileReport{}, errors.New(
			"host is not ready for Day 6; run 'labctl doctor' and resolve required tool failures",
		)
	}

	experiments := make([]ProfileExperiment, 0, len(suite.Scenarios))
	for index, scenario := range suite.Scenarios {
		if err := ctx.Err(); err != nil {
			return ScalarProfileReport{}, err
		}
		workloadPath, err := resolveRepositoryPath(root, scenario.Workload)
		if err != nil {
			return ScalarProfileReport{}, fmt.Errorf("scenario %q: %w", scenario.Name, err)
		}
		workloadBytes, err := readFile(workloadPath)
		if err != nil {
			return ScalarProfileReport{}, fmt.Errorf(
				"scenario %q: read workload %q: %w",
				scenario.Name,
				scenario.Workload,
				err,
			)
		}
		projection, err := protocol.ProjectWorkload(workloadBytes)
		if err != nil {
			return ScalarProfileReport{}, fmt.Errorf(
				"scenario %q: inspect workload %q: %w",
				scenario.Name,
				scenario.Workload,
				err,
			)
		}
		if projection.SchemaVersion != "paraflow.workload/v1" {
			return ScalarProfileReport{}, fmt.Errorf(
				"scenario %q: unsupported workload schema_version %q",
				scenario.Name,
				projection.SchemaVersion,
			)
		}
		if projection.Distribution != "uniform" && projection.Distribution != "hotspot" {
			return ScalarProfileReport{}, fmt.Errorf(
				"scenario %q: unsupported or missing distribution %q",
				scenario.Name,
				projection.Distribution,
			)
		}

		baselineRequest := Request{
			SchemaVersion: RequestSchema,
			ExperimentID:  fmt.Sprintf("day06:baseline:%016x", index+1),
			ScenarioName:  scenario.Name,
			Execution:     scenario.Execution,
			Sampling:      scenario.Sampling,
			Workload:      append([]byte(nil), workloadBytes...),
		}
		baselineResult, baselineOrchestration, err := baselineRunner(
			ctx,
			options.EnginePath,
			baselineRequest,
			projection,
		)
		if err != nil {
			return ScalarProfileReport{}, fmt.Errorf(
				"scenario %q fused baseline: %w",
				scenario.Name,
				err,
			)
		}
		if err := validateSourceAlignment(baselineResult.EngineBuild, options.Build); err != nil {
			return ScalarProfileReport{}, fmt.Errorf(
				"scenario %q fused baseline: %w",
				scenario.Name,
				err,
			)
		}

		profileRequest := ProfileRequest{
			SchemaVersion: ProfileRequestSchema,
			ExperimentID:  fmt.Sprintf("day06:profile:%016x", index+1),
			ScenarioName:  scenario.Name,
			Execution:     scenario.Execution,
			Sampling:      scenario.Sampling,
			Workload:      append([]byte(nil), workloadBytes...),
		}
		profileResult, profileOrchestration, err := profileRunner(
			ctx,
			options.EnginePath,
			profileRequest,
			projection,
		)
		if err != nil {
			return ScalarProfileReport{}, fmt.Errorf(
				"scenario %q stage profile: %w",
				scenario.Name,
				err,
			)
		}
		if err := validateSourceAlignment(profileResult.EngineBuild, options.Build); err != nil {
			return ScalarProfileReport{}, fmt.Errorf(
				"scenario %q stage profile: %w",
				scenario.Name,
				err,
			)
		}
		if profileResult.EngineBuild != baselineResult.EngineBuild {
			return ScalarProfileReport{}, fmt.Errorf(
				"scenario %q baseline and profile engine build identities differ",
				scenario.Name,
			)
		}

		baselineSummary, err := Summarize(baselineResult.Samples)
		if err != nil {
			return ScalarProfileReport{}, fmt.Errorf(
				"scenario %q summarize fused samples: %w",
				scenario.Name,
				err,
			)
		}
		profileSummary, err := SummarizeProfile(profileResult.Samples)
		if err != nil {
			return ScalarProfileReport{}, fmt.Errorf(
				"scenario %q summarize profile samples: %w",
				scenario.Name,
				err,
			)
		}
		analysis, err := AnalyzeScenario(
			projection,
			baselineResult,
			baselineSummary,
			profileResult,
			profileSummary,
		)
		if err != nil {
			return ScalarProfileReport{}, fmt.Errorf(
				"scenario %q analyze profile: %w",
				scenario.Name,
				err,
			)
		}

		experiments = append(experiments, ProfileExperiment{
			ScenarioName: scenario.Name,
			Workload: ProfileWorkloadIdentity{
				Path:          scenario.Workload,
				SHA256:        hashBytes(workloadBytes),
				SchemaVersion: projection.SchemaVersion,
				Name:          projection.Name,
				RecordCount:   projection.RecordCount,
				CategoryCount: projection.CategoryCount,
				Distribution:  projection.Distribution,
			},
			Baseline: BaselineEvidence{
				OrchestrationTotalNS: baselineOrchestration,
				EngineResult:         baselineResult,
				Summary:              baselineSummary,
			},
			StageProfile: StageProfileEvidence{
				OrchestrationTotalNS: profileOrchestration,
				EngineResult:         profileResult,
				Summary:              profileSummary,
			},
			Analysis: analysis,
		})
	}

	finalEngineHash, err := hashFile(options.EnginePath)
	if err != nil {
		return ScalarProfileReport{}, fmt.Errorf(
			"rehash engine artifact %q: %w",
			options.EnginePath,
			err,
		)
	}
	if finalEngineHash != engineHash {
		return ScalarProfileReport{}, errors.New(
			"engine artifact changed while the profile suite was running",
		)
	}

	report := ScalarProfileReport{
		SchemaVersion: ScalarProfileReportSchema,
		StartedAt:     startedAt,
		CompletedAt:   now().UTC(),
		Suite: SuiteIdentity{
			Path:          identityPath(root, options.SuitePath),
			SHA256:        hashBytes(suiteBytes),
			SchemaVersion: suite.SchemaVersion,
			Name:          suite.Name,
		},
		Controller:  options.Build,
		Environment: environment,
		EngineArtifact: Artifact{
			Path:   identityPath(root, options.EnginePath),
			SHA256: engineHash,
		},
		Experiments: experiments,
	}
	if report.CompletedAt.Before(report.StartedAt) {
		return ScalarProfileReport{}, errors.New(
			"profile capture clock moved backwards between start and completion",
		)
	}
	if err := persist(options.OutputPath, report); err != nil {
		return ScalarProfileReport{}, fmt.Errorf(
			"persist scalar profile report %q: %w",
			options.OutputPath,
			err,
		)
	}
	return report, nil
}

func validateProfileOptions(options ProfileOptions) error {
	required := []struct {
		name  string
		value string
	}{
		{name: "engine path", value: options.EnginePath},
		{name: "suite path", value: options.SuitePath},
		{name: "output path", value: options.OutputPath},
		{name: "repository root", value: options.RepositoryRoot},
	}
	for _, field := range required {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%s must not be empty", field.name)
		}
	}
	if strings.TrimSpace(options.Build.Version) == "" ||
		strings.TrimSpace(options.Build.FullCommit) == "" ||
		!validSourceState(options.Build.SourceState) {
		return errors.New("controller build identity is incomplete")
	}
	return nil
}
