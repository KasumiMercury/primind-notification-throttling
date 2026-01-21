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

// RadiusBatch represents a group of notifications with the same minute and slide radius.
// Used by the optimization strategy to perform per-notification-radius smoothing.
type RadiusBatch struct {
	MinuteKey string // Minute key (e.g., "2025-01-01-10-05")
	Minute    int    // Relative minute index within the window (0-based)
	Count     int    // Number of notifications
	Radius    int    // Slide radius in minutes (derived from SlideWindowWidth/60)
}

// SmoothingInput contains all information needed to calculate targets.
// This is used before lane classification to calculate per-minute targets.
type SmoothingInput struct {
	TotalCount      int            // Total notifications in window (all lanes)
	CountByMinute   map[string]int // Raw count per minute before smoothing
	CurrentByMinute map[string]int // Already committed + planned counts
	CapPerMinute    int            // Maximum capacity per minute
	RadiusBatches   []RadiusBatch  // Optional: per-notification radius info for optimization strategy
	StartValue      *int           // Optional: target from previous window's first minute for continuity
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
