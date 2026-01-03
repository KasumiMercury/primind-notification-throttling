package smoothing

import (
	"context"
	"time"
)

var _ Strategy = (*PassthroughStrategy)(nil)

// PassthroughStrategy implements smoothing.Strategy without any smoothing.
// It uses the raw distribution without kernel convolution.
type PassthroughStrategy struct{}

func NewPassthroughStrategy() *PassthroughStrategy {
	return &PassthroughStrategy{}
}

// CalculateTargets computes targets without smoothing.
// The raw distribution is used directly (no kernel convolution).
func (p *PassthroughStrategy) CalculateTargets(
	_ context.Context,
	start, end time.Time,
	input SmoothingInput,
) ([]TargetAllocation, error) {
	window := NewTimeWindow(start, end)
	if window == nil {
		return nil, nil
	}

	// Build raw distribution from CountByMinute (no smoothing)
	raw := make([]float64, window.NumMinutes)
	for i, key := range window.MinuteKeys {
		if count, ok := input.CountByMinute[key]; ok {
			raw[i] = float64(count)
		}
	}

	// Normalize to preserve total count
	normalized := normalizeToSum(raw, input.TotalCount)

	return buildTargetAllocations(window, normalized, input), nil
}

// buildTargetAllocations builds target allocations from normalized values.
func buildTargetAllocations(window *TimeWindow, normalizedTargets []int, input SmoothingInput) []TargetAllocation {
	allocations := make([]TargetAllocation, window.NumMinutes)

	for i := 0; i < window.NumMinutes; i++ {
		key := window.MinuteKeys[i]

		currentCount := 0
		if count, ok := input.CurrentByMinute[key]; ok {
			currentCount = count
		}

		target := normalizedTargets[i]
		available := target - currentCount

		// Apply capacity constraint if set
		if input.CapPerMinute > 0 && currentCount+available > input.CapPerMinute {
			available = input.CapPerMinute - currentCount
		}
		if available < 0 {
			available = 0
		}

		allocations[i] = TargetAllocation{
			MinuteKey:    key,
			MinuteTime:   window.MinuteTimes[i],
			Target:       target,
			CurrentCount: currentCount,
			Available:    available,
		}
	}

	return allocations
}
