package benchmark

import (
	"errors"
	"fmt"
	"math"
	"math/bits"
	"sort"

	"github.com/kvetinski/paraflow/labctl-go/internal/protocol"
)

const maxPortableJSONInteger = uint64(1<<53 - 1)

// SummarizeProfile derives descriptive statistics for every raw profile
// boundary. Raw samples remain authoritative in ProfileEngineResult.
func SummarizeProfile(samples []ProfileSample) (ProfileSummary, error) {
	if len(samples) == 0 {
		return ProfileSummary{}, errors.New("cannot summarize an empty profile sample set")
	}

	generation := make([]uint64, len(samples))
	normalize := make([]uint64, len(samples))
	score := make([]uint64, len(samples))
	filter := make([]uint64, len(samples))
	aggregate := make([]uint64, len(samples))
	stageSum := make([]uint64, len(samples))
	pipelineStages := make([]uint64, len(samples))
	profileTotal := make([]uint64, len(samples))

	for index, sample := range samples {
		generation[index] = sample.GenerationNS.Uint64()
		normalize[index] = sample.NormalizeNS.Uint64()
		score[index] = sample.ScoreNS.Uint64()
		filter[index] = sample.FilterNS.Uint64()
		aggregate[index] = sample.AggregateNS.Uint64()
		stageSum[index] = sample.StageSumNS.Uint64()
		profileTotal[index] = sample.ProfileTotalNS.Uint64()

		pipeline, err := sumUint64(
			normalize[index],
			score[index],
			filter[index],
			aggregate[index],
		)
		if err != nil {
			return ProfileSummary{}, fmt.Errorf("sample %d pipeline stage sum: %w", index, err)
		}
		pipelineStages[index] = pipeline
	}

	return ProfileSummary{
		Generation:     summarizeValues(generation),
		Normalize:      summarizeValues(normalize),
		Score:          summarizeValues(score),
		Filter:         summarizeValues(filter),
		Aggregate:      summarizeValues(aggregate),
		StageSum:       summarizeValues(stageSum),
		PipelineStages: summarizeValues(pipelineStages),
		ProfileTotal:   summarizeValues(profileTotal),
	}, nil
}

// AnalyzeScenario derives integer-only profile interpretation from one fused
// baseline and one stage-pass profile over the same workload.
func AnalyzeScenario(
	workload protocol.WorkloadProjection,
	baseline EngineResult,
	baselineSummary Summary,
	profile ProfileEngineResult,
	profileSummary ProfileSummary,
) (ScenarioProfileAnalysis, error) {
	if workload.RecordCount == 0 {
		return ScenarioProfileAnalysis{}, errors.New(
			"baseline analysis requires a non-empty workload",
		)
	}

	baselineResult, err := protocol.DecodeResult(baseline.Result, workload)
	if err != nil {
		return ScenarioProfileAnalysis{}, fmt.Errorf("decode fused baseline result: %w", err)
	}
	profileResult, err := protocol.DecodeResult(profile.Result, workload)
	if err != nil {
		return ScenarioProfileAnalysis{}, fmt.Errorf("decode stage-profile result: %w", err)
	}
	if !decodedResultsEqual(baselineResult, profileResult) {
		return ScenarioProfileAnalysis{}, errors.New(
			"fused baseline and stage profile produced different logical results",
		)
	}

	stageMedians := []uint64{
		profileSummary.Generation.MedianNS.Uint64(),
		profileSummary.Normalize.MedianNS.Uint64(),
		profileSummary.Score.MedianNS.Uint64(),
		profileSummary.Filter.MedianNS.Uint64(),
		profileSummary.Aggregate.MedianNS.Uint64(),
	}
	stageMedianSum, err := sumUint64(stageMedians...)
	if err != nil {
		return ScenarioProfileAnalysis{}, fmt.Errorf("sum stage medians: %w", err)
	}
	stagePipelineMedianSum, err := sumUint64(stageMedians[1:]...)
	if err != nil {
		return ScenarioProfileAnalysis{}, fmt.Errorf("sum pipeline-stage medians: %w", err)
	}
	shares, err := apportionBasisPoints(stageMedians)
	if err != nil {
		return ScenarioProfileAnalysis{}, fmt.Errorf("derive stage shares: %w", err)
	}

	fusedMedian := baselineSummary.Pipeline.MedianNS.Uint64()
	if fusedMedian == 0 {
		return ScenarioProfileAnalysis{}, errors.New(
			"fused pipeline median must be positive for ratio analysis",
		)
	}
	fusedPerRecord, err := scaledRatio(fusedMedian, workload.RecordCount, 1_000)
	if err != nil {
		return ScenarioProfileAnalysis{}, fmt.Errorf("fused per-record cost: %w", err)
	}
	stagePassPerRecord, err := scaledRatio(
		stagePipelineMedianSum,
		workload.RecordCount,
		1_000,
	)
	if err != nil {
		return ScenarioProfileAnalysis{}, fmt.Errorf("stage-pass per-record cost: %w", err)
	}
	observerRatio, err := scaledRatio(stagePipelineMedianSum, fusedMedian, 1_000)
	if err != nil {
		return ScenarioProfileAnalysis{}, fmt.Errorf("observer ratio: %w", err)
	}
	fusedMAD, err := ratioBasisPoints(
		baselineSummary.Pipeline.MedianAbsoluteDeviationNS.Uint64(),
		fusedMedian,
	)
	if err != nil {
		return ScenarioProfileAnalysis{}, fmt.Errorf("fused variability: %w", err)
	}
	profileTotalMedian := profileSummary.ProfileTotal.MedianNS.Uint64()
	if profileTotalMedian == 0 {
		return ScenarioProfileAnalysis{}, errors.New(
			"profile total median must be positive for variability analysis",
		)
	}
	profileMAD, err := ratioBasisPoints(
		profileSummary.ProfileTotal.MedianAbsoluteDeviationNS.Uint64(),
		profileTotalMedian,
	)
	if err != nil {
		return ScenarioProfileAnalysis{}, fmt.Errorf("profile variability: %w", err)
	}

	selectivity, err := scaledRatio(
		baselineResult.AcceptedCount,
		workload.RecordCount,
		10_000,
	)
	if err != nil {
		return ScenarioProfileAnalysis{}, fmt.Errorf("selectivity: %w", err)
	}
	if selectivity > math.MaxUint32 {
		return ScenarioProfileAnalysis{}, errors.New("selectivity exceeds the uint32 domain")
	}

	stageNames := []string{"generation", "normalize", "score", "filter", "aggregate"}
	return ScenarioProfileAnalysis{
		AcceptedCount:            baselineResult.AcceptedCount,
		SelectivityBPS:           uint32(selectivity),
		StageMedianSumNS:         Nanoseconds(stageMedianSum),
		StagePipelineMedianSumNS: Nanoseconds(stagePipelineMedianSum),
		StageShareBPS: StageSharesBPS{
			Generation: shares[0],
			Normalize:  shares[1],
			Score:      shares[2],
			Filter:     shares[3],
			Aggregate:  shares[4],
		},
		DominantStage:                           dominant(stageNames, stageMedians),
		DominantPipelineStage:                   dominant(stageNames[1:], stageMedians[1:]),
		FusedPipelineMedianNSPerRecordMilli:     fusedPerRecord,
		StagePassPipelineMedianNSPerRecordMilli: stagePassPerRecord,
		StagePassToFusedPipelineRatioMilli:      observerRatio,
		FusedPipelineMedianAbsoluteDeviationBPS: fusedMAD,
		ProfileTotalMedianAbsoluteDeviationBPS:  profileMAD,
	}, nil
}

