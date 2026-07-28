package benchmark

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kvetinski/paraflow/labctl-go/internal/buildinfo"
	"github.com/kvetinski/paraflow/labctl-go/internal/doctor"
	"github.com/kvetinski/paraflow/labctl-go/internal/jsoncheck"
	"github.com/kvetinski/paraflow/labctl-go/internal/protocol"
)

// Options configures one complete benchmark-suite capture.
type Options struct {
	EnginePath     string
	SuitePath      string
	OutputPath     string
	RepositoryRoot string
	Build          buildinfo.Info
	Probe          doctor.Probe
	ReadFile       func(string) ([]byte, error)
	Runner         EngineRunner
	Now            func() time.Time
	Persist        func(string, Capture) error
}

// Execute validates inputs, captures the environment, runs each scenario
// sequentially, derives descriptive statistics, and atomically persists one
// complete capture. No partial result file is produced on scenario failure.
func Execute(ctx context.Context, options Options) (Capture, error) {
	if err := validateOptions(options); err != nil {
		return Capture{}, err
	}
	if err := ctx.Err(); err != nil {
		return Capture{}, err
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
	runner := options.Runner
	if runner == nil {
		runner = RunEngine
	}
	persist := options.Persist
	if persist == nil {
		persist = PersistCapture
		if err := requireAbsent(options.OutputPath); err != nil {
			return Capture{}, err
		}
	}

	startedAt := now().UTC()
	suiteBytes, err := readFile(options.SuitePath)
	if err != nil {
		return Capture{}, fmt.Errorf("read benchmark suite %q: %w", options.SuitePath, err)
	}
	var suite Suite
	if err := jsoncheck.Decode(suiteBytes, &suite, true); err != nil {
		return Capture{}, fmt.Errorf("decode benchmark suite %q: %w", options.SuitePath, err)
	}
	if err := suite.Validate(); err != nil {
		return Capture{}, fmt.Errorf("validate benchmark suite %q: %w", options.SuitePath, err)
	}

	root, err := filepath.Abs(options.RepositoryRoot)
	if err != nil {
		return Capture{}, fmt.Errorf("resolve repository root %q: %w", options.RepositoryRoot, err)
	}
	engineHash, err := hashFile(options.EnginePath)
	if err != nil {
		return Capture{}, fmt.Errorf("hash engine artifact %q: %w", options.EnginePath, err)
	}

	environment := doctor.Check(ctx, probe, options.Build)
	if !environment.Ready {
		return Capture{}, errors.New(
			"host is not ready for Day 5; run 'labctl doctor' and resolve required tool failures",
		)
	}

	experiments := make([]Experiment, 0, len(suite.Scenarios))
	for index, scenario := range suite.Scenarios {
		if err := ctx.Err(); err != nil {
			return Capture{}, err
		}
		workloadPath, err := resolveRepositoryPath(root, scenario.Workload)
		if err != nil {
			return Capture{}, fmt.Errorf("scenario %q: %w", scenario.Name, err)
		}
		workloadBytes, err := readFile(workloadPath)
		if err != nil {
			return Capture{}, fmt.Errorf(
				"scenario %q: read workload %q: %w",
				scenario.Name,
				scenario.Workload,
				err,
			)
		}
		projection, err := protocol.ProjectWorkload(workloadBytes)
		if err != nil {
			return Capture{}, fmt.Errorf(
				"scenario %q: inspect workload %q: %w",
				scenario.Name,
				scenario.Workload,
				err,
			)
		}
		if projection.SchemaVersion != "paraflow.workload/v1" {
			return Capture{}, fmt.Errorf(
				"scenario %q: unsupported workload schema_version %q",
				scenario.Name,
				projection.SchemaVersion,
			)
		}

		request := Request{
			SchemaVersion: RequestSchema,
			ExperimentID:  fmt.Sprintf("day05:%016x", index+1),
			ScenarioName:  scenario.Name,
			Execution:     scenario.Execution,
			Sampling:      scenario.Sampling,
			Workload:      append([]byte(nil), workloadBytes...),
		}
		engineResult, orchestration, err := runner(
			ctx,
			options.EnginePath,
			request,
			projection,
		)
		if err != nil {
			return Capture{}, fmt.Errorf("scenario %q: %w", scenario.Name, err)
		}
		if err := validateSourceAlignment(engineResult.EngineBuild, options.Build); err != nil {
			return Capture{}, fmt.Errorf("scenario %q: %w", scenario.Name, err)
		}
		summary, err := Summarize(engineResult.Samples)
		if err != nil {
			return Capture{}, fmt.Errorf("scenario %q: summarize samples: %w", scenario.Name, err)
		}

		experiments = append(experiments, Experiment{
			ScenarioName: scenario.Name,
			Workload: WorkloadIdentity{
				Path:          scenario.Workload,
				SHA256:        hashBytes(workloadBytes),
				SchemaVersion: projection.SchemaVersion,
				Name:          projection.Name,
				RecordCount:   projection.RecordCount,
				CategoryCount: projection.CategoryCount,
			},
			OrchestrationTotalNS: orchestration,
			EngineResult:         engineResult,
			Summary:              summary,
		})
	}

	finalEngineHash, err := hashFile(options.EnginePath)
	if err != nil {
		return Capture{}, fmt.Errorf("rehash engine artifact %q: %w", options.EnginePath, err)
	}
	if finalEngineHash != engineHash {
		return Capture{}, errors.New("engine artifact changed while the benchmark suite was running")
	}

	capture := Capture{
		SchemaVersion: CaptureSchema,
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
	if capture.CompletedAt.Before(capture.StartedAt) {
		return Capture{}, errors.New("capture clock moved backwards between start and completion")
	}
	if err := persist(options.OutputPath, capture); err != nil {
		return Capture{}, fmt.Errorf("persist benchmark capture %q: %w", options.OutputPath, err)
	}
	return capture, nil
}

func validateOptions(options Options) error {
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

func validateSourceAlignment(engine EngineBuild, controller buildinfo.Info) error {
	if engine.SourceCommit != controller.FullCommit {
		return fmt.Errorf(
			"engine source commit %q does not match controller commit %q",
			engine.SourceCommit,
			controller.FullCommit,
		)
	}
	if engine.SourceState != controller.SourceState {
		return fmt.Errorf(
			"engine source state %q does not match controller source state %q",
			engine.SourceState,
			controller.SourceState,
		)
	}
	return nil
}

func identityPath(root, value string) string {
	absolute, err := filepath.Abs(value)
	if err != nil {
		return filepath.ToSlash(filepath.Clean(value))
	}
	relative, err := filepath.Rel(root, absolute)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(filepath.Clean(relative))
	}
	return filepath.ToSlash(filepath.Clean(absolute))
}

func resolveRepositoryPath(root, repositoryPath string) (string, error) {
	candidate := filepath.Join(root, filepath.FromSlash(repositoryPath))
	absolute, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve workload path %q: %w", repositoryPath, err)
	}
	relative, err := filepath.Rel(root, absolute)
	if err != nil {
		return "", fmt.Errorf("compare workload path %q with repository root: %w", repositoryPath, err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("workload path %q escapes the repository root", repositoryPath)
	}
	return absolute, nil
}

func hashBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = file.Close()
	}()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func requireAbsent(path string) error {
	_, err := os.Stat(path)
	switch {
	case err == nil:
		return fmt.Errorf("output path %q already exists", path)
	case errors.Is(err, os.ErrNotExist):
		return nil
	default:
		return fmt.Errorf("inspect output path %q: %w", path, err)
	}
}
