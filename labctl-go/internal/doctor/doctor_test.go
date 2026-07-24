package doctor

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func availableProbe(_ context.Context, tool Tool) ToolResult {
	return ToolResult{
		Name:       tool.Name,
		Required:   tool.Required,
		Introduced: tool.Introduced,
		Purpose:    tool.Purpose,
		Available:  true,
		Path:       "/tools/" + tool.Command,
		Version:    tool.Name + " test-version",
	}
}

func TestCheckIsReadyWhenRequiredToolsExist(t *testing.T) {
	t.Parallel()

	report := Check(context.Background(), availableProbe)

	if !report.Ready {
		t.Fatal("expected report to be ready")
	}
	if report.LogicalCPUs < 1 {
		t.Fatalf("logical CPU count must be positive, got %d", report.LogicalCPUs)
	}
	if len(report.Tools) != len(tools) {
		t.Fatalf("expected %d tool results, got %d", len(tools), len(report.Tools))
	}
}

func TestCheckFailsOnlyForMissingRequiredTool(t *testing.T) {
	t.Parallel()

	probe := func(ctx context.Context, tool Tool) ToolResult {
		result := availableProbe(ctx, tool)
		switch tool.Name {
		case "cargo":
			result.Available = false
			result.Path = ""
			result.Version = ""
			result.Problem = errors.New("not found").Error()
		case "nvcc":
			result.Available = false
			result.Path = ""
			result.Version = ""
			result.Problem = errors.New("not found").Error()
		}
		return result
	}

	report := Check(context.Background(), probe)

	if report.Ready {
		t.Fatal("missing required cargo must make the report unready")
	}
}

func TestReportStringLabelsRequirements(t *testing.T) {
	t.Parallel()

	report := Check(context.Background(), availableProbe)
	output := report.String()

	for _, expected := range []string{
		"ParaFlow environment",
		"go    required",
		"nvcc  planned",
		"Ready for Day 1: true",
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
