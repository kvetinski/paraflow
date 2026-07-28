package benchmark

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kvetinski/paraflow/labctl-go/internal/jsoncheck"
	"github.com/kvetinski/paraflow/labctl-go/internal/protocol"
)

type benchmarkVectorFixture struct {
	SchemaVersion string                `json:"schema_version"`
	Cases         []benchmarkVectorCase `json:"cases"`
}

type benchmarkVectorCase struct {
	Name         string       `json:"name"`
	Request      Request      `json:"request"`
	EngineResult EngineResult `json:"engine_result"`
}

func TestSharedBenchmarkVectorsDecodeAndValidateInGo(t *testing.T) {
	t.Parallel()

	var fixture benchmarkVectorFixture
	decodeRepositoryFixture(t, "contracts/conformance/benchmark-v1.json", &fixture)
	if fixture.SchemaVersion != "paraflow.benchmark-vectors/v1" {
		t.Fatalf("schema_version = %q", fixture.SchemaVersion)
	}
	if len(fixture.Cases) == 0 {
		t.Fatal("benchmark vector fixture must contain at least one case")
	}

	for _, vector := range fixture.Cases {
		vector := vector
		t.Run(vector.Name, func(t *testing.T) {
			t.Parallel()

			projection, err := protocol.ProjectWorkload(vector.Request.Workload)
			if err != nil {
				t.Fatalf("ProjectWorkload() error = %v", err)
			}
			if err := validateEngineResult(vector.EngineResult, vector.Request, projection); err != nil {
				t.Fatalf("validateEngineResult() error = %v", err)
			}
		})
	}
}

func TestSharedCompleteCaptureFixtureDecodesStrictlyInGo(t *testing.T) {
	t.Parallel()

	var capture Capture
	decodeRepositoryFixture(t, "contracts/conformance/benchmark-capture-v1.json", &capture)
	if capture.SchemaVersion != CaptureSchema {
		t.Fatalf("schema_version = %q", capture.SchemaVersion)
	}
	if len(capture.Experiments) != 1 {
		t.Fatalf("experiments = %d, want 1", len(capture.Experiments))
	}
	if len(capture.Experiments[0].EngineResult.Samples) != 2 {
		t.Fatalf(
			"retained samples = %d, want 2",
			len(capture.Experiments[0].EngineResult.Samples),
		)
	}
}

func decodeRepositoryFixture(t *testing.T, repositoryPath string, target any) {
	t.Helper()

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() could not locate the test source")
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", ".."))
	data, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(repositoryPath)))
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", repositoryPath, err)
	}
	if err := jsoncheck.Decode(data, target, true); err != nil {
		t.Fatalf("decode %q: %v", repositoryPath, err)
	}
}
