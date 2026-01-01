package smoothing

import (
	"context"
	"fmt"
	"log/slog"
	"time"
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

// CalculateTargets computes smoothed per-minute target allocations.
// 1. Build raw distribution from CountByMinute
// 2. Apply triangular kernel convolution with Greville edge handling
// 3. Normalize to preserve total count
// 4. Build allocations with capacity constraints
func (t *TriangularKernelStrategy) CalculateTargets(
	ctx context.Context,
	start, end time.Time,
	input SmoothingInput,
) ([]TargetAllocation, error) {
	window := NewTimeWindow(start, end)
	if window == nil {
		return nil, nil
	}

	// Build raw distribution from CountByMinute
	raw := make([]float64, window.NumMinutes)
	for i, key := range window.MinuteKeys {
		if count, ok := input.CountByMinute[key]; ok {
			raw[i] = float64(count)
		}
	}

	// Apply triangular kernel convolution with Greville edge handling
	smoothed := convolveWithGreville(raw, t.radius)

	// Normalize to preserve total count
	normalized := normalizeToSum(smoothed, input.TotalCount)

	allocations := buildTargetAllocations(window, normalized, input)

	if !validateNoLoss(allocations, input.TotalCount) {
		slog.WarnContext(ctx, "smoothing allocation validation failed: packet loss detected",
			slog.Int("expected", input.TotalCount),
		)
	}

	slog.DebugContext(ctx, "triangular kernel smoothing completed",
		slog.Int("num_minutes", window.NumMinutes),
		slog.Int("total_count", input.TotalCount),
		slog.Int("kernel_radius", t.radius),
	)

	return allocations, nil
}

func (t *TriangularKernelStrategy) Radius() int {
	return t.radius
}
