package benchmark

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kvetinski/paraflow/labctl-go/internal/buildinfo"
	"github.com/kvetinski/paraflow/labctl-go/internal/protocol"
)

func TestValidateEngineResultAcceptsStrictReleaseEvidence(t *testing.T) {
	t.Parallel()

	request, projection, result := validEngineFixture()
	if err := validateEngineResult(result, request, projection); err != nil {
		t.Fatalf("validateEngineResult() error = %v", err)
	}
}

func TestValidateEngineResultRejectsImpossibleOrUntrustedEvidence(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mutate func(*EngineResult)
		text   string
	}{
		{
			name: "debug profile",
			mutate: func(result *EngineResult) {
				result.EngineBuild.Profile = "debug"
			},
			text: "release profile",
		},
		{
			name: "wrong ordinal",
			mutate: func(result *EngineResult) {
				result.Samples[0].Ordinal = 7
			},
			text: "ordinal",
		},
		{
			name: "impossible boundary",
			mutate: func(result *EngineResult) {
				result.Samples[0].EngineTotalNS = 2
			},
			text: "smaller than generation_ns + pipeline_ns",
		},
		{
			name: "mismatched result",
			mutate: func(result *EngineResult) {
				result.Result = json.RawMessage(
					`{"schema_version":"paraflow.result/v1",` +
						`"accepted_count":"0x0000000000000002",` +
						`"score_sum":{"encoding":"ieee754-binary64","bits":"0x0000000000000000"},` +
						`"category_histogram":["0x0000000000000000"],` +
						`"accepted_id_sum":"0x0000000000000000",` +
						`"accepted_id_xor":"0x0000000000000000"}`,
				)
			},
			text: "accepted_count",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			request, projection, result := validEngineFixture()
			testCase.mutate(&result)
			err := validateEngineResult(result, request, projection)
			if err == nil || !strings.Contains(err.Error(), testCase.text) {
				t.Fatalf("error = %v, want text %q", err, testCase.text)
			}
		})
	}
}

func TestBenchmarkResponsePayloadExcludesOneLineTerminator(t *testing.T) {
	t.Parallel()

	for name, raw := range map[string][]byte{
		"none": []byte(`{}`),
		"lf":   []byte("{}\n"),
		"crlf": []byte("{}\r\n"),
	} {
		raw := raw
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			payload, err := benchmarkResponsePayload(raw)
			if err != nil {
				t.Fatalf("benchmarkResponsePayload() error = %v", err)
			}
			if string(payload) != `{}` {
				t.Fatalf("payload = %q", payload)
			}
		})
	}
}

func TestBenchmarkResponsePayloadRejectsUnsupportedLineEndings(t *testing.T) {
	t.Parallel()

	for name, raw := range map[string][]byte{
		"bare CR":      []byte("{}\r"),
		"double LF":    []byte("{}\n\n"),
		"CR plus CRLF": []byte("{}\r\r\n"),
	} {
		raw := raw
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := benchmarkResponsePayload(raw); err == nil {
				t.Fatal("unsupported line ending must be rejected")
			}
		})
	}
}

func TestBenchmarkResponsePayloadAcceptsExactLimitOnlyWithTerminatorExcluded(t *testing.T) {
	t.Parallel()

	payload := make([]byte, protocol.MaxFrameBytes)
	for index := range payload {
		payload[index] = ' '
	}
	payload[0] = '{'
	payload[1] = '}'

	withCRLF := append(append([]byte(nil), payload...), '\r', '\n')
	decoded, err := benchmarkResponsePayload(withCRLF)
	if err != nil {
		t.Fatalf("exact-limit payload with CRLF rejected: %v", err)
	}
	if len(decoded) != protocol.MaxFrameBytes {
		t.Fatalf("decoded payload length = %d", len(decoded))
	}

	oversized := append(append([]byte(nil), payload...), 'x')
	if _, err := benchmarkResponsePayload(oversized); err == nil {
		t.Fatal("payload beyond the exact limit must be rejected")
	}
}

func TestValidateSourceAlignmentRejectsMixedBuilds(t *testing.T) {
	t.Parallel()

	engine := EngineBuild{
		SourceCommit: "0123456789abcdef0123456789abcdef01234567",
		SourceState:  buildinfo.SourceClean,
	}
	controller := buildinfo.Info{
		FullCommit:  engine.SourceCommit,
		SourceState: buildinfo.SourceClean,
	}
	if err := validateSourceAlignment(engine, controller); err != nil {
		t.Fatalf("matching source identity rejected: %v", err)
	}

	wrongCommit := controller
	wrongCommit.FullCommit = "fedcba9876543210fedcba9876543210fedcba98"
	if err := validateSourceAlignment(engine, wrongCommit); err == nil {
		t.Fatal("mismatched source commits must be rejected")
	}

	dirty := controller
	dirty.SourceState = buildinfo.SourceDirty
	if err := validateSourceAlignment(engine, dirty); err == nil {
		t.Fatal("mismatched source states must be rejected")
	}
}

func validEngineFixture() (Request, protocol.WorkloadProjection, EngineResult) {
	projection := protocol.WorkloadProjection{
		SchemaVersion: "paraflow.workload/v1",
		Name:          "fixture",
		RecordCount:   0,
		CategoryCount: 1,
	}
	request := Request{
		SchemaVersion: RequestSchema,
		ExperimentID:  "day05:0000000000000001",
		ScenarioName:  "fixture scenario",
		Execution:     Execution{Backend: "scalar"},
		Sampling:      Sampling{WarmupIterations: 1, SampleIterations: 2},
		Workload:      json.RawMessage(`{"schema_version":"paraflow.workload/v1"}`),
	}
	resultWire := json.RawMessage(
		`{"schema_version":"paraflow.result/v1",` +
			`"accepted_count":"0x0000000000000000",` +
			`"score_sum":{"encoding":"ieee754-binary64","bits":"0x0000000000000000"},` +
			`"category_histogram":["0x0000000000000000"],` +
			`"accepted_id_sum":"0x0000000000000000",` +
			`"accepted_id_xor":"0x0000000000000000"}`,
	)
	result := EngineResult{
		SchemaVersion: EngineResultSchema,
		ExperimentID:  request.ExperimentID,
		ScenarioName:  request.ScenarioName,
		WorkloadName:  projection.Name,
		Execution:     request.Execution,
		Sampling:      request.Sampling,
		Timing: Timing{
			Clock:                 BenchmarkClock,
			Unit:                  BenchmarkTimeUnit,
			ProcessStartInSamples: false,
			ExperimentTotalNS:     100,
		},
		Correctness: Correctness{
			Status:     "passed",
			Oracle:     BenchmarkOracle,
			Comparison: BenchmarkComparison,
		},
		EngineBuild: EngineBuild{
			Version:      "0.1.0-alpha.2",
			Profile:      "release",
			Target:       "x86_64-unknown-linux-gnu",
			Rustc:        "rustc 1.88.0",
			SourceCommit: "0123456789abcdef0123456789abcdef01234567",
			SourceState:  "clean",
		},
		Samples: []Sample{
			{Ordinal: 0, GenerationNS: 10, PipelineNS: 20, EngineTotalNS: 35},
			{Ordinal: 1, GenerationNS: 11, PipelineNS: 21, EngineTotalNS: 36},
		},
		Result: resultWire,
	}
	return request, projection, result
}
