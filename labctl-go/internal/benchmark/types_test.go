package benchmark

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestNanosecondsUsesCanonicalFullWidthHex(t *testing.T) {
	t.Parallel()

	encoded, err := json.Marshal(Nanoseconds(math.MaxUint64))
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if got, want := string(encoded), `"0xffffffffffffffff"`; got != want {
		t.Fatalf("Marshal() = %s, want %s", got, want)
	}

	for _, invalid := range []string{
		`"0x1"`,
		`"0x000000000000000A"`,
		`1`,
	} {
		var value Nanoseconds
		if err := json.Unmarshal([]byte(invalid), &value); err == nil {
			t.Fatalf("Unmarshal(%s) must fail", invalid)
		}
	}
}

func TestSuiteValidationRejectsSemanticDriftAndUnsafePaths(t *testing.T) {
	t.Parallel()

	valid := Suite{
		SchemaVersion: SuiteSchema,
		Name:          "day05",
		Scenarios: []Scenario{{
			Name:      "uniform",
			Workload:  "workloads/uniform.json",
			Execution: Execution{Backend: "scalar"},
			Sampling:  Sampling{WarmupIterations: 1, SampleIterations: 3},
		}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid suite rejected: %v", err)
	}

	cases := []Suite{valid, valid, valid, valid}
	cases[0].Scenarios = append(cases[0].Scenarios, cases[0].Scenarios[0])
	cases[1].Scenarios[0].Workload = "../outside.json"
	cases[2].Scenarios[0].Execution.Backend = "simd"
	cases[3].Scenarios[0].Workload = strings.Repeat("a", 508) + ".json"
	for index, candidate := range cases {
		if err := candidate.Validate(); err == nil {
			t.Fatalf("invalid suite case %d was accepted", index)
		}
	}
}
