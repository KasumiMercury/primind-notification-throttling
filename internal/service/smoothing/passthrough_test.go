package smoothing

import (
	"context"
	"testing"
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
)

func TestPassthroughStrategy_CalculateAllocationsWithContext_BasicTest(t *testing.T) {
	strategy := NewPassthroughStrategy()

	ctx := context.Background()
	now := time.Now().Truncate(time.Minute)
	start := now
	end := now.Add(5 * time.Minute)

	input := AllocationInput{
		StrictCount:     0,
		LooseCount:      100,
		StrictByMinute:  make(map[string]int),
		CurrentByMinute: make(map[string]int),
		CapPerMinute:    0,
	}

	allocations, err := strategy.CalculateAllocationsWithContext(ctx, start, end, input)

	if err != nil {
		t.Errorf("CalculateAllocationsWithContext() error = %v, want nil", err)
	}
	if allocations == nil {
		t.Fatal("CalculateAllocationsWithContext() = nil, want allocations")
	}
	if len(allocations) != 5 {
		t.Errorf("CalculateAllocationsWithContext() len = %d, want 5", len(allocations))
	}

	// Verify total equals totalCount (100)
	totalTarget := 0
	for _, alloc := range allocations {
		totalTarget += alloc.Target
	}
	if totalTarget != 100 {
		t.Errorf("Total target = %d, want 100", totalTarget)
	}
}

func TestPassthroughStrategy_CalculateAllocationsWithContext_ReturnsAllocations(t *testing.T) {
	strategy := NewPassthroughStrategy()

	ctx := context.Background()
	start := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	end := start.Add(5 * time.Minute)

	input := AllocationInput{
		StrictCount:     10,
		LooseCount:      50,
		StrictByMinute:  map[string]int{"2025-01-01-10-00": 10},
		CurrentByMinute: map[string]int{},
		CapPerMinute:    60,
	}

	allocations, err := strategy.CalculateAllocationsWithContext(ctx, start, end, input)

	if err != nil {
		t.Errorf("CalculateAllocationsWithContext() error = %v, want nil", err)
	}
	if allocations == nil {
		t.Fatal("CalculateAllocationsWithContext() = nil, want allocations")
	}
	if len(allocations) != 5 {
		t.Errorf("CalculateAllocationsWithContext() len = %d, want 5", len(allocations))
	}

	// Verify total equals strict + loose (10 + 50 = 60)
	totalTarget := 0
	for _, alloc := range allocations {
		totalTarget += alloc.Target
	}
	expectedTotal := input.StrictCount + input.LooseCount
	if totalTarget != expectedTotal {
		t.Errorf("Total target = %d, want %d", totalTarget, expectedTotal)
	}

	// Verify first minute has higher target due to strict allocation
	if allocations[0].Target <= allocations[1].Target {
		t.Errorf("First minute target (%d) should be higher than second (%d) due to strict allocation",
			allocations[0].Target, allocations[1].Target)
	}
}

func TestPassthroughStrategy_FindBestSlot_ReturnsOriginalTime(t *testing.T) {
	strategy := NewPassthroughStrategy()

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

func TestPassthroughStrategy_FindBestSlot_WithNilAllocations(t *testing.T) {
	strategy := NewPassthroughStrategy()

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

func TestPassthroughStrategy_FindBestSlot_IgnoresSlideWindow(t *testing.T) {
	strategy := NewPassthroughStrategy()

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

func TestPassthroughStrategy_ImplementsStrategy(t *testing.T) {
	// This test verifies the interface implementation at compile time
	var _ Strategy = (*PassthroughStrategy)(nil)
	var _ Strategy = NewPassthroughStrategy()
}
