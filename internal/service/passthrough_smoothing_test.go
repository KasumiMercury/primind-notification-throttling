package service

import (
	"context"
	"testing"
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
)

func TestPassthroughSmoothingStrategy_CalculateAllocations_ReturnsNil(t *testing.T) {
	strategy := NewPassthroughSmoothingStrategy()

	ctx := context.Background()
	now := time.Now().Truncate(time.Minute)
	start := now
	end := now.Add(5 * time.Minute)

	allocations, err := strategy.CalculateAllocations(ctx, start, end, 100)

	if err != nil {
		t.Errorf("CalculateAllocations() error = %v, want nil", err)
	}
	if allocations != nil {
		t.Errorf("CalculateAllocations() = %v, want nil", allocations)
	}
}

func TestPassthroughSmoothingStrategy_FindBestSlot_ReturnsOriginalTime(t *testing.T) {
	strategy := NewPassthroughSmoothingStrategy()

	now := time.Now().Truncate(time.Minute)
	originalTime := now.Add(10 * time.Minute)

	// Create some allocations (should be ignored by passthrough)
	allocations := []MinuteAllocation{
		{MinuteKey: domain.MinuteKey(now), MinuteTime: now, Target: 10, CurrentCount: 5, Remaining: 5},
		{MinuteKey: domain.MinuteKey(now.Add(time.Minute)), MinuteTime: now.Add(time.Minute), Target: 20, CurrentCount: 15, Remaining: 5},
	}

	plannedTime, wasShifted := strategy.FindBestSlot(originalTime, 300, allocations)

	if !plannedTime.Equal(originalTime) {
		t.Errorf("FindBestSlot() plannedTime = %v, want %v (original time)", plannedTime, originalTime)
	}
	if wasShifted {
		t.Error("FindBestSlot() wasShifted = true, want false")
	}
}

func TestPassthroughSmoothingStrategy_FindBestSlot_WithNilAllocations(t *testing.T) {
	strategy := NewPassthroughSmoothingStrategy()

	now := time.Now().Truncate(time.Minute)
	originalTime := now.Add(10 * time.Minute)

	plannedTime, wasShifted := strategy.FindBestSlot(originalTime, 300, nil)

	if !plannedTime.Equal(originalTime) {
		t.Errorf("FindBestSlot() plannedTime = %v, want %v (original time)", plannedTime, originalTime)
	}
	if wasShifted {
		t.Error("FindBestSlot() wasShifted = true, want false")
	}
}

func TestPassthroughSmoothingStrategy_FindBestSlot_IgnoresSlideWindow(t *testing.T) {
	strategy := NewPassthroughSmoothingStrategy()

	now := time.Now().Truncate(time.Minute)
	originalTime := now.Add(10 * time.Minute)

	// Even with a very large slide window, passthrough should not shift
	plannedTime, wasShifted := strategy.FindBestSlot(originalTime, 3600, nil) // 1 hour window

	if !plannedTime.Equal(originalTime) {
		t.Errorf("FindBestSlot() plannedTime = %v, want %v (original time)", plannedTime, originalTime)
	}
	if wasShifted {
		t.Error("FindBestSlot() wasShifted = true, want false")
	}
}

func TestPassthroughSmoothingStrategy_ImplementsSmoothingStrategy(t *testing.T) {
	// This test verifies the interface implementation at compile time
	var _ SmoothingStrategy = (*PassthroughSmoothingStrategy)(nil)
	var _ SmoothingStrategy = NewPassthroughSmoothingStrategy()
}
