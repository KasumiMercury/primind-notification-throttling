package service

import (
	"context"
	"time"
)

var _ SmoothingStrategy = (*PassthroughSmoothingStrategy)(nil)

type PassthroughSmoothingStrategy struct{}

func NewPassthroughSmoothingStrategy() *PassthroughSmoothingStrategy {
	return &PassthroughSmoothingStrategy{}
}

func (p *PassthroughSmoothingStrategy) CalculateAllocations(
	ctx context.Context,
	start, end time.Time,
	totalCount int,
) ([]MinuteAllocation, error) {
	return nil, nil
}

func (p *PassthroughSmoothingStrategy) FindBestSlot(
	originalTime time.Time,
	slideWindowSeconds int,
	allocations []MinuteAllocation,
) (time.Time, bool) {
	return originalTime, false
}
