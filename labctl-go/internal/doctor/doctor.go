// Package doctor inspects whether a host can build the current ParaFlow
// milestone and reports future accelerator tools without requiring them.
package doctor

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const reportSchema = "paraflow.environment/v1"

// Tool describes one executable used during the twelve-week project.
type Tool struct {
	Name        string
	Command     string
	VersionArgs []string
	Required    bool
	Introduced  string
	Purpose     string
}

// ToolResult records one tool probe.
type ToolResult struct {
	Name       string `json:"name"`
	Required   bool   `json:"required_now"`
	Introduced string `json:"introduced"`
	Purpose    string `json:"purpose"`
	Available  bool   `json:"available"`
	Path       string `json:"path,omitempty"`
	Version    string `json:"version,omitempty"`
	Problem    string `json:"problem,omitempty"`
}

// Report is stable environment metadata for the current milestone.
type Report struct {
	SchemaVersion string       `json:"schema_version"`
	OS            string       `json:"os"`
	Architecture  string       `json:"architecture"`
	LogicalCPUs   int          `json:"logical_cpus"`
	GoVersion     string       `json:"go_version"`
	Ready         bool         `json:"ready_for_day_1"`
	Tools         []ToolResult `json:"tools"`
}

// Probe makes tool discovery injectable for deterministic tests.
type Probe func(context.Context, Tool) ToolResult

var tools = []Tool{
	{
		Name:        "git",
		Command:     "git",
		VersionArgs: []string{"--version"},
		Required:    true,
		Introduced:  "day-01",
		Purpose:     "source history and reproducibility identity",
	},
	{
		Name:        "go",
		Command:     "go",
		VersionArgs: []string{"version"},
		Required:    true,
		Introduced:  "day-01",
		Purpose:     "experiment control plane",
	},
	{
		Name:        "rustc",
		Command:     "rustc",
		VersionArgs: []string{"--version"},
		Required:    true,
		Introduced:  "day-01",
		Purpose:     "execution engine compiler",
	},
	{
		Name:        "cargo",
		Command:     "cargo",
		VersionArgs: []string{"--version"},
		Required:    true,
		Introduced:  "day-01",
		Purpose:     "Rust build and test orchestration",
	},
	{
		Name:        "c++",
		Command:     "c++",
		VersionArgs: []string{"--version"},
		Required:    false,
		Introduced:  "week-02",
		Purpose:     "native scalar, SIMD, and CUDA host kernels",
	},
	{
		Name:        "ispc",
		Command:     "ispc",
		VersionArgs: []string{"--version"},
		Required:    false,
		Introduced:  "week-02",
		Purpose:     "SPMD-to-SIMD kernels",
	},
	{
		Name:        "nvcc",
		Command:     "nvcc",
		VersionArgs: []string{"--version"},
		Required:    false,
		Introduced:  "week-09",
		Purpose:     "CUDA backend",
	},
}

// Check probes the host and determines whether Day 1 development can proceed.
func Check(ctx context.Context, probe Probe) Report {
	results := make([]ToolResult, 0, len(tools))
	ready := true

	for _, tool := range tools {
		result := probe(ctx, tool)
		results = append(results, result)
		if tool.Required && !result.Available {
			ready = false
		}
	}

	return Report{
		SchemaVersion: reportSchema,
		OS:            runtime.GOOS,
		Architecture:  runtime.GOARCH,
		LogicalCPUs:   runtime.NumCPU(),
		GoVersion:     runtime.Version(),
		Ready:         ready,
		Tools:         results,
	}
}

// CommandProbe discovers an executable and captures its version output.
func CommandProbe(ctx context.Context, tool Tool) ToolResult {
	result := ToolResult{
		Name:       tool.Name,
		Required:   tool.Required,
		Introduced: tool.Introduced,
		Purpose:    tool.Purpose,
	}

	path, err := exec.LookPath(tool.Command)
	if err != nil {
		result.Problem = err.Error()
		return result
	}
	result.Available = true
	result.Path = path

	probeContext, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	output, err := exec.CommandContext(
		probeContext,
		path,
		tool.VersionArgs...,
	).CombinedOutput()
	if err != nil {
		result.Problem = err.Error()
		return result
	}
	result.Version = firstLine(string(output))
	return result
}

// String renders a compact human-readable report.
func (report Report) String() string {
	var builder strings.Builder
	_, _ = fmt.Fprintf(
		&builder,
		"ParaFlow environment\nOS/arch: %s/%s\nLogical CPUs: %d\nGo runtime: %s\n\n",
		report.OS,
		report.Architecture,
		report.LogicalCPUs,
		report.GoVersion,
	)

	for _, tool := range report.Tools {
		status := "missing"
		if tool.Available {
			status = "ok"
		}
		requirement := "planned"
		if tool.Required {
			requirement = "required"
		}
		version := tool.Version
		if version == "" {
			version = tool.Problem
		}
		_, _ = fmt.Fprintf(
			&builder,
			"[%s] %-5s %-8s %-7s %s\n",
			status,
			tool.Name,
			requirement,
			tool.Introduced,
			version,
		)
	}

	_, _ = fmt.Fprintf(&builder, "\nReady for Day 1: %t", report.Ready)
	return builder.String()
}

func firstLine(value string) string {
	value = strings.TrimSpace(value)
	if newline := strings.IndexByte(value, '\n'); newline >= 0 {
		return strings.TrimSpace(value[:newline])
	}
	return value
}
