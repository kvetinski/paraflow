// Package app contains labctl command dispatch independently from process I/O.
package app

import (
	"context"
	"fmt"
	"io"
)

const help = `ParaFlow experiment controller

Usage:
  labctl version
  labctl help

Environment diagnostics are introduced in the Day 1 tooling milestone.
Experiment execution begins after the scalar Rust engine exists.`

// BuildInfo identifies the controller binary.
type BuildInfo struct {
	Version string
	Commit  string
}

// Run dispatches a labctl command and returns a process-compatible exit code.
func Run(
	_ context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	build BuildInfo,
) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stdout, help)
		return 0
	}

	switch args[0] {
	case "help", "--help", "-h":
		_, _ = fmt.Fprintln(stdout, help)
		return 0
	case "version", "--version":
		_, _ = fmt.Fprintf(
			stdout,
			"labctl %s (commit %s)\n",
			build.Version,
			build.Commit,
		)
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "error: unknown command %q\n", args[0])
		_, _ = fmt.Fprintln(stderr, "run 'labctl help' for usage")
		return 2
	}
}
