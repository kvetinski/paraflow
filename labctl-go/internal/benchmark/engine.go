package benchmark

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"strings"
	"time"

	"github.com/kvetinski/paraflow/labctl-go/internal/jsoncheck"
	"github.com/kvetinski/paraflow/labctl-go/internal/protocol"
)

const stderrTailBytes = 64 << 10

// EngineRunner executes one scenario in a fresh Rust process. The process
// itself performs all warm-ups and retained iterations.
type EngineRunner func(
	context.Context,
	string,
	Request,
	protocol.WorkloadProjection,
) (EngineResult, Nanoseconds, error)

// RunEngine starts `paraflow-engine benchmark`, sends one request, drains both
// output streams, strictly validates the response, and reports complete Go-side
// orchestration time separately from engine samples.
func RunEngine(
	ctx context.Context,
	enginePath string,
	request Request,
	workload protocol.WorkloadProjection,
) (EngineResult, Nanoseconds, error) {
	started := time.Now()
	responseBytes, diagnostics, err := runOneShotEngine(
		ctx,
		enginePath,
		"benchmark",
		request,
	)
	if err != nil {
		return EngineResult{}, 0, err
	}

	var result EngineResult
	if err := jsoncheck.Decode(responseBytes, &result, true); err != nil {
		return EngineResult{}, 0, processError(
			"decode benchmark response",
			err,
			diagnostics,
		)
	}
	if err := validateEngineResult(result, request, workload); err != nil {
		return EngineResult{}, 0, processError(
			"validate benchmark response",
			err,
			diagnostics,
		)
	}

	orchestration := Nanoseconds(durationNanoseconds(time.Since(started)))
	if orchestration.Uint64() < result.Timing.ExperimentTotalNS.Uint64() {
		return EngineResult{}, 0, errors.New(
			"orchestration_total_ns is smaller than engine experiment_total_ns",
		)
	}
	return result, orchestration, nil
}

func runOneShotEngine(
	ctx context.Context,
	enginePath string,
	subcommand string,
	request any,
) ([]byte, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(enginePath) == "" {
		return nil, "", errors.New("engine path must not be empty")
	}
	if strings.TrimSpace(subcommand) == "" {
		return nil, "", errors.New("engine subcommand must not be empty")
	}

	payload, err := json.Marshal(request)
	if err != nil {
		return nil, "", fmt.Errorf("encode %s request: %w", subcommand, err)
	}
	if len(payload) > protocol.MaxFrameBytes {
		return nil, "", fmt.Errorf(
			"%s request is %d bytes; maximum is %d",
			subcommand,
			len(payload),
			protocol.MaxFrameBytes,
		)
	}

	stdout := newCappedBuffer(protocol.MaxFrameBytes + 3)
	stderr := newTailBuffer(stderrTailBytes)
	command := exec.CommandContext(ctx, enginePath, subcommand)
	command.Stdin = bytes.NewReader(payload)
	command.Stdout = stdout
	command.Stderr = stderr

	if err := command.Run(); err != nil {
		if contextError := ctx.Err(); contextError != nil {
			return nil, "", contextError
		}
		return nil, "", processError("run "+subcommand+" process", err, stderr.String())
	}
	if stdout.Overflowed() {
		return nil, "", fmt.Errorf(
			"%s response exceeds %d payload bytes",
			subcommand,
			protocol.MaxFrameBytes,
		)
	}

	responseBytes, err := oneShotResponsePayload(stdout.Bytes(), subcommand)
	if err != nil {
		return nil, "", processError(
			"read "+subcommand+" response",
			err,
			stderr.String(),
		)
	}
	if len(bytes.TrimSpace(responseBytes)) == 0 {
		return nil, "", processError(
			"read "+subcommand+" response",
			errors.New("engine emitted an empty response"),
			stderr.String(),
		)
	}
	return responseBytes, stderr.String(), nil
}

func benchmarkResponsePayload(raw []byte) ([]byte, error) {
	return oneShotResponsePayload(raw, "benchmark")
}

func oneShotResponsePayload(raw []byte, boundary string) ([]byte, error) {
	if len(raw) > protocol.MaxFrameBytes+2 {
		return nil, fmt.Errorf(
			"%s response exceeds %d payload bytes",
			boundary,
			protocol.MaxFrameBytes,
		)
	}
	payload := raw
	if len(payload) != 0 && payload[len(payload)-1] == '\n' {
		payload = payload[:len(payload)-1]
		if len(payload) != 0 && payload[len(payload)-1] == '\r' {
			payload = payload[:len(payload)-1]
		}
	}
	if len(payload) != 0 && (payload[len(payload)-1] == '\r' || payload[len(payload)-1] == '\n') {
		return nil, fmt.Errorf(
			"%s response may use only one optional LF or CRLF terminator",
			boundary,
		)
	}
	if len(payload) > protocol.MaxFrameBytes {
		return nil, fmt.Errorf(
			"%s response exceeds %d payload bytes",
			boundary,
			protocol.MaxFrameBytes,
		)
	}
	return payload, nil
}

