package smoothing

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
)

var ErrPacketLoss = fmt.Errorf("smoothing: packet loss detected")

var _ Strategy = (*TriangularKernelStrategy)(nil)

type TriangularKernelStrategy struct {
	radius int
}

func NewTriangularKernelStrategy(radius int) *TriangularKernelStrategy {
	if radius < 1 {
		radius = 5
	}
	return &TriangularKernelStrategy{
		radius: radius,
	}
}

// 1. Create raw distribution: strict allocations + uniform loose distribution
// 2. Apply triangular kernel convolution with Greville edge handling
// 3. Normalize to preserve total (strict + loose)
// 4. Build allocations with capacity constraints
func (t *TriangularKernelStrategy) CalculateAllocationsWithContext(
	ctx context.Context,
	start, end time.Time,
	input AllocationInput,
) ([]MinuteAllocation, error) {
	window := NewTimeWindow(start, end)
	if window == nil {
		return nil, nil
	}

	raw := BuildRawDistribution(window, input)

	smoothed := convolveWithGreville(raw, t.radius)

	totalTarget := input.StrictCount + input.LooseCount
	normalized := normalizeToSum(smoothed, totalTarget)

	allocations := BuildAllocations(window, normalized, input)

	if !validateNoLoss(allocations, totalTarget) {
		slog.WarnContext(ctx, "smoothing allocation validation failed: packet loss detected",
			slog.Int("expected", totalTarget),
		)
	}

	slog.DebugContext(ctx, "triangular kernel smoothing completed",
		slog.Int("num_minutes", window.NumMinutes),
		slog.Int("strict_count", input.StrictCount),
		slog.Int("loose_count", input.LooseCount),
		slog.Int("kernel_radius", t.radius),
	)

	return allocations, nil
}

func (t *TriangularKernelStrategy) FindBestSlot(
	originalTime time.Time,
	slideWindowSeconds int,
	allocations []MinuteAllocation,
) (time.Time, bool) {
	if len(allocations) == 0 {
		return originalTime, false
	}

	originalKey := domain.MinuteKey(originalTime)

	for i := range allocations {
		if allocations[i].MinuteKey == originalKey && allocations[i].Remaining > 0 {
			return originalTime, false
		}
	}

	slideWindow := time.Duration(slideWindowSeconds) * time.Second
	windowStart := originalTime.Truncate(time.Minute)
	windowEnd := originalTime.Add(slideWindow)

	var bestAlloc *MinuteAllocation
	var bestIdx int
	for i := range allocations {
		alloc := &allocations[i]

		if alloc.MinuteTime.Before(windowStart) || alloc.MinuteTime.After(windowEnd) {
			continue
		}

		if alloc.Remaining <= 0 {
			continue
		}

		if bestAlloc == nil || alloc.Remaining > bestAlloc.Remaining {
			bestAlloc = alloc
			bestIdx = i
		}
	}

	if bestAlloc != nil {
		offset := originalTime.Sub(originalTime.Truncate(time.Minute))
		scheduledTime := allocations[bestIdx].MinuteTime.Add(offset)
		return scheduledTime, true
	}

	// No available slot found, return original time
	slog.Debug("no available slot found within slide window, using original time",
		slog.String("original_key", originalKey),
		slog.Int("slide_window_seconds", slideWindowSeconds),
	)
	return originalTime, false
}

func (t *TriangularKernelStrategy) Radius() int {
	return t.radius
}
