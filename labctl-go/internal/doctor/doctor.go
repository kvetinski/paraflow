// Package doctor inspects whether a host can build the current ParaFlow
// milestone and reports future accelerator tools without requiring them.
package doctor

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/kvetinski/paraflow/labctl-go/internal/buildinfo"
)

const (
	reportSchema        = "paraflow.environment/v3"
	currentMilestone    = "day-07"
	defaultProbeTimeout = 2 * time.Second
)

var numericVersionPattern = regexp.MustCompile(
	`(?:go)?([0-9]+)\.([0-9]+)(?:\.([0-9]+))?`,
)

// Tool describes one executable used during the twelve-week project.
type Tool struct {
	Name            string
	Command         string
	VersionArgs     []string
	Required        bool
	Minimum         string
	VersionContains string
	Introduced      string
	Purpose         string
}

// ToolResult records one tool probe.
type ToolResult struct {
	Name           string `json:"name"`
	Required       bool   `json:"required_now"`
	MinimumVersion string `json:"minimum_version,omitempty"`
	Introduced     string `json:"introduced"`
	Purpose        string `json:"purpose"`
	Found          bool   `json:"found"`
	Usable         bool   `json:"usable"`
	Path           string `json:"path,omitempty"`
	Version        string `json:"version,omitempty"`
	Problem        string `json:"problem,omitempty"`
}

// Report is stable environment metadata for the current milestone.
type Report struct {
	SchemaVersion string         `json:"schema_version"`
	Milestone     string         `json:"milestone"`
	CapturedAt    time.Time      `json:"captured_at"`
	OS            string         `json:"os"`
	KernelVersion string         `json:"kernel_version"`
	Architecture  string         `json:"architecture"`
	CPUModel      string         `json:"cpu_model"`
	PhysicalCores int            `json:"physical_cores"`
	LogicalCPUs   int            `json:"logical_cpus"`
	GoMaxProcs    int            `json:"gomaxprocs"`
	GoVersion     string         `json:"go_version"`
	Source        buildinfo.Info `json:"source"`
	Ready         bool           `json:"ready"`
	Tools         []ToolResult   `json:"tools"`
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
		Minimum:     "1.24.0",
		Introduced:  "day-01",
		Purpose:     "experiment control plane",
	},
	{
		Name:        "rustc",
		Command:     "rustc",
		VersionArgs: []string{"--version"},
		Required:    true,
		Minimum:     "1.97.1",
		Introduced:  "day-01",
		Purpose:     "execution engine compiler",
	},
	{
		Name:        "cargo",
		Command:     "cargo",
		VersionArgs: []string{"--version"},
		Required:    true,
		Minimum:     "1.97.1",
		Introduced:  "day-01",
		Purpose:     "Rust build and test orchestration",
	},
	{
		Name:        "cc",
		Command:     "cc",
		VersionArgs: []string{"--version"},
		Required:    true,
		Introduced:  "day-01",
		Purpose:     "C toolchain required by Go race-detector tests",
	},
	{
		Name:        "node",
		Command:     "node",
		VersionArgs: []string{"--version"},
		Required:    true,
		Minimum:     "20.0.0",
		Introduced:  "day-01",
		Purpose:     "Draft 2020-12 contract validation",
	},
	{
		Name:        "npm",
		Command:     "npm",
		VersionArgs: []string{"--version"},
		Required:    true,
		Introduced:  "day-01",
		Purpose:     "reproducible schema-validator installation",
	},
	{
		Name:        "rustfmt",
		Command:     "rustfmt",
		VersionArgs: []string{"--version"},
		Required:    true,
		Introduced:  "day-01",
		Purpose:     "Rust formatting gate",
	},
	{
		Name:        "clippy",
		Command:     "cargo",
		VersionArgs: []string{"clippy", "--version"},
		Required:    true,
		Introduced:  "day-01",
		Purpose:     "Rust lint gate",
	},
	{
		Name:        "bash",
		Command:     "bash",
		VersionArgs: []string{"--version"},
		Required:    true,
		Minimum:     "4.0.0",
		Introduced:  "day-01",
		Purpose:     "repository scripts and workload discovery",
	},
	{
		Name:            "make",
		Command:         "make",
		VersionArgs:     []string{"--version"},
		Required:        true,
		VersionContains: "GNU Make",
		Introduced:      "day-01",
		Purpose:         "repository quality-gate orchestration",
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

// Check probes the host and determines whether the current milestone can proceed.
func Check(ctx context.Context, probe Probe, source buildinfo.Info) Report {
	results := make([]ToolResult, 0, len(tools))
	ready := true

	for _, tool := range tools {
		result := assess(tool, probe(ctx, tool))
		results = append(results, result)
		if tool.Required && !result.Usable {
			ready = false
		}
	}

	return Report{
		SchemaVersion: reportSchema,
		Milestone:     currentMilestone,
		CapturedAt:    time.Now().UTC(),
		OS:            runtime.GOOS,
		KernelVersion: captureKernelVersion(ctx),
		Architecture:  runtime.GOARCH,
		CPUModel:      captureCPUModel(ctx),
		PhysicalCores: capturePhysicalCores(ctx),
		LogicalCPUs:   runtime.NumCPU(),
		GoMaxProcs:    runtime.GOMAXPROCS(0),
		GoVersion:     runtime.Version(),
		Source:        source,
		Ready:         ready,
		Tools:         results,
	}
}

// CommandProbe discovers an executable and captures its version output.
func CommandProbe(ctx context.Context, tool Tool) ToolResult {
	return commandProbe(ctx, tool, defaultProbeTimeout)
}

func commandProbe(ctx context.Context, tool Tool, timeout time.Duration) ToolResult {
	result := ToolResult{
		Name:           tool.Name,
		Required:       tool.Required,
		MinimumVersion: tool.Minimum,
		Introduced:     tool.Introduced,
		Purpose:        tool.Purpose,
	}

	path, err := exec.LookPath(tool.Command)
	if err != nil {
		result.Problem = err.Error()
		return assess(tool, result)
	}
	result.Found = true
	result.Path = path

	probeContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	output, err := exec.CommandContext(
		probeContext,
		path,
		tool.VersionArgs...,
	).CombinedOutput()
	outputLine := versionLine(string(output))
	if err != nil {
		switch {
		case errors.Is(probeContext.Err(), context.DeadlineExceeded):
			result.Problem = fmt.Sprintf("version probe timed out after %s", timeout)
		case errors.Is(probeContext.Err(), context.Canceled):
			result.Problem = "version probe canceled"
		default:
			result.Problem = fmt.Sprintf("version command failed: %v", err)
			if outputLine != "" {
				result.Problem += ": " + outputLine
			}
		}
		return assess(tool, result)
	}
	result.Version = outputLine
	return assess(tool, result)
}

// String renders a compact human-readable report.
func (report Report) String() string {
	var builder strings.Builder
	_, _ = fmt.Fprintf(
		&builder,
		"ParaFlow environment\nOS/arch: %s/%s\nPhysical/logical CPUs: %d/%d\nGo runtime: %s\n\n",
		report.OS,
		report.Architecture,
		report.PhysicalCores,
		report.LogicalCPUs,
		report.GoVersion,
	)
	_, _ = fmt.Fprintf(
		&builder,
		"Captured: %s\nKernel: %s\nCPU: %s\nGOMAXPROCS: %d\nSource: %s\n\n",
		report.CapturedAt.Format(time.RFC3339),
		report.KernelVersion,
		report.CPUModel,
		report.GoMaxProcs,
		report.Source,
	)

	for _, tool := range report.Tools {
		status := "missing"
		if tool.Found {
			status = "broken"
		}
		if tool.Usable {
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
			"[%s] %-7s %-8s %-7s %s\n",
			status,
			tool.Name,
			requirement,
			tool.Introduced,
			version,
		)
	}

	_, _ = fmt.Fprintf(
		&builder,
		"\nReady for current milestone (%s): %t",
		report.Milestone,
		report.Ready,
	)
	return builder.String()
}

func firstLine(value string) string {
	value = strings.TrimSpace(value)
	if newline := strings.IndexByte(value, '\n'); newline >= 0 {
		return strings.TrimSpace(value[:newline])
	}
	return value
}

func versionLine(value string) string {
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if numericVersionPattern.MatchString(line) {
			return line
		}
	}
	return firstLine(value)
}

