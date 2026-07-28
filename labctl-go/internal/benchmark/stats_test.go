package benchmark

import (
	"math"
	"testing"
)

func TestSummarizeRetainsRobustIntegerStatistics(t *testing.T) {
	t.Parallel()

	summary, err := Summarize([]Sample{
		{GenerationNS: 10, PipelineNS: 100, EngineTotalNS: 120},
		{GenerationNS: 14, PipelineNS: 104, EngineTotalNS: 125},
		{GenerationNS: 12, PipelineNS: 102, EngineTotalNS: 123},
		{GenerationNS: 1_000, PipelineNS: 106, EngineTotalNS: 1_130},
	})
	if err != nil {
		t.Fatalf("Summarize() error = %v", err)
	}

	got := summary.Generation
	if got.Count != 4 || got.MinimumNS != 10 || got.MedianNS != 13 ||
		got.MedianAbsoluteDeviationNS != 2 || got.MaximumNS != 1_000 {
		t.Fatalf("unexpected generation statistics: %#v", got)
	}
}

func TestIntegerMedianAvoidsOverflow(t *testing.T) {
	t.Parallel()

	got := integerMedian([]uint64{math.MaxUint64 - 1, math.MaxUint64})
	if got != math.MaxUint64-1 {
		t.Fatalf("integerMedian() = %d, want %d", got, uint64(math.MaxUint64-1))
	}
}

func TestSummarizeRejectsEmptySamples(t *testing.T) {
	t.Parallel()

	if _, err := Summarize(nil); err == nil {
		t.Fatal("Summarize(nil) must fail")
	}
}
