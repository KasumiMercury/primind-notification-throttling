package sliding

import (
	"context"
	"log/slog"
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
	"github.com/KasumiMercury/primind-notification-throttling/internal/infra/timemgmt"
)

var _ Discovery = (*GreedyDiscovery)(nil)

// GreedyDiscovery implements Discovery using a greedy slot-finding algorithm.
// It finds the first available slot within the slide window.
type GreedyDiscovery struct{}

func NewGreedyDiscovery() *GreedyDiscovery {
	return &GreedyDiscovery{}
}

func (g *GreedyDiscovery) FindSlotForLoose(
	ctx context.Context,
	remind timemgmt.RemindResponse,
	slideCtx *SlideContext,
) SlideResult {
	return g.findSlot(ctx, remind, slideCtx)
}

// FindSlotForStrict finds a slot for strict lane notifications.
// Strict notifications respect original time and only shift when at hard capacity.
// They do NOT consider smoothing targets - only the hard cap matters.
func (g *GreedyDiscovery) FindSlotForStrict(
	ctx context.Context,
	remind timemgmt.RemindResponse,
	slideCtx *SlideContext,
) SlideResult {
	originalTime := remind.Time
	originalKey := domain.MinuteKey(originalTime)

	if len(slideCtx.Targets) == 0 {
		return SlideResult{PlannedTime: originalTime, WasShifted: false}
	}

	// For Strict: Check ONLY hard cap (not smoothing target)
	// Use original time unless it's at capacity
	for _, target := range slideCtx.Targets {
		if target.MinuteKey == originalKey {
			if target.CurrentCount < slideCtx.CapPerMinute {
				// Under hard cap - use original time
				return SlideResult{PlannedTime: originalTime, WasShifted: false}
			}
			break
		}
	}

	// Original slot at capacity - must find alternative
	slideWindow := time.Duration(remind.SlideWindowWidth) * time.Second
	if slideWindow < time.Minute {
		// Cannot shift if slide window < 1 minute
		return SlideResult{PlannedTime: originalTime, WasShifted: false}
	}

	windowStart := originalTime.Add(-slideWindow)
	windowEnd := originalTime.Add(slideWindow)
	offset := originalTime.Sub(originalTime.Truncate(time.Minute))

	// Find first slot under hard cap (greedy by time proximity)
	for _, target := range slideCtx.Targets {
		plannedTime := target.MinuteTime.Add(offset)
		if plannedTime.Before(windowStart) || plannedTime.After(windowEnd) {
			continue
		}
		if target.CurrentCount < slideCtx.CapPerMinute {
			plannedTime := target.MinuteTime.Add(offset)

			slog.DebugContext(ctx, "greedy-strict: found slot under cap",
				slog.String("remind_id", remind.ID),
				slog.String("original_key", originalKey),
				slog.String("planned_key", target.MinuteKey),
				slog.Int("current_count", target.CurrentCount),
				slog.Int("cap", slideCtx.CapPerMinute),
			)
			return SlideResult{PlannedTime: plannedTime, WasShifted: true}
		}
	}

	// No slot under cap found - use original time anyway
	slog.DebugContext(ctx, "greedy-strict: no slot under cap found, using original time",
		slog.String("remind_id", remind.ID),
		slog.String("original_key", originalKey),
	)
	return SlideResult{PlannedTime: originalTime, WasShifted: false}
}

func (g *GreedyDiscovery) findSlot(
	ctx context.Context,
	remind timemgmt.RemindResponse,
	slideCtx *SlideContext,
) SlideResult {
	originalTime := remind.Time
	originalKey := domain.MinuteKey(originalTime)

	if len(slideCtx.Targets) == 0 {
		return SlideResult{PlannedTime: originalTime, WasShifted: false}
	}

	// Check if original slot has available capacity (toward target and under cap)
	for _, target := range slideCtx.Targets {
		if target.MinuteKey == originalKey {
			if target.Available > 0 && target.CurrentCount < slideCtx.CapPerMinute {
				return SlideResult{PlannedTime: originalTime, WasShifted: false}
			}
			break
		}
	}

	// Search for the first available slot within slide window
	slideWindow := time.Duration(remind.SlideWindowWidth) * time.Second
	if slideWindow < time.Minute {
		// Cannot shift if slide window < 1 minute
		return SlideResult{PlannedTime: originalTime, WasShifted: false}
	}

	windowStart := originalTime.Add(-slideWindow)
	windowEnd := originalTime.Add(slideWindow)
	offset := originalTime.Sub(originalTime.Truncate(time.Minute))

	// Find first available slot (greedy)
	for _, target := range slideCtx.Targets {
		plannedTime := target.MinuteTime.Add(offset)
		if plannedTime.Before(windowStart) || plannedTime.After(windowEnd) {
			continue
		}

		// Check both target availability and hard cap
		if target.Available > 0 && target.CurrentCount < slideCtx.CapPerMinute {
			plannedTime := target.MinuteTime.Add(offset)

			slog.DebugContext(ctx, "greedy: found slot",
				slog.String("remind_id", remind.ID),
				slog.String("original_key", originalKey),
				slog.String("planned_key", target.MinuteKey),
				slog.Int("available", target.Available),
			)

			return SlideResult{PlannedTime: plannedTime, WasShifted: true}
		}
	}

	// No available slot found, return original time
	slog.DebugContext(ctx, "greedy: no available slot found, using original time",
		slog.String("remind_id", remind.ID),
		slog.String("original_key", originalKey),
	)
	return SlideResult{PlannedTime: originalTime, WasShifted: false}
}

func (g *GreedyDiscovery) UpdateContext(slideCtx *SlideContext, minuteKey string) {
	UpdateContext(slideCtx, minuteKey)
}

func (g *GreedyDiscovery) SupportsBatch() bool {
	return false
}

func (g *GreedyDiscovery) PrepareSlots(
	ctx context.Context,
	looseItems []*PriorityItem,
	strictItems []*PriorityItem,
	slideCtx *SlideContext,
) error {
	return nil
}
