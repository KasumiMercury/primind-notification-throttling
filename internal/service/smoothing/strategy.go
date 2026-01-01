package smoothing

import (
	"context"
	"time"
)

// TargetAllocation represents the smoothed target for a minute.
// Used by both smoothing and sliding packages.
type TargetAllocation struct {
	MinuteKey    string
	MinuteTime   time.Time
	Target       int // Smoothed target count for this minute
	CurrentCount int // Already committed/planned count
	Available    int // Target - CurrentCount (available capacity toward target)
}

// SmoothingInput contains all information needed to calculate targets.
// This is used before lane classification to calculate per-minute targets.
type SmoothingInput struct {
	TotalCount      int            // Total notifications in window (all lanes)
	CountByMinute   map[string]int // Raw count per minute before smoothing
	CurrentByMinute map[string]int // Already committed + planned counts
	CapPerMinute    int            // Maximum capacity per minute
}

// UpdateTargetAllocation updates the target allocation after a notification is assigned.
func UpdateTargetAllocation(allocations []TargetAllocation, minuteKey string) {
	for i := range allocations {
		if allocations[i].MinuteKey == minuteKey {
			allocations[i].CurrentCount++
			if allocations[i].Available > 0 {
				allocations[i].Available--
			}
			return
		}
	}
}

// Strategy defines the interface for smoothing algorithms.
type Strategy interface {
	// CalculateTargets computes smoothed per-minute target allocations.
	// This should be called BEFORE lane classification to calculate targets for the entire window.
	CalculateTargets(ctx context.Context, start, end time.Time, input SmoothingInput) ([]TargetAllocation, error)
}

// MinuteAllocation is kept for backward compatibility.
// Deprecated: Use TargetAllocation instead.
type MinuteAllocation = TargetAllocation

// AllocationInput is kept for backward compatibility.
// Deprecated: Use SmoothingInput instead.
type AllocationInput struct {
	StrictCount     int
	LooseCount      int
	StrictByMinute  map[string]int // Pre-allocated strict packets per minute (minuteKey -> count)
	CurrentByMinute map[string]int // Existing committed + planned counts (minuteKey -> count)
	CapPerMinute    int
}

// UpdateAllocation is kept for backward compatibility.
// Deprecated: Use UpdateTargetAllocation instead.
func UpdateAllocation(allocations []MinuteAllocation, minuteKey string) {
	UpdateTargetAllocation(allocations, minuteKey)
}
