package service

import (
	"context"
	"time"
)

type SmoothingStrategy interface {
	CalculateAllocations(ctx context.Context, start, end time.Time, totalCount int) ([]MinuteAllocation, error)
	FindBestSlot(originalTime time.Time, slideWindowSeconds int, allocations []MinuteAllocation) (time.Time, bool)
}
