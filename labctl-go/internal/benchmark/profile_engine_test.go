package benchmark

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kvetinski/paraflow/labctl-go/internal/protocol"
)

func TestValidateProfileEngineResultAcceptsStrictStageEvidence(t *testing.T) {
	t.Parallel()

	_, request, projection, _, result := validPairedProfileFixture()
	if err := validateProfileEngineResult(result, request, projection); err != nil {
		t.Fatalf("validateProfileEngineResult() error = %v", err)
	}
}

func TestValidateProfileEngineResultRejectsImpossibleOrUntrustedEvidence(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mutate func(*ProfileEngineResult)
		text   string
	}{
		{
			name: "debug profile",
			mutate: func(result *ProfileEngineResult) {
				result.EngineBuild.Profile = "debug"
			},
			text: "release profile",
		},
		{
			name: "wrong topology",
			mutate: func(result *ProfileEngineResult) {
				result.Timing.Topology = "fused"
			},
			text: "topology",
		},
		{
			name: "wrong ordinal",
			mutate: func(result *ProfileEngineResult) {
				result.Samples[0].Ordinal = 4
			},
			text: "ordinal",
		},
		{
			name: "wrong stage sum",
			mutate: func(result *ProfileEngineResult) {
				result.Samples[0].StageSumNS++
			},
			text: "exact stage sum",
		},
		{
			name: "profile total below stage sum",
			mutate: func(result *ProfileEngineResult) {
				result.Samples[0].ProfileTotalNS = 1
			},
			text: "smaller than stage_sum_ns",
		},
		{
			name: "experiment total below retained samples",
			mutate: func(result *ProfileEngineResult) {
				result.Timing.ExperimentTotalNS = 1
			},
			text: "sum of retained",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, request, projection, _, result := validPairedProfileFixture()
			testCase.mutate(&result)
			err := validateProfileEngineResult(result, request, projection)
			if err == nil || !strings.Contains(err.Error(), testCase.text) {
				t.Fatalf("error = %v, want text %q", err, testCase.text)
			}
		})
	}
}

func validPairedProfileFixture() (
	Request,
	ProfileRequest,
	protocol.WorkloadProjection,
	EngineResult,
	ProfileEngineResult,
) {
	projection := protocol.WorkloadProjection{
		SchemaVersion: "paraflow.workload/v1",
		Name:          "fixture",
		RecordCount:   8,
		CategoryCount: 1,
		Distribution:  "uniform",
	}
	sampling := Sampling{WarmupIterations: 1, SampleIterations: 2}
	execution := Execution{Backend: "scalar"}
	workload := json.RawMessage(
		`{"schema_version":"paraflow.workload/v1","name":"fixture"}`,
	)
	baselineRequest := Request{
		SchemaVersion: RequestSchema,
		ExperimentID:  "day06:baseline:0000000000000001",
		ScenarioName:  "fixture scenario",
		Execution:     execution,
		Sampling:      sampling,
		Workload:      workload,
	}
	profileRequest := ProfileRequest{
		SchemaVersion: ProfileRequestSchema,
		ExperimentID:  "day06:profile:0000000000000001",
		ScenarioName:  "fixture scenario",
		Execution:     execution,
		Sampling:      sampling,
		Workload:      workload,
	}
	resultWire := json.RawMessage(
		`{"schema_version":"paraflow.result/v1",` +
			`"accepted_count":"0x0000000000000003",` +
			`"score_sum":{"encoding":"ieee754-binary64","bits":"0x401a000000000000"},` +
			`"category_histogram":["0x0000000000000003"],` +
			`"accepted_id_sum":"0x0000000000000010",` +
			`"accepted_id_xor":"0x6ebb399a18884447"}`,
	)
	build := EngineBuild{
		Version:      "0.1.0-alpha.3",
		Profile:      "release",
		Target:       "x86_64-unknown-linux-gnu",
		Rustc:        "rustc 1.97.1",
		SourceCommit: "0123456789abcdef0123456789abcdef01234567",
		SourceState:  "clean",
	}
	correctness := Correctness{
		Status:     "passed",
		Oracle:     BenchmarkOracle,
		Comparison: BenchmarkComparison,
	}
	baseline := EngineResult{
		SchemaVersion: EngineResultSchema,
		ExperimentID:  baselineRequest.ExperimentID,
		ScenarioName:  baselineRequest.ScenarioName,
		WorkloadName:  projection.Name,
		Execution:     execution,
		Sampling:      sampling,
		Timing: Timing{
			Clock:                 BenchmarkClock,
			Unit:                  BenchmarkTimeUnit,
			ProcessStartInSamples: false,
			ExperimentTotalNS:     2_000,
		},
		Correctness: correctness,
		EngineBuild: build,
		Samples: []Sample{
			{Ordinal: 0, GenerationNS: 100, PipelineNS: 200, EngineTotalNS: 320},
			{Ordinal: 1, GenerationNS: 110, PipelineNS: 220, EngineTotalNS: 350},
		},
		Result: resultWire,
	}
	profile := ProfileEngineResult{
		SchemaVersion: ProfileEngineResultSchema,
		ExperimentID:  profileRequest.ExperimentID,
		ScenarioName:  profileRequest.ScenarioName,
		WorkloadName:  projection.Name,
		Execution:     execution,
		Sampling:      sampling,
		Timing: ProfileTiming{
			Clock:                 BenchmarkClock,
			Unit:                  BenchmarkTimeUnit,
			ProcessStartInSamples: false,
			Topology:              ProfileTopology,
			Observer:              ProfileObserver,
			ExperimentTotalNS:     3_000,
		},
		Correctness: correctness,
		EngineBuild: build,
		Samples: []ProfileSample{
			{
				Ordinal:        0,
				GenerationNS:   100,
				NormalizeNS:    50,
				ScoreNS:        40,
				FilterNS:       30,
				AggregateNS:    20,
				StageSumNS:     240,
				ProfileTotalNS: 260,
			},
			{
				Ordinal:        1,
				GenerationNS:   110,
				NormalizeNS:    60,
				ScoreNS:        50,
				FilterNS:       40,
				AggregateNS:    30,
				StageSumNS:     290,
				ProfileTotalNS: 310,
			},
		},
		Result: resultWire,
	}
	return baselineRequest, profileRequest, projection, baseline, profile
}