func validateEngineResult(
	result EngineResult,
	request Request,
	workload protocol.WorkloadProjection,
) error {
	switch {
	case result.SchemaVersion != EngineResultSchema:
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
			"benchmark engine must use the release profile, got %q",
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
		return errors.New("benchmark result is missing the canonical workload result")
	}

	var retainedTotal uint64
	for index, sample := range result.Samples {
		if sample.Ordinal != uint32(index) {
			return fmt.Errorf(
				"sample %d has ordinal %d",
				index,
				sample.Ordinal,
			)
		}
		generation := sample.GenerationNS.Uint64()
		pipeline := sample.PipelineNS.Uint64()
		boundarySum, overflow := addUint64(generation, pipeline)
		if overflow {
			return fmt.Errorf("sample %d generation and pipeline timings overflow u64", index)
		}
		if sample.EngineTotalNS.Uint64() < boundarySum {
			return fmt.Errorf(
				"sample %d engine_total_ns is smaller than generation_ns + pipeline_ns",
				index,
			)
		}
		var retainedOverflow bool
		retainedTotal, retainedOverflow = addUint64(retainedTotal, sample.EngineTotalNS.Uint64())
		if retainedOverflow {
			return errors.New("retained engine timing total overflows u64")
		}
	}
	if result.Timing.ExperimentTotalNS.Uint64() < retainedTotal {
		return errors.New(
			"experiment_total_ns is smaller than the sum of retained engine_total_ns values",
		)
	}

	if _, err := protocol.DecodeResult(result.Result, workload); err != nil {
		return fmt.Errorf("validate canonical result: %w", err)
	}
	return nil
}

func validSourceState(value string) bool {
	return value == "clean" || value == "dirty" || value == "unknown"
}

func addUint64(left, right uint64) (uint64, bool) {
	if math.MaxUint64-left < right {
		return 0, true
	}
	return left + right, false
}

func durationNanoseconds(duration time.Duration) uint64 {
	if duration <= 0 {
		return 0
	}
	return uint64(duration.Nanoseconds())
}

func processError(operation string, cause error, diagnostics string) error {
	diagnostics = strings.TrimSpace(diagnostics)
	if diagnostics == "" {
		return fmt.Errorf("%s: %w", operation, cause)
	}
	return fmt.Errorf("%s: %w (stderr tail: %q)", operation, cause, diagnostics)
}

type cappedBuffer struct {
	data       []byte
	limit      int
	overflowed bool
}

func newCappedBuffer(limit int) *cappedBuffer {
	return &cappedBuffer{data: make([]byte, 0, limit), limit: limit}
}

func (buffer *cappedBuffer) Write(data []byte) (int, error) {
	originalLength := len(data)
	remaining := buffer.limit - len(buffer.data)
	if remaining > 0 {
		kept := min(remaining, len(data))
		buffer.data = append(buffer.data, data[:kept]...)
		data = data[kept:]
	}
	if len(data) != 0 {
		buffer.overflowed = true
	}
	return originalLength, nil
}

func (buffer *cappedBuffer) Bytes() []byte {
	return buffer.data
}

func (buffer *cappedBuffer) Overflowed() bool {
	return buffer.overflowed
}

type tailBuffer struct {
	data  []byte
	limit int
}

func newTailBuffer(limit int) *tailBuffer {
	return &tailBuffer{data: make([]byte, 0, limit), limit: limit}
}

func (buffer *tailBuffer) Write(data []byte) (int, error) {
	originalLength := len(data)
	if len(data) >= buffer.limit {
		buffer.data = append(buffer.data[:0], data[len(data)-buffer.limit:]...)
		return originalLength, nil
	}
	overflow := len(buffer.data) + len(data) - buffer.limit
	if overflow > 0 {
		copy(buffer.data, buffer.data[overflow:])
		buffer.data = buffer.data[:len(buffer.data)-overflow]
	}
	buffer.data = append(buffer.data, data...)
	return originalLength, nil
}

func (buffer *tailBuffer) String() string {
	return string(buffer.data)
}
