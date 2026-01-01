package smoothing

import (
	"context"
	"time"
)

var _ Strategy = (*PassthroughStrategy)(nil)

// PassthroughStrategy implements smoothing.Strategy without any smoothing.
// It distributes allocations uniformly across minutes without kernel convolution.
type PassthroughStrategy struct{}

func NewPassthroughStrategy() *PassthroughStrategy {
	return &PassthroughStrategy{}
}

func (p *PassthroughStrategy) CalculateAllocationsWithContext(
	_ context.Context,
	start, end time.Time,
	input AllocationInput,
) ([]MinuteAllocation, error) {
	window := NewTimeWindow(start, end)
	if window == nil {
		return nil, nil
	}

	// Build raw distribution (strict + uniform loose) - no smoothing
	raw := BuildRawDistribution(window, input)

	// Normalize to preserve total (strict + loose)
	totalTarget := input.StrictCount + input.LooseCount
	normalized := normalizeToSum(raw, totalTarget)

	return BuildAllocations(window, normalized, input), nil
}

func (p *PassthroughStrategy) FindBestSlot(
	originalTime time.Time,
	_ int,
	_ []MinuteAllocation,
) (time.Time, bool) {
	// Passthrough never shifts time
	return originalTime, false
}
