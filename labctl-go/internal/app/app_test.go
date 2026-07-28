package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kvetinski/paraflow/labctl-go/internal/benchmark"
	"github.com/kvetinski/paraflow/labctl-go/internal/buildinfo"
	"github.com/kvetinski/paraflow/labctl-go/internal/doctor"
	"github.com/kvetinski/paraflow/labctl-go/internal/protocol"
)

func availableProbe(_ context.Context, tool doctor.Tool) doctor.ToolResult {
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
	case "rustfmt":
		version = "rustfmt 1.8.0-stable (test)"
	case "bash":
		version = "GNU bash, version 5.0.0 (test)"
	case "make":
		version = "GNU Make 4.4"
	}
	return doctor.ToolResult{
		Name:     tool.Name,
		Required: tool.Required,
		Found:    true,
		Path:     "/tools/" + tool.Command,
		Version:  version,
	}
}

func testDependencies() Dependencies {
	return Dependencies{
		Build: BuildInfo{
			Version:     "test",
			FullCommit:  "0123456789abcdef0123456789abcdef01234567",
			SourceState: buildinfo.SourceClean,
		},
		Probe: availableProbe,
	}
}

func TestVersion(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(
		context.Background(),
		[]string{"version"},
		&stdout,
		&stderr,
		testDependencies(),
	)

	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
	want := "labctl test " +
		"(commit 0123456789abcdef0123456789abcdef01234567, source clean)"
	if got := strings.TrimSpace(stdout.String()); got != want {
		t.Fatalf("unexpected version output: %q", got)
	}
}

func TestDoctorJSON(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(
		context.Background(),
		[]string{"doctor", "--json"},
		&stdout,
		&stderr,
		testDependencies(),
	)

	if exitCode != 0 {
		t.Fatalf("expected zero exit code, got %d: %s", exitCode, stderr.String())
	}

	var report doctor.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode doctor report: %v", err)
	}
	if !report.Ready {
		t.Fatal("expected doctor report to be ready")
	}
	if report.SchemaVersion != "paraflow.environment/v3" {
		t.Fatalf("unexpected report schema: %q", report.SchemaVersion)
	}
	if report.Milestone != "day-05" {
		t.Fatalf("unexpected report milestone: %q", report.Milestone)
	}
	if report.Source != testDependencies().Build {
		t.Fatalf("unexpected source identity: %#v", report.Source)
	}
	if report.CapturedAt.IsZero() {
		t.Fatal("doctor report must include its capture time")
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode doctor envelope: %v", err)
	}
	if _, exists := envelope["ready_for_day_1"]; exists {
		t.Fatal("legacy ready_for_day_1 key must not appear in environment/v3")
	}
	if _, exists := envelope["ready"]; !exists {
		t.Fatal("environment/v3 must contain the ready key")
	}
}

func TestDoctorReturnsFailureWhenRequiredToolIsUnusable(t *testing.T) {
	t.Parallel()

	dependencies := testDependencies()
	dependencies.Probe = func(ctx context.Context, tool doctor.Tool) doctor.ToolResult {
		result := availableProbe(ctx, tool)
		if tool.Name == "cargo" {
			result.Version = "cargo 1.87.0 (test)"
		}
		return result
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(
		context.Background(),
		[]string{"doctor", "--json"},
		&stdout,
		&stderr,
		dependencies,
	)

	if exitCode != 1 {
		t.Fatalf("expected readiness failure exit code 1, got %d", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}

	var report doctor.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode doctor report: %v", err)
	}
	if report.Ready {
		t.Fatal("outdated required Cargo must make doctor unready")
	}
}

func TestUnknownCommandIsUsageError(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(
		context.Background(),
		[]string{"unknown"},
		&stdout,
		&stderr,
		testDependencies(),
	)

	if exitCode != 2 {
		t.Fatalf("expected usage exit code 2, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestDoctorRejectsUnexpectedArguments(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(
		context.Background(),
		[]string{"doctor", "--verbose"},
		&stdout,
		&stderr,
		testDependencies(),
	)

	if exitCode != 2 {
		t.Fatalf("expected usage exit code 2, got %d", exitCode)
	}
}

func TestBenchmarkCommandPassesExplicitBoundariesAndPrintsMedians(t *testing.T) {
	t.Parallel()

	dependencies := testDependencies()
	dependencies.RunBenchmark = func(
		_ context.Context,
		options benchmark.Options,
	) (benchmark.Capture, error) {
		if options.EnginePath != "engine" || options.SuitePath != "suite.json" ||
			options.OutputPath != "results/capture.json" || options.RepositoryRoot != "repo" {
			t.Fatalf("unexpected benchmark options: %#v", options)
		}
		if options.Build != dependencies.Build || options.Probe == nil {
			t.Fatalf("benchmark dependencies were not injected: %#v", options)
		}
		return benchmark.Capture{
			Suite: benchmark.SuiteIdentity{Name: "day05 baseline"},
			Experiments: []benchmark.Experiment{{
				ScenarioName: "uniform-1k",
				Summary: benchmark.Summary{
					Generation: benchmark.Statistics{MedianNS: 10},
					Pipeline:   benchmark.Statistics{MedianNS: 20},
					EngineTotal: benchmark.Statistics{
						Count:    3,
						MedianNS: 35,
					},
				},
			}},
		}, nil
	}

	exitCode, stdout, stderr := runForTest(
		t,
		dependencies,
		"benchmark",
		"--suite", "suite.json",
		"--engine", "engine",
		"--repository-root", "repo",
		"--output", "results/capture.json",
	)
	if exitCode != 0 || stderr != "" {
		t.Fatalf("exit = %d, stderr = %q", exitCode, stderr)
	}
	for _, expected := range []string{
		"capture: results/capture.json",
		"suite: \"day05 baseline\"",
		"samples=3",
		"pipeline_median_ns=20",
		"no speedup claim is implied",
	} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("stdout missing %q:\n%s", expected, stdout)
		}
	}
}

