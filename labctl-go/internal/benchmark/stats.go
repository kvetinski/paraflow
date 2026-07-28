package benchmark

import (
	"errors"
	"sort"
)

// Summarize derives median and median absolute deviation while retaining raw
// samples separately in EngineResult. Even-sized medians use the floor of the
// arithmetic midpoint, avoiding floating-point conversion and overflow.
func Summarize(samples []Sample) (Summary, error) {
	if len(samples) == 0 {
		return Summary{}, errors.New("cannot summarize an empty sample set")
	}

	generation := make([]uint64, len(samples))
	pipeline := make([]uint64, len(samples))
	engineTotal := make([]uint64, len(samples))
	for index, sample := range samples {
		generation[index] = sample.GenerationNS.Uint64()
		pipeline[index] = sample.PipelineNS.Uint64()
		engineTotal[index] = sample.EngineTotalNS.Uint64()
	}

	return Summary{
		Generation:  summarizeValues(generation),
		Pipeline:    summarizeValues(pipeline),
		EngineTotal: summarizeValues(engineTotal),
	}, nil
}

func summarizeValues(values []uint64) Statistics {
	sorted := append([]uint64(nil), values...)
	sort.Slice(sorted, func(left, right int) bool {
		return sorted[left] < sorted[right]
	})

	median := integerMedian(sorted)
	deviations := make([]uint64, len(sorted))
	for index, value := range sorted {
		if value >= median {
			deviations[index] = value - median
		} else {
			deviations[index] = median - value
		}
	}
	sort.Slice(deviations, func(left, right int) bool {
		return deviations[left] < deviations[right]
	})

	return Statistics{
		Count:                     len(sorted),
		MinimumNS:                 Nanoseconds(sorted[0]),
		MedianNS:                  Nanoseconds(median),
		MedianAbsoluteDeviationNS: Nanoseconds(integerMedian(deviations)),
		MaximumNS:                 Nanoseconds(sorted[len(sorted)-1]),
	}
}

func integerMedian(sorted []uint64) uint64 {
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	lower := sorted[middle-1]
	upper := sorted[middle]
	return lower + (upper-lower)/2
}