func decodedResultsEqual(left, right protocol.ResultV1) bool {
	if left.AcceptedCount != right.AcceptedCount ||
		math.Float64bits(left.ScoreSum) != math.Float64bits(right.ScoreSum) ||
		left.AcceptedIDSum != right.AcceptedIDSum ||
		left.AcceptedIDXOR != right.AcceptedIDXOR ||
		len(left.CategoryHistogram) != len(right.CategoryHistogram) {
		return false
	}
	for index := range left.CategoryHistogram {
		if left.CategoryHistogram[index] != right.CategoryHistogram[index] {
			return false
		}
	}
	return true
}

func sumUint64(values ...uint64) (uint64, error) {
	var total uint64
	for _, value := range values {
		var overflow bool
		total, overflow = addUint64(total, value)
		if overflow {
			return 0, errors.New("u64 addition overflow")
		}
	}
	return total, nil
}

func scaledRatio(numerator, denominator, scale uint64) (uint64, error) {
	if denominator == 0 {
		return 0, errors.New("ratio denominator must be positive")
	}
	high, low := bits.Mul64(numerator, scale)
	if high >= denominator {
		return 0, errors.New("scaled ratio exceeds the u64 domain")
	}
	quotient, _ := bits.Div64(high, low, denominator)
	if quotient > maxPortableJSONInteger {
		return 0, errors.New("scaled ratio exceeds the portable JSON integer domain")
	}
	return quotient, nil
}

func ratioBasisPoints(numerator, denominator uint64) (uint32, error) {
	ratio, err := scaledRatio(numerator, denominator, 10_000)
	if err != nil {
		return 0, err
	}
	if ratio > math.MaxUint32 {
		return 0, errors.New("basis-point ratio exceeds the uint32 domain")
	}
	return uint32(ratio), nil
}

func apportionBasisPoints(values []uint64) ([]uint32, error) {
	total, err := sumUint64(values...)
	if err != nil {
		return nil, err
	}
	if total == 0 {
		return nil, errors.New("at least one stage median must be positive")
	}

	type remainder struct {
		index int
		value uint64
	}
	shares := make([]uint32, len(values))
	remainders := make([]remainder, len(values))
	var assigned uint32
	for index, value := range values {
		high, low := bits.Mul64(value, 10_000)
		quotient, residual := bits.Div64(high, low, total)
		if quotient > 10_000 {
			return nil, errors.New("stage share exceeds 10,000 basis points")
		}
		shares[index] = uint32(quotient)
		assigned += shares[index]
		remainders[index] = remainder{index: index, value: residual}
	}

	sort.SliceStable(remainders, func(left, right int) bool {
		return remainders[left].value > remainders[right].value
	})
	remaining := int(uint32(10_000) - assigned)
	if remaining > len(remainders) {
		return nil, errors.New("basis-point apportionment remainder is invalid")
	}
	for index := 0; index < remaining; index++ {
		shares[remainders[index].index]++
	}
	return shares, nil
}

func dominant(names []string, values []uint64) string {
	index := 0
	for candidate := 1; candidate < len(values); candidate++ {
		if values[candidate] > values[index] {
			index = candidate
		}
	}
	return names[index]
}
