package slot

import (
	"context"
	"log/slog"
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
)

type Calculator struct {
	slotCounter         Counter
	requestCapPerMinute int
}

func NewCalculator(slotCounter Counter, requestCapPerMinute int) *Calculator {
	return &Calculator{
		slotCounter:         slotCounter,
		requestCapPerMinute: requestCapPerMinute,
	}
}

// FindSlot finds an available slot for a notification within its slide window.
// This is used as a fallback when SlideDiscovery is not available.
func (c *Calculator) FindSlot(
	ctx context.Context,
	originalTime time.Time,
	slideWindowSeconds int,
) (scheduledTime time.Time, wasShifted bool) {
	// If no cap configured, use original time
	if c.requestCapPerMinute <= 0 {
		return originalTime, false
	}

	originalMinuteKey := domain.MinuteKey(originalTime)

	// Check current slot capacity
	currentCount, err := c.slotCounter.GetTotalCountForMinute(ctx, originalMinuteKey)
	if err != nil {
		slog.WarnContext(ctx, "failed to get slot count, using original time",
			slog.String("minute_key", originalMinuteKey),
			slog.String("error", err.Error()),
		)
		return originalTime, false
	}

	// If under cap, use original time
	if currentCount < c.requestCapPerMinute {
		return originalTime, false
	}

	// Cap exceeded
	// If slide window is less than 1 minute, cannot shift
	if slideWindowSeconds < 60 {
		return originalTime, false
	}

	slideWindowDuration := time.Duration(slideWindowSeconds) * time.Second

	// Search for available slot minute by minute within slide window
	for offset := time.Minute; offset <= slideWindowDuration; offset += time.Minute {
		candidateTime := originalTime.Add(offset)
		candidateMinuteKey := domain.MinuteKey(candidateTime)

		count, err := c.slotCounter.GetTotalCountForMinute(ctx, candidateMinuteKey)
		if err != nil {
			slog.WarnContext(ctx, "failed to get slot count for candidate minute",
				slog.String("minute_key", candidateMinuteKey),
				slog.String("error", err.Error()),
			)
			continue
		}

		if count < c.requestCapPerMinute {
			slog.DebugContext(ctx, "shifted due to cap",
				slog.String("original_minute", originalMinuteKey),
				slog.String("shifted_minute", candidateMinuteKey),
				slog.Duration("shift", offset),
			)
			return candidateTime, true
		}
	}

	// No available slot found within slide window, use original time
	slog.WarnContext(ctx, "no available slot within slide window, using original time",
		slog.Int("slide_window_seconds", slideWindowSeconds),
	)
	return originalTime, false
}
