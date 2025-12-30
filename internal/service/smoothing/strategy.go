package smoothing

import (
	"context"
	"time"
)

type MinuteAllocation struct {
	MinuteKey    string
	MinuteTime   time.Time
	Target       int
	CurrentCount int
	Remaining    int
}

func UpdateAllocation(allocations []MinuteAllocation, minuteKey string) {
	for i := range allocations {
		if allocations[i].MinuteKey == minuteKey {
			if allocations[i].Remaining > 0 {
				allocations[i].Remaining--
			}
			allocations[i].CurrentCount++
			return
		}
	}
}

type Strategy interface {
	CalculateAllocations(ctx context.Context, start, end time.Time, totalCount int) ([]MinuteAllocation, error)
	FindBestSlot(originalTime time.Time, slideWindowSeconds int, allocations []MinuteAllocation) (time.Time, bool)
}