func TestBenchmarkCommandRejectsIncompleteOrDuplicateFlags(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"benchmark"},
		{"benchmark", "--engine", "engine"},
		{"benchmark", "--engine", "engine", "--engine", "again", "--suite", "s", "--output", "o"},
		{"benchmark", "--unknown", "x", "--engine", "e", "--suite", "s", "--output", "o"},
	} {
		exitCode, _, _ := runForTest(t, testDependencies(), args...)
		if exitCode != 2 {
			t.Fatalf("Run(%v) exit code = %d, want 2", args, exitCode)
		}
	}
}

func TestRunExecutesThroughWorkerAndShutsItDown(t *testing.T) {
	t.Parallel()

	workload := []byte(
		`{"schema_version":"paraflow.workload/v1","name":"day-05-smoke","dataset":` +
			`{"record_count":3,"category_count":2}}`,
	)
	session := &fakeWorkerSession{
		result: protocol.ResultV1{
			AcceptedCount:     3,
			ScoreSum:          6.5,
			CategoryHistogram: []uint64{1, 2},
			AcceptedIDSum:     16,
			AcceptedIDXOR:     0x6ebb399a18884447,
		},
	}
	dependencies := testDependencies()
	dependencies.ReadFile = func(path string) ([]byte, error) {
		if path != "workload.json" {
			t.Fatalf("ReadFile() path = %q", path)
		}
		return workload, nil
	}
	dependencies.StartWorker = func(
		_ context.Context,
		path string,
	) (WorkerSession, error) {
		if path != "./paraflow-engine" {
			t.Fatalf("StartWorker() path = %q", path)
		}
		return session, nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(
		context.Background(),
		[]string{
			"run",
			"--engine",
			"./paraflow-engine",
			"workload.json",
		},
		&stdout,
		&stderr,
		dependencies,
	)

	if exitCode != 0 {
		t.Fatalf("Run() exit code = %d, stderr = %s", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
	if !bytes.Equal(session.workload, workload) {
		t.Fatalf("Execute() workload = %s", session.workload)
	}
	if got, want := session.events, []string{"execute", "shutdown", "close"}; !equalStrings(got, want) {
		t.Fatalf("worker events = %v, want %v", got, want)
	}

	wantOutput := "" +
		"workload: \"day-05-smoke\"\n" +
		"backend: scalar\n" +
		"accepted_count: 3\n" +
		"score_sum: 6.5\n" +
		"score_sum_bits: 0x401a000000000000\n" +
		"category_histogram: [1,2]\n" +
		"accepted_id_sum: 0x0000000000000010\n" +
		"accepted_id_xor: 0x6ebb399a18884447\n"
	if stdout.String() != wantOutput {
		t.Fatalf("stdout mismatch\n got: %s\nwant: %s", stdout.String(), wantOutput)
	}
}

func TestRunFailuresAreActionableAndLeakFree(t *testing.T) {
	t.Parallel()

	const workload = `{"schema_version":"paraflow.workload/v1","name":"failure","dataset":` +
		`{"record_count":1,"category_count":1}}`

	t.Run("read", func(t *testing.T) {
		dependencies := testDependencies()
		dependencies.ReadFile = func(string) ([]byte, error) {
			return nil, errors.New("injected read failure")
		}
		dependencies.StartWorker = func(
			context.Context,
			string,
		) (WorkerSession, error) {
			t.Fatal("worker must not start after a read failure")
			return nil, nil
		}

		exitCode, _, stderr := runForTest(
			t,
			dependencies,
			"run",
			"--engine",
			"engine",
			"missing.json",
		)
		if exitCode != 1 || !strings.Contains(stderr, "injected read failure") {
			t.Fatalf("exit = %d, stderr = %q", exitCode, stderr)
		}
	})

	t.Run("start", func(t *testing.T) {
		dependencies := testDependencies()
		dependencies.ReadFile = func(string) ([]byte, error) {
			return []byte(workload), nil
		}
		dependencies.StartWorker = func(
			context.Context,
			string,
		) (WorkerSession, error) {
			return nil, errors.New("injected start failure")
		}

		exitCode, _, stderr := runForTest(
			t,
			dependencies,
			"run",
			"--engine",
			"engine",
			"workload.json",
		)
		if exitCode != 1 || !strings.Contains(stderr, "injected start failure") {
			t.Fatalf("exit = %d, stderr = %q", exitCode, stderr)
		}
	})

	t.Run("execute", func(t *testing.T) {
		session := &fakeWorkerSession{
			executeError: errors.New("injected execute failure"),
		}
		dependencies := runDependencies([]byte(workload), session)

		exitCode, _, stderr := runForTest(
			t,
			dependencies,
			"run",
			"--engine",
			"engine",
			"workload.json",
		)
		if exitCode != 1 || !strings.Contains(stderr, "injected execute failure") {
			t.Fatalf("exit = %d, stderr = %q", exitCode, stderr)
		}
		if got, want := session.events, []string{"execute", "close"}; !equalStrings(got, want) {
			t.Fatalf("worker events = %v, want %v", got, want)
		}
	})

	t.Run("shutdown", func(t *testing.T) {
		session := &fakeWorkerSession{
			shutdownError: errors.New("injected shutdown failure"),
		}
		dependencies := runDependencies([]byte(workload), session)

		exitCode, _, stderr := runForTest(
			t,
			dependencies,
			"run",
			"--engine",
			"engine",
			"workload.json",
		)
		if exitCode != 1 || !strings.Contains(stderr, "injected shutdown failure") {
			t.Fatalf("exit = %d, stderr = %q", exitCode, stderr)
		}
		if got, want := session.events, []string{"execute", "shutdown", "close"}; !equalStrings(got, want) {
			t.Fatalf("worker events = %v, want %v", got, want)
		}
	})
}

func TestRunRequiresExplicitEngineAndWorkload(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"run"},
		{"run", "workload.json"},
		{"run", "--engine", "engine"},
		{"run", "--backend", "engine", "workload.json"},
		{"run", "--engine", "", "workload.json"},
		{"run", "--engine", "engine", ""},
	} {
		exitCode, _, stderr := runForTest(t, testDependencies(), args...)
		if exitCode != 2 {
			t.Fatalf("Run(%v) exit code = %d, stderr = %q", args, exitCode, stderr)
		}
	}
}

type fakeWorkerSession struct {
	result        protocol.ResultV1
	executeError  error
	shutdownError error
	closeError    error
	workload      []byte
	events        []string
}

func (session *fakeWorkerSession) Execute(
	_ context.Context,
	workload json.RawMessage,
) (protocol.ResultV1, error) {
	session.events = append(session.events, "execute")
	session.workload = append(session.workload[:0], workload...)
	return session.result, session.executeError
}

func (session *fakeWorkerSession) Shutdown(context.Context) error {
	session.events = append(session.events, "shutdown")
	return session.shutdownError
}

func (session *fakeWorkerSession) Close() error {
	session.events = append(session.events, "close")
	return session.closeError
}

func runDependencies(
	workload []byte,
	session WorkerSession,
) Dependencies {
	dependencies := testDependencies()
	dependencies.ReadFile = func(string) ([]byte, error) {
		return workload, nil
	}
	dependencies.StartWorker = func(
		context.Context,
		string,
	) (WorkerSession, error) {
		return session, nil
	}
	return dependencies
}

func runForTest(
	t *testing.T,
	dependencies Dependencies,
	args ...string,
) (int, string, string) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(
		context.Background(),
		args,
		&stdout,
		&stderr,
		dependencies,
	)
	return exitCode, stdout.String(), stderr.String()
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
