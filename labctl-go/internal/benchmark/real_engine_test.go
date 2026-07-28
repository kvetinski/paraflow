package benchmark

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kvetinski/paraflow/labctl-go/internal/protocol"
)

func TestRealEngineRunsWarmupsAndSamplesInsideOneProcess(t *testing.T) {
	enginePath := os.Getenv("PARAFLOW_ENGINE_PATH")
	repositoryRoot := os.Getenv("PARAFLOW_REPOSITORY_ROOT")
	if enginePath == "" || repositoryRoot == "" {
		t.Skip("real release engine path and repository root are not configured")
	}

	workload, err := os.ReadFile(filepath.Join(
		repositoryRoot,
		"workloads",
		"bench-uniform-1k-v1.json",
	))
	if err != nil {
		t.Fatalf("read workload: %v", err)
	}
	projection, err := protocol.ProjectWorkload(workload)
	if err != nil {
		t.Fatalf("project workload: %v", err)
	}
	request := Request{
		SchemaVersion: RequestSchema,
		ExperimentID:  "day05:real-process",
		ScenarioName:  "real-process-smoke",
		Execution:     Execution{Backend: "scalar"},
		Sampling:      Sampling{WarmupIterations: 1, SampleIterations: 3},
		Workload:      json.RawMessage(workload),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, orchestration, err := RunEngine(ctx, enginePath, request, projection)
	if err != nil {
		t.Fatalf("RunEngine() error = %v", err)
	}
	if len(result.Samples) != 3 {
		t.Fatalf("sample count = %d, want 3", len(result.Samples))
	}
	if orchestration.Uint64() < result.Timing.ExperimentTotalNS.Uint64() {
		t.Fatalf(
			"orchestration %d is smaller than engine experiment %d",
			orchestration,
			result.Timing.ExperimentTotalNS,
		)
	}
}
