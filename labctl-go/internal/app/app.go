// Package app contains labctl command dispatch independently from process I/O.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"github.com/kvetinski/paraflow/labctl-go/internal/benchmark"
	"github.com/kvetinski/paraflow/labctl-go/internal/buildinfo"
	"github.com/kvetinski/paraflow/labctl-go/internal/doctor"
	"github.com/kvetinski/paraflow/labctl-go/internal/protocol"
	"github.com/kvetinski/paraflow/labctl-go/internal/worker"
)

const help = `ParaFlow experiment controller

Usage:
  labctl run --engine <path> <workload.json>
  labctl benchmark --engine <path> --suite <suite.json> --output <capture.json> [--repository-root <path>]
  labctl profile --engine <path> --suite <suite.json> --output <report.json> [--repository-root <path>]
  labctl verify [--json] [--repository-root <path>] [--engine <path>] <evidence.json>
  labctl doctor [--json]
  labctl version
  labctl help

The run command verifies one workload through the versioned Rust worker.
The benchmark command captures the fused scalar baseline. The profile command
pairs that unchanged baseline with diagnostic materialized stage passes and
persists raw evidence plus integer-only analysis. The verify command replays
all deterministic evidence checks without rerunning a benchmark.`

// BuildInfo identifies the controller binary.
type BuildInfo = buildinfo.Info

// WorkerSession is the process boundary needed by the run command.
type WorkerSession interface {
	Execute(context.Context, json.RawMessage) (protocol.ResultV1, error)
	Shutdown(context.Context) error
	Close() error
}

// StartWorker launches one reusable engine session.
type StartWorker func(context.Context, string) (WorkerSession, error)

// RunBenchmark executes and persists one benchmark capture.
type RunBenchmark func(context.Context, benchmark.Options) (benchmark.Capture, error)

// RunProfile executes and persists one paired scalar profile report.
type RunProfile func(
	context.Context,
	benchmark.ProfileOptions,
) (benchmark.ScalarProfileReport, error)

// VerifyEvidence replays deterministic checks over one persisted artifact.
type VerifyEvidence func(benchmark.VerifyOptions) (benchmark.EvidenceVerification, error)

// Dependencies contains side effects injected into command dispatch.
type Dependencies struct {
	Build          BuildInfo
	Probe          doctor.Probe
	ReadFile       func(string) ([]byte, error)
	StartWorker    StartWorker
	RunBenchmark   RunBenchmark
	RunProfile     RunProfile
	VerifyEvidence VerifyEvidence
}

// Run dispatches a labctl command and returns a process-compatible exit code.
func Run(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	dependencies Dependencies,
) int {
	if len(args) == 0 {
		return write(stdout, help)
	}

	switch args[0] {
	case "help", "--help", "-h":
		if len(args) != 1 {
			return usageError(stderr, "help accepts no arguments")
		}
		return write(stdout, help)
	case "version", "--version":
		if len(args) != 1 {
			return usageError(stderr, "version accepts no arguments")
		}
		return write(
			stdout,
			fmt.Sprintf("labctl %s", dependencies.Build),
		)
	case "doctor":
		return runDoctor(
			ctx,
			args[1:],
			stdout,
			stderr,
			dependencies.Probe,
			dependencies.Build,
		)
	case "benchmark":
		return runBenchmark(
			ctx,
			args[1:],
			stdout,
			stderr,
			dependencies,
		)
	case "profile":
		return runProfile(
			ctx,
			args[1:],
			stdout,
			stderr,
			dependencies,
		)
	case "verify":
		return runVerify(args[1:], stdout, stderr, dependencies)
	case "run":
		return runWorkload(
			ctx,
			args[1:],
			stdout,
			stderr,
			dependencies.ReadFile,
			dependencies.StartWorker,
		)
	default:
		return usageError(stderr, fmt.Sprintf("unknown command %q", args[0]))
	}
}

