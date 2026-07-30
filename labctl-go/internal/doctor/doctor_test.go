package doctor

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kvetinski/paraflow/labctl-go/internal/buildinfo"
)

var testSource = buildinfo.Info{
	Version:     "test",
	FullCommit:  "0123456789abcdef0123456789abcdef01234567",
	SourceState: buildinfo.SourceClean,
}

func availableProbe(_ context.Context, tool Tool) ToolResult {
	return ToolResult{
		Name:     tool.Name,
		Required: tool.Required,
		Found:    true,
		Path:     "/tools/" + tool.Command,
		Version:  compatibleVersion(tool),
	}
}

func compatibleVersion(tool Tool) string {
	switch tool.Name {
	case "go":
		return "go version go1.24.0 linux/amd64"
	case "rustc":
		return "rustc 1.97.1 (test)"
	case "cargo":
		return "cargo 1.97.1 (test)"
	case "node":
		return "v20.0.0"
	case "rustfmt":
		return "rustfmt 1.8.0-stable (test)"
	case "bash":
		return "GNU bash, version 5.0.0 (test)"
	case "make":
		return "GNU Make 4.4"
	default:
		return tool.Name + " version 1.0.0"
	}
}

func TestCheckIsReadyWhenRequiredToolsAreUsable(t *testing.T) {
	t.Parallel()

	report := Check(context.Background(), availableProbe, testSource)

	if !report.Ready {
		t.Fatal("expected report to be ready")
	}
	if report.SchemaVersion != "paraflow.environment/v3" {
		t.Fatalf("unexpected report schema: %q", report.SchemaVersion)
	}
	if report.Milestone != "day-07" {
		t.Fatalf("unexpected milestone: %q", report.Milestone)
	}
	if report.CapturedAt.IsZero() {
		t.Fatal("captured timestamp must be present")
	}
	if report.KernelVersion == "" {
		t.Fatal("kernel version must be explicit")
	}
	if report.CPUModel == "" {
		t.Fatal("CPU model must be explicit")
	}
	if report.LogicalCPUs < 1 {
		t.Fatalf("logical CPU count must be positive, got %d", report.LogicalCPUs)
	}
	if report.GoMaxProcs < 1 {
		t.Fatalf("GOMAXPROCS must be positive, got %d", report.GoMaxProcs)
	}
	if report.Source != testSource {
		t.Fatalf("unexpected source identity: %#v", report.Source)
	}
	if len(report.Tools) != len(tools) {
		t.Fatalf("expected %d tool results, got %d", len(tools), len(report.Tools))
	}
}

func TestCheckFailsForMissingRequiredToolButNotMissingPlannedTool(t *testing.T) {
	t.Parallel()

	requiredMissing := func(ctx context.Context, tool Tool) ToolResult {
		result := availableProbe(ctx, tool)
		if tool.Name == "cargo" {
			result.Found = false
			result.Path = ""
			result.Version = ""
			result.Problem = "not found"
		}
		if tool.Name == "nvcc" {
			result.Found = false
			result.Path = ""
			result.Version = ""
			result.Problem = "not found"
		}
		return result
	}

	report := Check(context.Background(), requiredMissing, testSource)
	if report.Ready {
		t.Fatal("missing required cargo must make the report unready")
	}

	optionalMissing := func(ctx context.Context, tool Tool) ToolResult {
		result := availableProbe(ctx, tool)
		if !tool.Required {
			result.Found = false
			result.Path = ""
			result.Version = ""
			result.Problem = "not found"
		}
		return result
	}
	report = Check(context.Background(), optionalMissing, testSource)
	if !report.Ready {
		t.Fatal("missing planned tools must not make the report unready")
	}
}

func TestCommandProbeMarksNonzeroVersionCommandUnusable(t *testing.T) {
	t.Parallel()

	tool := helperTool("cc", "", "exit", "compiler failed")
	result := commandProbe(context.Background(), tool, 5*time.Second)

	if !result.Found {
		t.Fatal("helper executable should be found")
	}
	if result.Usable {
		t.Fatal("nonzero version command must not be usable")
	}
	if !strings.Contains(result.Problem, "version command failed") {
		t.Fatalf("unexpected problem: %q", result.Problem)
	}
}

func TestCommandProbeMarksTimeoutUnusable(t *testing.T) {
	t.Parallel()

	tool := helperTool("cc", "", "sleep", "")
	result := commandProbe(context.Background(), tool, 25*time.Millisecond)

	if !result.Found {
		t.Fatal("helper executable should be found")
	}
	if result.Usable {
		t.Fatal("timed-out version command must not be usable")
	}
	if !strings.Contains(result.Problem, "timed out") {
		t.Fatalf("unexpected problem: %q", result.Problem)
	}
}

