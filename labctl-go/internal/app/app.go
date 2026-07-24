// Package app contains labctl command dispatch independently from process I/O.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/kvetinski/paraflow/labctl-go/internal/buildinfo"
	"github.com/kvetinski/paraflow/labctl-go/internal/doctor"
)

const help = `ParaFlow experiment controller

Usage:
  labctl doctor [--json]
  labctl version
  labctl help

Experiment execution begins after the scalar Rust engine exists.`

// BuildInfo identifies the controller binary.
type BuildInfo = buildinfo.Info

// Dependencies contains side effects injected into command dispatch.
type Dependencies struct {
	Build BuildInfo
	Probe doctor.Probe
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
	default:
		return usageError(stderr, fmt.Sprintf("unknown command %q", args[0]))
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

func write(output io.Writer, message string) int {
	if _, err := fmt.Fprintln(output, message); err != nil {
		return 1
	}
	return 0
}
