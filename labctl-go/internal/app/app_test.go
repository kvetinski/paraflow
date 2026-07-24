package app

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kvetinski/paraflow/labctl-go/internal/doctor"
)

func availableProbe(_ context.Context, tool doctor.Tool) doctor.ToolResult {
	return doctor.ToolResult{
		Name:       tool.Name,
		Required:   tool.Required,
		Introduced: tool.Introduced,
		Purpose:    tool.Purpose,
		Available:  true,
		Path:       "/tools/" + tool.Command,
		Version:    "test-version",
	}
}

func testDependencies() Dependencies {
	return Dependencies{
		Build: BuildInfo{Version: "test", Commit: "abc123"},
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
	if got := strings.TrimSpace(stdout.String()); got != "labctl test (commit abc123)" {
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