func runVerify(
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	dependencies Dependencies,
) int {
	options, jsonOutput, err := parseVerifyOptions(args)
	if err != nil {
		return usageError(stderr, err.Error())
	}
	options.ReadFile = dependencies.ReadFile
	verify := dependencies.VerifyEvidence
	if verify == nil {
		verify = benchmark.VerifyEvidence
	}
	verification, err := verify(options)
	if err != nil {
		return commandError(stderr, fmt.Sprintf("verify evidence: %v", err))
	}

	if jsonOutput {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(verification); err != nil {
			return commandError(stderr, fmt.Sprintf("write verification receipt: %v", err))
		}
		return 0
	}

	engineStatus := "identity recorded; bytes not supplied"
	if verification.EngineArtifactVerified {
		engineStatus = "SHA-256 verified"
	}
	return write(
		stdout,
		fmt.Sprintf(
			"evidence: %s\n"+
				"status: %s\n"+
				"schema: %s\n"+
				"suite: %q\n"+
				"experiments: %d\n"+
				"retained_samples: %d\n"+
				"repository_identities: %d\n"+
				"engine_artifact: %s\n"+
				"evidence_sha256: %s",
			verification.EvidencePath,
			verification.Status,
			verification.EvidenceSchema,
			verification.SuiteName,
			verification.ExperimentCount,
			verification.RetainedSampleCount,
			verification.RepositoryIdentitiesVerified,
			engineStatus,
			verification.EvidenceSHA256,
		),
	)
}

func parseVerifyOptions(
	args []string,
) (benchmark.VerifyOptions, bool, error) {
	options := benchmark.VerifyOptions{RepositoryRoot: "."}
	seen := make(map[string]struct{})
	jsonOutput := false

	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch argument {
		case "--json":
			if _, duplicate := seen[argument]; duplicate {
				return benchmark.VerifyOptions{}, false, errors.New(
					"verify flag --json was provided more than once",
				)
			}
			seen[argument] = struct{}{}
			jsonOutput = true
		case "--repository-root", "--engine":
			if _, duplicate := seen[argument]; duplicate {
				return benchmark.VerifyOptions{}, false, fmt.Errorf(
					"verify flag %s was provided more than once",
					argument,
				)
			}
			seen[argument] = struct{}{}
			index++
			if index == len(args) || args[index] == "" ||
				strings.HasPrefix(args[index], "--") {
				return benchmark.VerifyOptions{}, false, fmt.Errorf(
					"verify flag %s requires a non-empty value",
					argument,
				)
			}
			if argument == "--repository-root" {
				options.RepositoryRoot = args[index]
			} else {
				options.EnginePath = args[index]
			}
		default:
			if strings.HasPrefix(argument, "-") {
				return benchmark.VerifyOptions{}, false, fmt.Errorf(
					"unknown verify flag %q",
					argument,
				)
			}
			if options.EvidencePath != "" {
				return benchmark.VerifyOptions{}, false, errors.New(
					"verify accepts exactly one evidence path",
				)
			}
			options.EvidencePath = argument
		}
	}
	if options.EvidencePath == "" {
		return benchmark.VerifyOptions{}, false, errors.New(
			"verify requires <evidence.json>",
		)
	}
	return options, jsonOutput, nil
}

func runProfile(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	dependencies Dependencies,
) int {
	options, err := parseProfileOptions(args)
	if err != nil {
		return usageError(stderr, err.Error())
	}
	options.Build = dependencies.Build
	options.Probe = dependencies.Probe
	options.ReadFile = dependencies.ReadFile

	execute := dependencies.RunProfile
	if execute == nil {
		execute = benchmark.ExecuteProfile
	}
	report, err := execute(ctx, options)
	if err != nil {
		return commandError(stderr, fmt.Sprintf("profile suite: %v", err))
	}

	var builder strings.Builder
	_, _ = fmt.Fprintf(
		&builder,
		"report: %s\nsuite: %q\nscenarios: %d\n",
		options.OutputPath,
		report.Suite.Name,
		len(report.Experiments),
	)
	for _, experiment := range report.Experiments {
		_, _ = fmt.Fprintf(
			&builder,
			"- %s: records=%d fused_pipeline_median_ns=%d dominant_stage=%s dominant_pipeline_stage=%s stage_pass_to_fused_ratio_milli=%d\n",
			experiment.ScenarioName,
			experiment.Workload.RecordCount,
			experiment.Baseline.Summary.Pipeline.MedianNS.Uint64(),
			experiment.Analysis.DominantStage,
			experiment.Analysis.DominantPipelineStage,
			experiment.Analysis.StagePassToFusedPipelineRatioMilli,
		)
	}
	builder.WriteString(
		"raw fused and stage-pass samples retained; observer ratio is not a speedup claim",
	)
	return write(stdout, builder.String())
}

