package sliding

import (
	"context"
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/infra/timemgmt"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service/smoothing"
)

// SlideContext contains context for sliding decisions.
type SlideContext struct {
	Targets      []smoothing.TargetAllocation
	CapPerMinute int
}

// SlideResult represents the result of sliding a notification.
type SlideResult struct {
	PlannedTime time.Time
	WasShifted  bool
}

// Discovery defines the interface for notification sliding algorithms.
// Implementations should be easily replaceable to test different strategies.
type Discovery interface {
	// FindSlotForLoose finds the best slot for a loose lane notification.
	// Should prioritize meeting smoothed targets while respecting slide window.
	FindSlotForLoose(
		ctx context.Context,
		remind timemgmt.RemindResponse,
		slideCtx *SlideContext,
	) SlideResult

	// FindSlotForStrict finds a slot for a strict lane notification.
	// Called AFTER loose lane adjustment to handle residual capacity issues.
	FindSlotForStrict(
		ctx context.Context,
		remind timemgmt.RemindResponse,
		slideCtx *SlideContext,
	) SlideResult

	// UpdateContext updates the slide context after a notification is assigned.
	// This allows the discovery to track state between assignments.
	UpdateContext(slideCtx *SlideContext, minuteKey string)
}

// UpdateContext is a helper function to update the context after assignment.
func UpdateContext(slideCtx *SlideContext, minuteKey string) {
	smoothing.UpdateTargetAllocation(slideCtx.Targets, minuteKey)
}
