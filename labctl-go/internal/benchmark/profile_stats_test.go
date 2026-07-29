package benchmark

import (
	"encoding/json"
	"math"
	"testing"
)

func TestSummarizeProfileRetainsEveryDeclaredBoundary(t *testing.T) {
	t.Parallel()

	_, _, _, _, profile := validPairedProfileFixture()
	summary, err := SummarizeProfile(profile.Samples)
	if err != nil {
		t.Fatalf("SummarizeProfile() error = %v", err)
	}

	if summary.Generation.MedianNS != 105 ||
		summary.Normalize.MedianNS != 55 ||
		summary.Score.MedianNS != 45 ||
		summary.Filter.MedianNS != 35 ||
		summary.Aggregate.MedianNS != 25 ||
		summary.StageSum.MedianNS != 265 ||
		summary.PipelineStages.MedianNS != 160 ||
		summary.ProfileTotal.MedianNS != 285 {
		t.Fatalf("unexpected profile summary: %#v", summary)
	}
}

func TestAnalyzeScenarioDerivesExactIntegerEvidence(t *testing.T) {
	t.Parallel()

	_, _, projection, baseline, profile := validPairedProfileFixture()
	baselineSummary, err := Summarize(baseline.Samples)
	if err != nil {
		t.Fatalf("Summarize() error = %v", err)
	}
	profileSummary, err := SummarizeProfile(profile.Samples)
	if err != nil {
		t.Fatalf("SummarizeProfile() error = %v", err)
	}

	analysis, err := AnalyzeScenario(
		projection,
		baseline,
		baselineSummary,
		profile,
		profileSummary,
	)
	if err != nil {
		t.Fatalf("AnalyzeScenario() error = %v", err)
	}

	if analysis.AcceptedCount != 3 || analysis.SelectivityBPS != 3_750 {
		t.Fatalf("unexpected selectivity analysis: %#v", analysis)
	}
	if analysis.StageMedianSumNS != 265 ||
		analysis.StagePipelineMedianSumNS != 160 {
		t.Fatalf("unexpected stage sums: %#v", analysis)
	}
	if analysis.StageShareBPS != (StageSharesBPS{
		Generation: 3_962,
		Normalize:  2_076,
		Score:      1_698,
		Filter:     1_321,
		Aggregate:  943,
	}) {
		t.Fatalf("unexpected stage shares: %#v", analysis.StageShareBPS)
	}
	if analysis.DominantStage != "generation" ||
		analysis.DominantPipelineStage != "normalize" {
		t.Fatalf("unexpected dominant stages: %#v", analysis)
	}
	if analysis.FusedPipelineMedianNSPerRecordMilli != 26_250 ||
		analysis.StagePassPipelineMedianNSPerRecordMilli != 20_000 ||
		analysis.StagePassToFusedPipelineRatioMilli != 761 {
		t.Fatalf("unexpected scaled costs: %#v", analysis)
	}
	if analysis.FusedPipelineMedianAbsoluteDeviationBPS != 476 ||
		analysis.ProfileTotalMedianAbsoluteDeviationBPS != 877 {
		t.Fatalf("unexpected variability: %#v", analysis)
	}
}

func TestAnalyzeScenarioRejectsDifferentLogicalResults(t *testing.T) {
	t.Parallel()

	_, _, projection, baseline, profile := validPairedProfileFixture()
	profile.Result = json.RawMessage(
		`{"schema_version":"paraflow.result/v1",` +
			`"accepted_count":"0x0000000000000004",` +
			`"score_sum":{"encoding":"ieee754-binary64","bits":"0x401a000000000000"},` +
			`"category_histogram":["0x0000000000000004"],` +
			`"accepted_id_sum":"0x0000000000000010",` +
			`"accepted_id_xor":"0x6ebb399a18884447"}`,
	)
	baselineSummary, _ := Summarize(baseline.Samples)
	profileSummary, _ := SummarizeProfile(profile.Samples)

	if _, err := AnalyzeScenario(
		projection,
		baseline,
		baselineSummary,
		profile,
		profileSummary,
	); err == nil {
		t.Fatal("different fused/profile results must be rejected")
	}
}

func TestApportionBasisPointsUsesStableLargestRemainders(t *testing.T) {
	t.Parallel()

	shares, err := apportionBasisPoints([]uint64{1, 1, 1})
	if err != nil {
		t.Fatalf("apportionBasisPoints() error = %v", err)
	}
	if len(shares) != 3 || shares[0] != 3_334 ||
		shares[1] != 3_333 || shares[2] != 3_333 {
		t.Fatalf("shares = %v", shares)
	}
}

func TestProfileStatisticsRejectEmptyAndOverflowingInputs(t *testing.T) {
	t.Parallel()

	if _, err := SummarizeProfile(nil); err == nil {
		t.Fatal("SummarizeProfile(nil) must fail")
	}
	if _, err := SummarizeProfile([]ProfileSample{
		{
			NormalizeNS: math.MaxUint64,
			ScoreNS:     1,
		},
	}); err == nil {
		t.Fatal("overflowing pipeline stage sum must fail")
	}
	if _, err := scaledRatio(math.MaxUint64, 1, 1_000); err == nil {
		t.Fatal("ratio outside u64 must fail")
	}
}
