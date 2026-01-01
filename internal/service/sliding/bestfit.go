package sliding

import (
	"context"
	"log/slog"
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
	"github.com/KasumiMercury/primind-notification-throttling/internal/infra/timemgmt"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service/smoothing"
)

var _ Discovery = (*BestFitDiscovery)(nil)

// BestFitDiscovery implements Discovery using a best-fit slot-finding algorithm.
// It finds the slot with the most available capacity within the slide window.
type BestFitDiscovery struct{}

func NewBestFitDiscovery() *BestFitDiscovery {
	return &BestFitDiscovery{}
}

func (b *BestFitDiscovery) FindSlotForLoose(
	ctx context.Context,
	remind timemgmt.RemindResponse,
	slideCtx *SlideContext,
) SlideResult {
	return b.findSlot(ctx, remind, slideCtx)
}

func (b *BestFitDiscovery) FindSlotForStrict(
	ctx context.Context,
	remind timemgmt.RemindResponse,
	slideCtx *SlideContext,
) SlideResult {
	return b.findSlot(ctx, remind, slideCtx)
}

func (b *BestFitDiscovery) findSlot(
	ctx context.Context,
	remind timemgmt.RemindResponse,
	slideCtx *SlideContext,
) SlideResult {
	originalTime := remind.Time
	originalKey := domain.MinuteKey(originalTime)

	if len(slideCtx.Targets) == 0 {
		return SlideResult{PlannedTime: originalTime, WasShifted: false}
	}

	// Check if original slot has available capacity
	for _, target := range slideCtx.Targets {
		if target.MinuteKey == originalKey {
			if target.Available > 0 && target.CurrentCount < slideCtx.CapPerMinute {
				return SlideResult{PlannedTime: originalTime, WasShifted: false}
			}
			break
		}
	}

	// Search for the slot with maximum availability within slide window
	slideWindow := time.Duration(remind.SlideWindowWidth) * time.Second
	if slideWindow < time.Minute {
		// Cannot shift if slide window < 1 minute
		return SlideResult{PlannedTime: originalTime, WasShifted: false}
	}

	windowStart := originalTime.Truncate(time.Minute)
	windowEnd := originalTime.Add(slideWindow)

	var bestTarget *smoothing.TargetAllocation
	var bestTime time.Time

	// Find the slot with maximum available capacity (best-fit)
	for i := range slideCtx.Targets {
		t := &slideCtx.Targets[i]

		if t.MinuteTime.Before(windowStart) || t.MinuteTime.After(windowEnd) {
			continue
		}

		// Check both target availability and hard cap
		if t.Available <= 0 || t.CurrentCount >= slideCtx.CapPerMinute {
			continue
		}

		// Track the slot with maximum availability
		if bestTarget == nil || t.Available > bestTarget.Available {
			bestTarget = t
			bestTime = t.MinuteTime
		}
	}

	if bestTarget != nil {
		offset := originalTime.Sub(originalTime.Truncate(time.Minute))
		plannedTime := bestTime.Add(offset)

		slog.DebugContext(ctx, "bestfit: found slot with max availability",
			slog.String("remind_id", remind.ID),
			slog.String("original_key", originalKey),
			slog.String("planned_key", bestTarget.MinuteKey),
			slog.Int("available", bestTarget.Available),
		)

		return SlideResult{PlannedTime: plannedTime, WasShifted: true}
	}

	// No available slot found, return original time
	slog.DebugContext(ctx, "bestfit: no available slot found, using original time",
		slog.String("remind_id", remind.ID),
		slog.String("original_key", originalKey),
	)
	return SlideResult{PlannedTime: originalTime, WasShifted: false}
}

func (b *BestFitDiscovery) UpdateContext(slideCtx *SlideContext, minuteKey string) {
	UpdateContext(slideCtx, minuteKey)
}
