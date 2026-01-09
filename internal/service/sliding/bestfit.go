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

// FindSlotForStrict finds a slot for strict lane notifications.
// Strict notifications respect original time and only shift when at hard capacity.
// When shifting is necessary, it finds the slot with maximum headroom under cap.
func (b *BestFitDiscovery) FindSlotForStrict(
	ctx context.Context,
	remind timemgmt.RemindResponse,
	slideCtx *SlideContext,
) SlideResult {
	originalTime := remind.Time
	originalKey := domain.MinuteKey(originalTime)

	if len(slideCtx.Targets) == 0 {
		return SlideResult{PlannedTime: originalTime, WasShifted: false}
	}

	// For Strict: Check ONLY hard cap at original time
	// Use original time unless it's at capacity
	for _, target := range slideCtx.Targets {
		if target.MinuteKey == originalKey {
			if target.CurrentCount < slideCtx.CapPerMinute {
				// Under hard cap - use original time
				slog.DebugContext(ctx, "bestfit-strict: using original slot (under cap)",
					slog.String("remind_id", remind.ID),
					slog.String("original_key", originalKey),
					slog.Int("current_count", target.CurrentCount),
					slog.Int("cap", slideCtx.CapPerMinute),
				)
				return SlideResult{PlannedTime: originalTime, WasShifted: false}
			}
			break
		}
	}

	slideWindow := time.Duration(remind.SlideWindowWidth) * time.Second
	if slideWindow < time.Minute {
		return SlideResult{PlannedTime: originalTime, WasShifted: false}
	}

	windowStart := originalTime.Truncate(time.Minute)
	windowEnd := originalTime.Add(slideWindow)

	var bestTarget *smoothing.TargetAllocation
	var bestHeadroom int

	for i := range slideCtx.Targets {
		t := &slideCtx.Targets[i]
		if t.MinuteTime.Before(windowStart) || t.MinuteTime.After(windowEnd) {
			continue
		}
		headroom := slideCtx.CapPerMinute - t.CurrentCount
		if headroom <= 0 {
			continue
		}
		if bestTarget == nil || headroom > bestHeadroom {
			bestTarget = t
			bestHeadroom = headroom
		}
	}

	if bestTarget != nil {
		offset := originalTime.Sub(originalTime.Truncate(time.Minute))
		plannedTime := bestTarget.MinuteTime.Add(offset)

		slog.DebugContext(ctx, "bestfit-strict: found alternative slot",
			slog.String("remind_id", remind.ID),
			slog.String("original_key", originalKey),
			slog.String("planned_key", bestTarget.MinuteKey),
			slog.Int("headroom", bestHeadroom),
		)
		return SlideResult{PlannedTime: plannedTime, WasShifted: true}
	}

	// No slot under cap found - use original time anyway
	slog.DebugContext(ctx, "bestfit-strict: no slot under cap found, using original time",
		slog.String("remind_id", remind.ID),
		slog.String("original_key", originalKey),
	)
	return SlideResult{PlannedTime: originalTime, WasShifted: false}
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

	slideWindow := time.Duration(remind.SlideWindowWidth) * time.Second
	if slideWindow < time.Minute {
		// Cannot shift if slide window < 1 minute
		return SlideResult{PlannedTime: originalTime, WasShifted: false}
	}

	windowStart := originalTime.Truncate(time.Minute)
	windowEnd := originalTime.Add(slideWindow)

	// Find original slot's availability
	var originalAvailable int
	for _, target := range slideCtx.Targets {
		if target.MinuteKey == originalKey {
			if target.CurrentCount < slideCtx.CapPerMinute {
				originalAvailable = target.Available
			}
			break
		}
	}

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

	// Use best slot if it has MORE availability than original
	// (This enables shifting toward smoothing targets even when original has space)
	if bestTarget != nil && bestTarget.Available > originalAvailable {
		offset := originalTime.Sub(originalTime.Truncate(time.Minute))
		plannedTime := bestTime.Add(offset)

		shifted := bestTarget.MinuteKey != originalKey

		slog.DebugContext(ctx, "bestfit: selected slot",
			slog.String("remind_id", remind.ID),
			slog.String("original_key", originalKey),
			slog.String("planned_key", bestTarget.MinuteKey),
			slog.Int("original_available", originalAvailable),
			slog.Int("best_available", bestTarget.Available),
			slog.Bool("shifted", shifted),
		)

		return SlideResult{PlannedTime: plannedTime, WasShifted: shifted}
	}

	// Use original if it has capacity, or no better slot found
	if originalAvailable > 0 {
		slog.DebugContext(ctx, "bestfit: using original slot",
			slog.String("remind_id", remind.ID),
			slog.String("original_key", originalKey),
			slog.Int("original_available", originalAvailable),
		)
		return SlideResult{PlannedTime: originalTime, WasShifted: false}
	}

	// No slot available
	slog.DebugContext(ctx, "bestfit: no available slot, using original time",
		slog.String("remind_id", remind.ID),
		slog.String("original_key", originalKey),
	)
	return SlideResult{PlannedTime: originalTime, WasShifted: false}
}

func (b *BestFitDiscovery) UpdateContext(slideCtx *SlideContext, minuteKey string) {
	UpdateContext(slideCtx, minuteKey)
}

func (b *BestFitDiscovery) SupportsBatch() bool {
	return false
}

func (b *BestFitDiscovery) PrepareSlots(
	ctx context.Context,
	looseItems []*PriorityItem,
	strictItems []*PriorityItem,
	slideCtx *SlideContext,
) error {
	return nil
}
