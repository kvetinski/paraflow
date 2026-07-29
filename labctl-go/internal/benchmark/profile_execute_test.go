package benchmark

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kvetinski/paraflow/labctl-go/internal/buildinfo"
	"github.com/kvetinski/paraflow/labctl-go/internal/protocol"
)

func TestExecuteProfileBuildsPairedSelfContainedEvidence(t *testing.T) {
	t.Parallel()

	root, suitePath, enginePath := writeProfileTestInputs(t)
	outputPath := filepath.Join(root, "results", "day06-profile.json")
	_, _, _, baselineResult, profileResult := validPairedProfileFixture()
	var baselineCalls int
	var profileCalls int
	times := []time.Time{
		time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 26, 12, 0, 1, 0, time.UTC),
	}

	report, err := ExecuteProfile(context.Background(), ProfileOptions{
		EnginePath:     enginePath,
		SuitePath:      suitePath,
		OutputPath:     outputPath,
		RepositoryRoot: root,
		Build: buildinfo.Info{
			Version:     "test",
			FullCommit:  baselineResult.EngineBuild.SourceCommit,
			SourceState: baselineResult.EngineBuild.SourceState,
		},
		Probe: availableBenchmarkProbe,
		BaselineRunner: func(
			_ context.Context,
			path string,
			request Request,
			projection protocol.WorkloadProjection,
		) (EngineResult, Nanoseconds, error) {
			baselineCalls++
			if path != enginePath || projection.Distribution != "uniform" {
				t.Fatalf("unexpected baseline inputs: %q %#v", path, projection)
			}
			baselineResult.ExperimentID = request.ExperimentID
			baselineResult.ScenarioName = request.ScenarioName
			baselineResult.Execution = request.Execution
			baselineResult.Sampling = request.Sampling
			return baselineResult, 2_500, nil
		},
		ProfileRunner: func(
			_ context.Context,
			path string,
			request ProfileRequest,
			projection protocol.WorkloadProjection,
		) (ProfileEngineResult, Nanoseconds, error) {
			profileCalls++
			if path != enginePath || projection.RecordCount != 8 {
				t.Fatalf("unexpected profile inputs: %q %#v", path, projection)
			}
			profileResult.ExperimentID = request.ExperimentID
			profileResult.ScenarioName = request.ScenarioName
			profileResult.Execution = request.Execution
			profileResult.Sampling = request.Sampling
			return profileResult, 3_500, nil
		},
		Now: func() time.Time {
			value := times[0]
			times = times[1:]
			return value
		},
	})
	if err != nil {
		t.Fatalf("ExecuteProfile() error = %v", err)
	}
	if baselineCalls != 1 || profileCalls != 1 || len(report.Experiments) != 1 {
		t.Fatalf(
			"baseline calls = %d, profile calls = %d, experiments = %d",
			baselineCalls,
			profileCalls,
			len(report.Experiments),
		)
	}
	experiment := report.Experiments[0]
	if experiment.Analysis.DominantPipelineStage != "normalize" {
		t.Fatalf("unexpected analysis: %#v", experiment.Analysis)
	}
	if report.Suite.Path != "benchmarks/suites/day06-test.json" ||
		report.EngineArtifact.Path != "paraflow-engine" {
		t.Fatalf(
			"unexpected artifact identities: suite=%q engine=%q",
			report.Suite.Path,
			report.EngineArtifact.Path,
		)
	}

	persisted, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile(report) error = %v", err)
	}
	var decoded ScalarProfileReport
	if err := json.Unmarshal(persisted, &decoded); err != nil {
		t.Fatalf("decode persisted report: %v", err)
	}
	if decoded.SchemaVersion != ScalarProfileReportSchema ||
		len(decoded.Experiments) != 1 {
		t.Fatalf("unexpected persisted report: %#v", decoded)
	}
}

