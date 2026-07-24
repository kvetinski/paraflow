package app

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kvetinski/paraflow/labctl-go/internal/buildinfo"
	"github.com/kvetinski/paraflow/labctl-go/internal/doctor"
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
	if report.Source != testDependencies().Build {
		t.Fatalf("unexpected source identity: %#v", report.Source)
	}
	if report.CapturedAt.IsZero() {
		t.Fatal("doctor report must include its capture time")
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