func assess(tool Tool, result ToolResult) ToolResult {
	result.Name = tool.Name
	result.Required = tool.Required
	result.MinimumVersion = tool.Minimum
	result.Introduced = tool.Introduced
	result.Purpose = tool.Purpose
	result.Usable = false

	if !result.Found || result.Problem != "" {
		return result
	}
	if strings.TrimSpace(result.Version) == "" {
		result.Problem = "version command returned no output"
		return result
	}
	if tool.VersionContains != "" && !strings.Contains(result.Version, tool.VersionContains) {
		result.Problem = fmt.Sprintf(
			"version output must contain %q; found %q",
			tool.VersionContains,
			result.Version,
		)
		return result
	}
	if tool.Minimum != "" {
		actual, err := parseNumericVersion(result.Version)
		if err != nil {
			result.Problem = fmt.Sprintf("cannot parse tool version: %v", err)
			return result
		}
		minimum, err := parseNumericVersion(tool.Minimum)
		if err != nil {
			result.Problem = fmt.Sprintf("invalid minimum version %q: %v", tool.Minimum, err)
			return result
		}
		if actual.lessThan(minimum) {
			result.Problem = fmt.Sprintf(
				"requires >= %s; found %s",
				minimum,
				actual,
			)
			return result
		}
	}

	result.Usable = true
	return result
}

type numericVersion struct {
	major int
	minor int
	patch int
}

func parseNumericVersion(value string) (numericVersion, error) {
	match := numericVersionPattern.FindStringSubmatch(value)
	if match == nil {
		return numericVersion{}, fmt.Errorf("no numeric version in %q", value)
	}

	parts := [3]int{}
	for index := range parts {
		if match[index+1] == "" {
			continue
		}
		parsed, err := strconv.Atoi(match[index+1])
		if err != nil {
			return numericVersion{}, fmt.Errorf("parse component %q: %w", match[index+1], err)
		}
		parts[index] = parsed
	}
	return numericVersion{major: parts[0], minor: parts[1], patch: parts[2]}, nil
}

func (version numericVersion) lessThan(other numericVersion) bool {
	if version.major != other.major {
		return version.major < other.major
	}
	if version.minor != other.minor {
		return version.minor < other.minor
	}
	return version.patch < other.patch
}

func (version numericVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", version.major, version.minor, version.patch)
}
