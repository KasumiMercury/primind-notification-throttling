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

type AllocationInput struct {
	StrictCount     int
	LooseCount      int
	StrictByMinute  map[string]int // Pre-allocated strict packets per minute (minuteKey -> count)
	CurrentByMinute map[string]int // Existing committed + planned counts (minuteKey -> count)
	CapPerMinute    int
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
	// CalculateAllocationsWithContext calculates allocations with full context including strict lane information
	CalculateAllocationsWithContext(ctx context.Context, start, end time.Time, input AllocationInput) ([]MinuteAllocation, error)

	// FindBestSlot finds the best available slot for a packet within its slide window
	FindBestSlot(originalTime time.Time, slideWindowSeconds int, allocations []MinuteAllocation) (time.Time, bool)
}
