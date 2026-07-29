package benchmark

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kvetinski/paraflow/labctl-go/internal/jsoncheck"
	"github.com/kvetinski/paraflow/labctl-go/internal/protocol"
)

// ProfileEngineRunner executes one diagnostic profile scenario in a fresh Rust
// process. The process owns all warm-ups and retained stage samples.
type ProfileEngineRunner func(
	context.Context,
	string,
	ProfileRequest,
	protocol.WorkloadProjection,
) (ProfileEngineResult, Nanoseconds, error)

// RunProfileEngine starts `paraflow-engine profile`, strictly validates its
// response, and reports complete Go-side orchestration separately.
func RunProfileEngine(
	ctx context.Context,
	enginePath string,
	request ProfileRequest,
	workload protocol.WorkloadProjection,
) (ProfileEngineResult, Nanoseconds, error) {
	started := time.Now()
	responseBytes, diagnostics, err := runOneShotEngine(
		ctx,
		enginePath,
		"profile",
		request,
	)
	if err != nil {
		return ProfileEngineResult{}, 0, err
	}

	var result ProfileEngineResult
	if err := jsoncheck.Decode(responseBytes, &result, true); err != nil {
		return ProfileEngineResult{}, 0, processError(
			"decode profile response",
			err,
			diagnostics,
		)
	}
	if err := validateProfileEngineResult(result, request, workload); err != nil {
		return ProfileEngineResult{}, 0, processError(
			"validate profile response",
			err,
			diagnostics,
		)
	}

	orchestration := Nanoseconds(durationNanoseconds(time.Since(started)))
	if orchestration.Uint64() < result.Timing.ExperimentTotalNS.Uint64() {
		return ProfileEngineResult{}, 0, errors.New(
			"profile orchestration_total_ns is smaller than engine experiment_total_ns",
		)
	}
	return result, orchestration, nil
}

func validateProfileEngineResult(
	result ProfileEngineResult,
	request ProfileRequest,
	workload protocol.WorkloadProjection,
) error {
	switch {
	case result.SchemaVersion != ProfileEngineResultSchema:
		return fmt.Errorf("unexpected schema_version %q", result.SchemaVersion)
	case result.ExperimentID != request.ExperimentID:
		return fmt.Errorf(
			"experiment_id mismatch: got %q, want %q",
			result.ExperimentID,
			request.ExperimentID,
		)
	case result.ScenarioName != request.ScenarioName:
		return fmt.Errorf(
			"scenario_name mismatch: got %q, want %q",
			result.ScenarioName,
			request.ScenarioName,
		)
	case result.WorkloadName != workload.Name:
		return fmt.Errorf(
			"workload_name mismatch: got %q, want %q",
			result.WorkloadName,
			workload.Name,
		)
	case result.Execution != request.Execution:
		return fmt.Errorf(
			"execution mismatch: got %#v, want %#v",
			result.Execution,
			request.Execution,
		)
	case result.Sampling != request.Sampling:
		return fmt.Errorf(
			"sampling mismatch: got %#v, want %#v",
			result.Sampling,
			request.Sampling,
		)
	case result.Timing.Clock != BenchmarkClock:
		return fmt.Errorf("unexpected timing clock %q", result.Timing.Clock)
	case result.Timing.Unit != BenchmarkTimeUnit:
		return fmt.Errorf("unexpected timing unit %q", result.Timing.Unit)
	case result.Timing.ProcessStartInSamples:
		return errors.New("process_start_in_samples must be false")
	case result.Timing.Topology != ProfileTopology:
		return fmt.Errorf("unexpected profile topology %q", result.Timing.Topology)
	case result.Timing.Observer != ProfileObserver:
		return fmt.Errorf("unexpected profile observer %q", result.Timing.Observer)
	case result.Correctness.Status != "passed":
		return fmt.Errorf("correctness status must be passed, got %q", result.Correctness.Status)
	case result.Correctness.Oracle != BenchmarkOracle:
		return fmt.Errorf("unexpected correctness oracle %q", result.Correctness.Oracle)
	case result.Correctness.Comparison != BenchmarkComparison:
		return fmt.Errorf("unexpected comparison policy %q", result.Correctness.Comparison)
	case strings.TrimSpace(result.EngineBuild.Version) == "":
		return errors.New("engine build version must not be empty")
	case result.EngineBuild.Profile != "release":
		return fmt.Errorf(
			"profile engine must use the release profile, got %q",
			result.EngineBuild.Profile,
		)
	case strings.TrimSpace(result.EngineBuild.Target) == "":
		return errors.New("engine build target must not be empty")
	case strings.TrimSpace(result.EngineBuild.Rustc) == "":
		return errors.New("engine build rustc identity must not be empty")
	case strings.TrimSpace(result.EngineBuild.SourceCommit) == "":
		return errors.New("engine source commit must not be empty")
	case !validSourceState(result.EngineBuild.SourceState):
		return fmt.Errorf("invalid engine source state %q", result.EngineBuild.SourceState)
	case len(result.Samples) != int(request.Sampling.SampleIterations):
		return fmt.Errorf(
			"sample count mismatch: got %d, want %d",
			len(result.Samples),
			request.Sampling.SampleIterations,
		)
	case !rawPresent(result.Result):
		return errors.New("profile result is missing the canonical workload result")
	}

	var retainedTotal uint64
	for index, sample := range result.Samples {
		if sample.Ordinal != uint32(index) {
			return fmt.Errorf("sample %d has ordinal %d", index, sample.Ordinal)
		}
		expectedStageSum, err := sumUint64(
			sample.GenerationNS.Uint64(),
			sample.NormalizeNS.Uint64(),
			sample.ScoreNS.Uint64(),
			sample.FilterNS.Uint64(),
			sample.AggregateNS.Uint64(),
		)
		if err != nil {
			return fmt.Errorf("sample %d stage timings: %w", index, err)
		}
		if sample.StageSumNS.Uint64() != expectedStageSum {
			return fmt.Errorf(
				"sample %d stage_sum_ns is %d; exact stage sum is %d",
				index,
				sample.StageSumNS.Uint64(),
				expectedStageSum,
			)
		}
		if sample.ProfileTotalNS.Uint64() < expectedStageSum {
			return fmt.Errorf(
				"sample %d profile_total_ns is smaller than stage_sum_ns",
				index,
			)
		}
		var overflow bool
		retainedTotal, overflow = addUint64(
			retainedTotal,
			sample.ProfileTotalNS.Uint64(),
		)
		if overflow {
			return errors.New("retained profile timing total overflows u64")
		}
	}
	if result.Timing.ExperimentTotalNS.Uint64() < retainedTotal {
		return errors.New(
			"experiment_total_ns is smaller than the sum of retained profile_total_ns values",
		)
	}

	if _, err := protocol.DecodeResult(result.Result, workload); err != nil {
		return fmt.Errorf("validate canonical result: %w", err)
	}
	return nil
}