func TestCommandProbeRejectsOutdatedRequiredVersion(t *testing.T) {
	t.Parallel()

	tool := helperTool(
		"go",
		"1.24.0",
		"version",
		"go version go1.23.9 linux/amd64",
	)
	result := commandProbe(context.Background(), tool, 5*time.Second)

	if !result.Found {
		t.Fatal("helper executable should be found")
	}
	if result.Usable {
		t.Fatal("outdated Go must not be usable")
	}
	if !strings.Contains(result.Problem, "requires >= 1.24.0; found 1.23.9") {
		t.Fatalf("unexpected problem: %q", result.Problem)
	}
}

func TestCommandProbeAcceptsMinimumRequiredVersion(t *testing.T) {
	t.Parallel()

	tool := helperTool(
		"rustc",
		"1.97.1",
		"version",
		"rustc 1.97.1 (test)",
	)
	result := commandProbe(context.Background(), tool, 5*time.Second)

	if !result.Found || !result.Usable {
		t.Fatalf("minimum supported version should be usable: %#v", result)
	}
}

func TestCommandProbeRejectsUnexpectedToolFamily(t *testing.T) {
	t.Parallel()

	tool := helperTool("make", "", "version", "BSD make 1.0")
	tool.VersionContains = "GNU Make"
	result := commandProbe(context.Background(), tool, 5*time.Second)

	if result.Usable {
		t.Fatal("unexpected make implementation must not be usable")
	}
	if !strings.Contains(result.Problem, `must contain "GNU Make"`) {
		t.Fatalf("unexpected problem: %q", result.Problem)
	}
}

func TestReportStringLabelsRequirementsAndSource(t *testing.T) {
	t.Parallel()

	report := Check(context.Background(), availableProbe, testSource)
	output := report.String()

	for _, expected := range []string{
		"ParaFlow environment",
		"go      required",
		"cc      required",
		"nvcc    planned",
		"Source: test (commit 0123456789abcdef0123456789abcdef01234567, source clean)",
		"Ready for current milestone (day-07): true",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected output to contain %q:\n%s", expected, output)
		}
	}
}

func TestFirstLineNormalizesVersionOutput(t *testing.T) {
	t.Parallel()

	got := firstLine("compiler 1.2.3\nCopyright example\n")
	if got != "compiler 1.2.3" {
		t.Fatalf("unexpected first line: %q", got)
	}
}

func TestVersionLineSkipsWarningsBeforeVersion(t *testing.T) {
	t.Parallel()

	got := versionLine("npm warn unknown configuration\n11.9.0\n")
	if got != "11.9.0" {
		t.Fatalf("unexpected version line: %q", got)
	}
}

func helperTool(name, minimum, mode, value string) Tool {
	return Tool{
		Name:        name,
		Command:     os.Args[0],
		VersionArgs: []string{"-test.run=^TestProbeHelperProcess$", "--", mode, value},
		Required:    true,
		Minimum:     minimum,
		Introduced:  "test",
		Purpose:     "test helper",
	}
}

// TestProbeHelperProcess is executed as a child process by command-probe tests.
func TestProbeHelperProcess(t *testing.T) {
	separator := -1
	for index, argument := range os.Args {
		if argument == "--" {
			separator = index
			break
		}
	}
	if separator < 0 || len(os.Args) <= separator+1 {
		return
	}

	mode := os.Args[separator+1]
	value := ""
	if len(os.Args) > separator+2 {
		value = os.Args[separator+2]
	}

	switch mode {
	case "version":
		_, _ = fmt.Fprintln(os.Stdout, value)
	case "exit":
		_, _ = fmt.Fprintln(os.Stderr, value)
		os.Exit(7)
	case "sleep":
		time.Sleep(5 * time.Second)
	default:
		os.Exit(8)
	}
}

func TestLinuxPhysicalCoreCountDeduplicatesSMTSiblings(t *testing.T) {
	t.Parallel()

	cpuinfo := "" +
		"processor : 0\nphysical id : 0\ncore id : 0\n\n" +
		"processor : 1\nphysical id : 0\ncore id : 0\n\n" +
		"processor : 2\nphysical id : 0\ncore id : 1\n\n" +
		"processor : 3\nphysical id : 1\ncore id : 0\n"
	if got := linuxPhysicalCoreCount(strings.NewReader(cpuinfo)); got != 3 {
		t.Fatalf("linuxPhysicalCoreCount() = %d, want 3", got)
	}
}
