// Package app contains labctl command dispatch independently from process I/O.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/kvetinski/paraflow/labctl-go/internal/buildinfo"
	"github.com/kvetinski/paraflow/labctl-go/internal/doctor"
	"github.com/kvetinski/paraflow/labctl-go/internal/protocol"
	"github.com/kvetinski/paraflow/labctl-go/internal/worker"
)

const help = `ParaFlow experiment controller

Usage:
  labctl run --engine <path> <workload.json>
  labctl doctor [--json]
  labctl version
  labctl help

The run command executes one workload through the versioned Rust worker
protocol. It verifies correctness but does not collect timing samples.`

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

// Dependencies contains side effects injected into command dispatch.
type Dependencies struct {
	Build       BuildInfo
	Probe       doctor.Probe
	ReadFile    func(string) ([]byte, error)
	StartWorker StartWorker
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
