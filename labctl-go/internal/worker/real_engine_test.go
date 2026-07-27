package worker

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kvetinski/paraflow/labctl-go/internal/protocol"
)

const (
	realEnginePathEnvironment = "PARAFLOW_ENGINE_PATH"
	repositoryRootEnvironment = "PARAFLOW_REPOSITORY_ROOT"
)

func TestRealEngineSessionReusesOneProcess(t *testing.T) {
	enginePath := os.Getenv(realEnginePathEnvironment)
	repositoryRoot := os.Getenv(repositoryRootEnvironment)
	if enginePath == "" || repositoryRoot == "" {
		t.Skip(
			"set PARAFLOW_ENGINE_PATH and PARAFLOW_REPOSITORY_ROOT " +
				"to run release interop",
		)
	}

	empty := readRealWorkload(t, repositoryRoot, "edge-empty-v1.json")
	scalar := readRealWorkload(t, repositoryRoot, "edge-scalar-v1.json")
	invalid := invalidCategoryCount(t, empty)

	session, err := Start(context.Background(), enginePath)
	if err != nil {
		t.Fatalf("Start(real engine) error = %v", err)
	}
	t.Cleanup(func() {
		_ = session.Close()
	})

	emptyResult, err := session.Execute(context.Background(), empty)
	if err != nil {
		t.Fatalf("Execute(empty) error = %v", err)
	}
	if emptyResult.AcceptedCount != 0 ||
		!slices.Equal(emptyResult.CategoryHistogram, []uint64{0}) {
		t.Fatalf("empty result = %#v", emptyResult)
	}

	_, err = session.Execute(context.Background(), invalid)
	var remoteError *protocol.RemoteError
	if !errors.As(err, &remoteError) {
		t.Fatalf("Execute(invalid) error = %T %v, want RemoteError", err, err)
	}
	if remoteError.Code != "invalid_workload" || len(remoteError.Issues) == 0 {
		t.Fatalf("invalid workload error = %#v", remoteError)
	}

	scalarResult, err := session.Execute(context.Background(), scalar)
	if err != nil {
		t.Fatalf("Execute(scalar after error) error = %v", err)
	}
	if scalarResult.AcceptedCount != 3 ||
		math.Float64bits(scalarResult.ScoreSum) != 0x401a000000000000 ||
		!slices.Equal(
			scalarResult.CategoryHistogram,
			[]uint64{1, 1, 1, 0},
		) ||
		scalarResult.AcceptedIDSum != 0x10 ||
		scalarResult.AcceptedIDXOR != 0x6ebb399a18884447 {
		t.Fatalf("scalar result = %#v", scalarResult)
	}

	if err := session.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown(real engine) error = %v", err)
	}
}

func readRealWorkload(
	t *testing.T,
	repositoryRoot string,
	name string,
) json.RawMessage {
	t.Helper()
	path := filepath.Join(repositoryRoot, "workloads", name)
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return source
}

func invalidCategoryCount(
	t *testing.T,
	source json.RawMessage,
) json.RawMessage {
	t.Helper()
	var workload map[string]any
	if err := json.Unmarshal(source, &workload); err != nil {
		t.Fatalf("decode workload for mutation: %v", err)
	}
	dataset, ok := workload["dataset"].(map[string]any)
	if !ok {
		t.Fatal("workload dataset is not an object")
	}
	dataset["category_count"] = 0
	invalid, err := json.Marshal(workload)
	if err != nil {
		t.Fatalf("encode invalid workload: %v", err)
	}
	return invalid
}