func TestExecuteProfileRejectsDifferentEngineBuildsBeforePersistence(t *testing.T) {
	t.Parallel()

	root, suitePath, enginePath := writeProfileTestInputs(t)
	_, _, _, baselineResult, profileResult := validPairedProfileFixture()
	profileResult.EngineBuild.Version = "different"
	persisted := false

	_, err := ExecuteProfile(context.Background(), ProfileOptions{
		EnginePath:     enginePath,
		SuitePath:      suitePath,
		OutputPath:     filepath.Join(root, "report.json"),
		RepositoryRoot: root,
		Build: buildinfo.Info{
			Version:     "test",
			FullCommit:  baselineResult.EngineBuild.SourceCommit,
			SourceState: baselineResult.EngineBuild.SourceState,
		},
		Probe: availableBenchmarkProbe,
		BaselineRunner: func(
			_ context.Context,
			_ string,
			request Request,
			_ protocol.WorkloadProjection,
		) (EngineResult, Nanoseconds, error) {
			baselineResult.ExperimentID = request.ExperimentID
			baselineResult.ScenarioName = request.ScenarioName
			return baselineResult, 2_500, nil
		},
		ProfileRunner: func(
			_ context.Context,
			_ string,
			request ProfileRequest,
			_ protocol.WorkloadProjection,
		) (ProfileEngineResult, Nanoseconds, error) {
			profileResult.ExperimentID = request.ExperimentID
			profileResult.ScenarioName = request.ScenarioName
			return profileResult, 3_500, nil
		},
		Now: func() time.Time {
			return time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
		},
		Persist: func(string, ScalarProfileReport) error {
			persisted = true
			return nil
		},
	})
	if err == nil {
		t.Fatal("different baseline/profile engine builds must be rejected")
	}
	if persisted {
		t.Fatal("invalid paired evidence must not be persisted")
	}
}

func TestPersistScalarProfileReportRefusesToOverwriteEvidence(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "profile.json")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := PersistScalarProfileReport(
		path,
		ScalarProfileReport{SchemaVersion: ScalarProfileReportSchema},
	); err == nil {
		t.Fatal("PersistScalarProfileReport() must reject an existing path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "original" {
		t.Fatalf("existing evidence was changed: %q", data)
	}
}

func writeProfileTestInputs(t *testing.T) (string, string, string) {
	t.Helper()

	root := t.TempDir()
	workloadDirectory := filepath.Join(root, "workloads")
	suiteDirectory := filepath.Join(root, "benchmarks", "suites")
	if err := os.MkdirAll(workloadDirectory, 0o755); err != nil {
		t.Fatalf("MkdirAll(workloads) error = %v", err)
	}
	if err := os.MkdirAll(suiteDirectory, 0o755); err != nil {
		t.Fatalf("MkdirAll(suites) error = %v", err)
	}
	workload := []byte(
		`{"schema_version":"paraflow.workload/v1","name":"fixture",` +
			`"dataset":{"record_count":8,"category_count":1,` +
			`"distribution":{"kind":"uniform"}}}`,
	)
	if err := os.WriteFile(
		filepath.Join(workloadDirectory, "fixture.json"),
		workload,
		0o644,
	); err != nil {
		t.Fatalf("WriteFile(workload) error = %v", err)
	}
	suite := []byte(
		`{"schema_version":"paraflow.benchmark-suite/v1","name":"test suite",` +
			`"scenarios":[{"name":"fixture scenario",` +
			`"workload":"workloads/fixture.json","execution":{"backend":"scalar"},` +
			`"sampling":{"warmup_iterations":1,"sample_iterations":2}}]}`,
	)
	suitePath := filepath.Join(suiteDirectory, "day06-test.json")
	if err := os.WriteFile(suitePath, suite, 0o644); err != nil {
		t.Fatalf("WriteFile(suite) error = %v", err)
	}
	enginePath := filepath.Join(root, "paraflow-engine")
	if err := os.WriteFile(enginePath, []byte("engine artifact"), 0o755); err != nil {
		t.Fatalf("WriteFile(engine) error = %v", err)
	}
	return root, suitePath, enginePath
}
