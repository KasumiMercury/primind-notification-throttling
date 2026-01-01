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

func (g *GreedyDiscovery) FindSlotForStrict(
	ctx context.Context,
	remind timemgmt.RemindResponse,
	slideCtx *SlideContext,
) SlideResult {
	return g.findSlot(ctx, remind, slideCtx)
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

	windowStart := originalTime.Truncate(time.Minute)
	windowEnd := originalTime.Add(slideWindow)

	// Find first available slot (greedy)
	for _, target := range slideCtx.Targets {
		if target.MinuteTime.Before(windowStart) || target.MinuteTime.After(windowEnd) {
			continue
		}

		// Check both target availability and hard cap
		if target.Available > 0 && target.CurrentCount < slideCtx.CapPerMinute {
			offset := originalTime.Sub(originalTime.Truncate(time.Minute))
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