func parseProfileOptions(args []string) (benchmark.ProfileOptions, error) {
	if len(args) == 0 || len(args)%2 != 0 {
		return benchmark.ProfileOptions{}, errors.New(
			"profile requires --engine <path> --suite <suite.json> --output <report.json> [--repository-root <path>]",
		)
	}

	options := benchmark.ProfileOptions{RepositoryRoot: "."}
	seen := make(map[string]struct{})
	for index := 0; index < len(args); index += 2 {
		flag := args[index]
		value := args[index+1]
		if _, duplicate := seen[flag]; duplicate {
			return benchmark.ProfileOptions{}, fmt.Errorf(
				"profile flag %s was provided more than once",
				flag,
			)
		}
		seen[flag] = struct{}{}
		if value == "" {
			return benchmark.ProfileOptions{}, fmt.Errorf(
				"profile flag %s requires a non-empty value",
				flag,
			)
		}
		switch flag {
		case "--engine":
			options.EnginePath = value
		case "--suite":
			options.SuitePath = value
		case "--output":
			options.OutputPath = value
		case "--repository-root":
			options.RepositoryRoot = value
		default:
			return benchmark.ProfileOptions{}, fmt.Errorf(
				"unknown profile flag %q",
				flag,
			)
		}
	}
	if options.EnginePath == "" || options.SuitePath == "" || options.OutputPath == "" {
		return benchmark.ProfileOptions{}, errors.New(
			"profile requires --engine <path> --suite <suite.json> --output <report.json>",
		)
	}
	return options, nil
}

func runBenchmark(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	dependencies Dependencies,
) int {
	options, err := parseBenchmarkOptions(args)
	if err != nil {
		return usageError(stderr, err.Error())
	}
	options.Build = dependencies.Build
	options.Probe = dependencies.Probe
	options.ReadFile = dependencies.ReadFile

	execute := dependencies.RunBenchmark
	if execute == nil {
		execute = benchmark.Execute
	}
	capture, err := execute(ctx, options)
	if err != nil {
		return commandError(stderr, fmt.Sprintf("benchmark suite: %v", err))
	}

	var builder strings.Builder
	_, _ = fmt.Fprintf(
		&builder,
		"capture: %s\nsuite: %q\nscenarios: %d\n",
		options.OutputPath,
		capture.Suite.Name,
		len(capture.Experiments),
	)
	for _, experiment := range capture.Experiments {
		_, _ = fmt.Fprintf(
			&builder,
			"- %s: samples=%d generation_median_ns=%d pipeline_median_ns=%d engine_total_median_ns=%d\n",
			experiment.ScenarioName,
			experiment.Summary.EngineTotal.Count,
			experiment.Summary.Generation.MedianNS.Uint64(),
			experiment.Summary.Pipeline.MedianNS.Uint64(),
			experiment.Summary.EngineTotal.MedianNS.Uint64(),
		)
	}
	builder.WriteString("raw samples retained; no speedup claim is implied")
	return write(stdout, builder.String())
}

func parseBenchmarkOptions(args []string) (benchmark.Options, error) {
	if len(args) == 0 || len(args)%2 != 0 {
		return benchmark.Options{}, errors.New(
			"benchmark requires --engine <path> --suite <suite.json> --output <capture.json> [--repository-root <path>]",
		)
	}

	options := benchmark.Options{RepositoryRoot: "."}
	seen := make(map[string]struct{})
	for index := 0; index < len(args); index += 2 {
		flag := args[index]
		value := args[index+1]
		if _, duplicate := seen[flag]; duplicate {
			return benchmark.Options{}, fmt.Errorf("benchmark flag %s was provided more than once", flag)
		}
		seen[flag] = struct{}{}
		if value == "" {
			return benchmark.Options{}, fmt.Errorf("benchmark flag %s requires a non-empty value", flag)
		}
		switch flag {
		case "--engine":
			options.EnginePath = value
		case "--suite":
			options.SuitePath = value
		case "--output":
			options.OutputPath = value
		case "--repository-root":
			options.RepositoryRoot = value
		default:
			return benchmark.Options{}, fmt.Errorf("unknown benchmark flag %q", flag)
		}
	}
	if options.EnginePath == "" || options.SuitePath == "" || options.OutputPath == "" {
		return benchmark.Options{}, errors.New(
			"benchmark requires --engine <path> --suite <suite.json> --output <capture.json>",
		)
	}
	return options, nil
}

