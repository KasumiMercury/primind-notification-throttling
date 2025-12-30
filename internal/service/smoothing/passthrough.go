package smoothing

import (
	"context"
	"time"
)

var _ Strategy = (*PassthroughStrategy)(nil)

type PassthroughStrategy struct{}

func NewPassthroughStrategy() *PassthroughStrategy {
	return &PassthroughStrategy{}
}

func (p *PassthroughStrategy) CalculateAllocations(
	ctx context.Context,
	start, end time.Time,
	totalCount int,
) ([]MinuteAllocation, error) {
	return nil, nil
}

func (p *PassthroughStrategy) FindBestSlot(
	originalTime time.Time,
	slideWindowSeconds int,
	allocations []MinuteAllocation,
) (time.Time, bool) {
	return originalTime, false
}
