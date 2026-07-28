package benchmark

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kvetinski/paraflow/labctl-go/internal/buildinfo"
	"github.com/kvetinski/paraflow/labctl-go/internal/doctor"
	"github.com/kvetinski/paraflow/labctl-go/internal/protocol"
)

func TestExecuteBuildsOneCompleteCaptureWithoutDroppingRawSamples(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workloadDirectory := filepath.Join(root, "workloads")
	if err := os.MkdirAll(workloadDirectory, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	workload := []byte(
		`{"schema_version":"paraflow.workload/v1","name":"fixture",` +
			`"dataset":{"record_count":0,"category_count":1}}`,
	)
	if err := os.WriteFile(filepath.Join(workloadDirectory, "fixture.json"), workload, 0o644); err != nil {
		t.Fatalf("WriteFile(workload) error = %v", err)
	}
	suite := []byte(
		`{"schema_version":"paraflow.benchmark-suite/v1","name":"test suite",` +
			`"scenarios":[{"name":"fixture scenario","workload":"workloads/fixture.json",` +
			`"execution":{"backend":"scalar"},` +
			`"sampling":{"warmup_iterations":1,"sample_iterations":2}}]}`,
	)
	suitePath := filepath.Join(root, "suite.json")
	if err := os.WriteFile(suitePath, suite, 0o644); err != nil {
		t.Fatalf("WriteFile(suite) error = %v", err)
	}
	enginePath := filepath.Join(root, "paraflow-engine")
	if err := os.WriteFile(enginePath, []byte("engine artifact"), 0o755); err != nil {
		t.Fatalf("WriteFile(engine) error = %v", err)
	}
	outputPath := filepath.Join(root, "results", "capture.json")

	request, projection, engineResult := validEngineFixture()
	_ = request
	_ = projection
	var runnerCalls int
	runner := func(
		_ context.Context,
		path string,
		actualRequest Request,
		actualProjection protocol.WorkloadProjection,
	) (EngineResult, Nanoseconds, error) {
		runnerCalls++
		if path != enginePath {
			t.Fatalf("runner engine path = %q", path)
		}
		if actualRequest.ExperimentID != "day05:0000000000000001" {
			t.Fatalf("experiment ID = %q", actualRequest.ExperimentID)
		}
		if actualProjection.Name != "fixture" {
			t.Fatalf("workload projection = %#v", actualProjection)
		}
		engineResult.ExperimentID = actualRequest.ExperimentID
		engineResult.ScenarioName = actualRequest.ScenarioName
		engineResult.Execution = actualRequest.Execution
		engineResult.Sampling = actualRequest.Sampling
		return engineResult, 250, nil
	}

	times := []time.Time{
		time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 26, 12, 0, 1, 0, time.UTC),
	}
	capture, err := Execute(context.Background(), Options{
		EnginePath:     enginePath,
		SuitePath:      suitePath,
		OutputPath:     outputPath,
		RepositoryRoot: root,
		Build: buildinfo.Info{
			Version:     "test",
			FullCommit:  "0123456789abcdef0123456789abcdef01234567",
			SourceState: buildinfo.SourceClean,
		},
		Probe:  availableBenchmarkProbe,
		Runner: runner,
		Now: func() time.Time {
			value := times[0]
			times = times[1:]
			return value
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if runnerCalls != 1 || len(capture.Experiments) != 1 {
		t.Fatalf("runner calls = %d, experiments = %d", runnerCalls, len(capture.Experiments))
	}
	if got := len(capture.Experiments[0].EngineResult.Samples); got != 2 {
		t.Fatalf("retained samples = %d, want 2", got)
	}
	if capture.Experiments[0].Summary.Pipeline.MedianNS != 20 {
		t.Fatalf("pipeline median = %d", capture.Experiments[0].Summary.Pipeline.MedianNS)
	}
	if capture.Suite.Path != "suite.json" {
		t.Fatalf("suite identity path = %q, want repository-relative path", capture.Suite.Path)
	}
	if capture.EngineArtifact.Path != "paraflow-engine" {
		t.Fatalf(
			"engine identity path = %q, want repository-relative path",
			capture.EngineArtifact.Path,
		)
	}

	persisted, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile(capture) error = %v", err)
	}
	var decoded Capture
	if err := json.Unmarshal(persisted, &decoded); err != nil {
		t.Fatalf("decode persisted capture: %v", err)
	}
	if decoded.SchemaVersion != CaptureSchema || len(decoded.Experiments) != 1 {
		t.Fatalf("unexpected persisted capture: %#v", decoded)
	}
}

func TestExecuteRejectsAnEngineArtifactChangedDuringTheSuite(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workloadDirectory := filepath.Join(root, "workloads")
	if err := os.MkdirAll(workloadDirectory, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	workload := []byte(
		`{"schema_version":"paraflow.workload/v1","name":"fixture",` +
			`"dataset":{"record_count":0,"category_count":1}}`,
	)
	if err := os.WriteFile(filepath.Join(workloadDirectory, "fixture.json"), workload, 0o644); err != nil {
		t.Fatalf("WriteFile(workload) error = %v", err)
	}
	suitePath := filepath.Join(root, "suite.json")
	suite := []byte(
		`{"schema_version":"paraflow.benchmark-suite/v1","name":"test suite",` +
			`"scenarios":[{"name":"fixture scenario","workload":"workloads/fixture.json",` +
			`"execution":{"backend":"scalar"},` +
			`"sampling":{"warmup_iterations":0,"sample_iterations":2}}]}`,
	)
	if err := os.WriteFile(suitePath, suite, 0o644); err != nil {
		t.Fatalf("WriteFile(suite) error = %v", err)
	}
	enginePath := filepath.Join(root, "paraflow-engine")
	if err := os.WriteFile(enginePath, []byte("engine-v1"), 0o755); err != nil {
		t.Fatalf("WriteFile(engine) error = %v", err)
	}

	_, _, engineResult := validEngineFixture()
	persisted := false
	_, err := Execute(context.Background(), Options{
		EnginePath:     enginePath,
		SuitePath:      suitePath,
		OutputPath:     filepath.Join(root, "capture.json"),
		RepositoryRoot: root,
		Build: buildinfo.Info{
			Version:     "test",
			FullCommit:  engineResult.EngineBuild.SourceCommit,
			SourceState: engineResult.EngineBuild.SourceState,
		},
		Probe: availableBenchmarkProbe,
		Runner: func(
			_ context.Context,
			_ string,
			request Request,
			_ protocol.WorkloadProjection,
		) (EngineResult, Nanoseconds, error) {
			engineResult.ExperimentID = request.ExperimentID
			engineResult.ScenarioName = request.ScenarioName
			engineResult.Execution = request.Execution
			engineResult.Sampling = request.Sampling
			if err := os.WriteFile(enginePath, []byte("engine-v2"), 0o755); err != nil {
				t.Fatalf("mutate engine artifact: %v", err)
			}
			return engineResult, 250, nil
		},
		Now: func() time.Time {
			return time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
		},
		Persist: func(string, Capture) error {
			persisted = true
			return nil
		},
	})
	if err == nil {
		t.Fatal("Execute() must reject an engine artifact changed during the suite")
	}
	if persisted {
		t.Fatal("a capture must not be persisted after engine identity changes")
	}
}

func TestPersistCaptureRefusesToOverwriteEvidence(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "capture.json")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := PersistCapture(path, Capture{SchemaVersion: CaptureSchema}); err == nil {
		t.Fatal("PersistCapture() must reject an existing path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "original" {
		t.Fatalf("existing evidence was changed: %q", data)
	}
}

func availableBenchmarkProbe(_ context.Context, tool doctor.Tool) doctor.ToolResult {
	version := tool.Name + " version 1.0.0"
	switch tool.Name {
	case "go":
		version = "go version go1.24.0 linux/amd64"
	case "rustc":
		version = "rustc 1.88.0 (test)"
	case "cargo":
		version = "cargo 1.88.0 (test)"
	case "node":
		version = "v20.0.0"
	case "bash":
		version = "GNU bash, version 5.0.0"
	case "make":
		version = "GNU Make 4.4"
	}
	return doctor.ToolResult{
		Name:     tool.Name,
		Required: tool.Required,
		Found:    true,
		Usable:   true,
		Path:     "/tools/" + tool.Command,
		Version:  version,
	}
}