func runWorkload(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	readFile func(string) ([]byte, error),
	startWorker StartWorker,
) int {
	if len(args) != 3 || args[0] != "--engine" {
		return usageError(
			stderr,
			"run requires --engine <path> <workload.json>",
		)
	}
	enginePath := args[1]
	workloadPath := args[2]
	if enginePath == "" || workloadPath == "" {
		return usageError(
			stderr,
			"run requires non-empty engine and workload paths",
		)
	}

	if readFile == nil {
		readFile = os.ReadFile
	}
	source, err := readFile(workloadPath)
	if err != nil {
		return commandError(
			stderr,
			fmt.Sprintf("read workload %q: %v", workloadPath, err),
		)
	}
	projection, err := protocol.ProjectWorkload(source)
	if err != nil {
		return commandError(
			stderr,
			fmt.Sprintf("inspect workload %q: %v", workloadPath, err),
		)
	}

	if startWorker == nil {
		startWorker = func(
			ctx context.Context,
			path string,
		) (WorkerSession, error) {
			return worker.Start(ctx, path)
		}
	}
	session, err := startWorker(ctx, enginePath)
	if err != nil {
		return commandError(
			stderr,
			fmt.Sprintf("start engine %q: %v", enginePath, err),
		)
	}
	defer func() {
		_ = session.Close()
	}()

	result, err := session.Execute(ctx, source)
	if err != nil {
		return commandError(
			stderr,
			fmt.Sprintf("execute workload %q: %v", workloadPath, err),
		)
	}
	if err := session.Shutdown(ctx); err != nil {
		return commandError(
			stderr,
			fmt.Sprintf("shut down engine %q: %v", enginePath, err),
		)
	}

	return writeRunResult(stdout, projection.Name, result)
}

func writeRunResult(
	output io.Writer,
	workloadName string,
	result protocol.ResultV1,
) int {
	histogram, err := json.Marshal(result.CategoryHistogram)
	if err != nil {
		return 1
	}
	message := fmt.Sprintf(
		"workload: %q\n"+
			"backend: %s\n"+
			"accepted_count: %d\n"+
			"score_sum: %s\n"+
			"score_sum_bits: 0x%016x\n"+
			"category_histogram: %s\n"+
			"accepted_id_sum: 0x%016x\n"+
			"accepted_id_xor: 0x%016x",
		workloadName,
		protocol.BackendScalar,
		result.AcceptedCount,
		formatFloat(result.ScoreSum),
		math.Float64bits(result.ScoreSum),
		histogram,
		result.AcceptedIDSum,
		result.AcceptedIDXOR,
	)
	return write(output, message)
}

func formatFloat(value float64) string {
	switch {
	case math.IsInf(value, 1):
		return "+Inf"
	case math.IsInf(value, -1):
		return "-Inf"
	case math.IsNaN(value):
		return "NaN"
	default:
		return fmt.Sprintf("%.17g", value)
	}
}

func runDoctor(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	probe doctor.Probe,
	build BuildInfo,
) int {
	jsonOutput := false
	switch {
	case len(args) == 0:
	case len(args) == 1 && args[0] == "--json":
		jsonOutput = true
	default:
		return usageError(stderr, "doctor accepts only the optional --json flag")
	}

	if probe == nil {
		probe = doctor.CommandProbe
	}
	report := doctor.Check(ctx, probe, build)

	if jsonOutput {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			_, _ = fmt.Fprintf(stderr, "error: write doctor report: %v\n", err)
			return 1
		}
	} else if exitCode := write(stdout, report.String()); exitCode != 0 {
		return exitCode
	}

	if report.Ready {
		return 0
	}
	return 1
}

func usageError(stderr io.Writer, message string) int {
	if _, err := fmt.Fprintf(
		stderr,
		"error: %s\nrun 'labctl help' for usage\n",
		message,
	); err != nil {
		return 1
	}
	return 2
}

func commandError(stderr io.Writer, message string) int {
	if _, err := fmt.Fprintf(stderr, "error: %s\n", message); err != nil {
		return 1
	}
	return 1
}

func write(output io.Writer, message string) int {
	if _, err := fmt.Fprintln(output, message); err != nil {
		return 1
	}
	return 0
}
