package smoothing

import (
	"context"
	"testing"
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
)

func TestPassthroughStrategy_CalculateTargets_BasicTest(t *testing.T) {
	strategy := NewPassthroughStrategy()

	ctx := context.Background()
	now := time.Now().Truncate(time.Minute)
	start := now
	end := now.Add(5 * time.Minute)

	// Create count by minute
	countByMinute := make(map[string]int)
	for i := 0; i < 5; i++ {
		key := domain.MinuteKey(now.Add(time.Duration(i) * time.Minute))
		countByMinute[key] = 20 // 20 per minute, total 100
	}

	input := SmoothingInput{
		TotalCount:      100,
		CountByMinute:   countByMinute,
		CurrentByMinute: make(map[string]int),
		CapPerMinute:    0,
	}

	allocations, err := strategy.CalculateTargets(ctx, start, end, input)

	if err != nil {
		t.Errorf("CalculateTargets() error = %v, want nil", err)
	}
	if allocations == nil {
		t.Fatal("CalculateTargets() = nil, want allocations")
	}
	if len(allocations) != 5 {
		t.Errorf("CalculateTargets() len = %d, want 5", len(allocations))
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

func TestPassthroughStrategy_CalculateTargets_PreservesDistribution(t *testing.T) {
	strategy := NewPassthroughStrategy()

	ctx := context.Background()
	start := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	end := start.Add(5 * time.Minute)

	// Create uneven distribution
	countByMinute := map[string]int{
		"2025-01-01-10-00": 30, // Higher count at first minute
		"2025-01-01-10-01": 10,
		"2025-01-01-10-02": 10,
		"2025-01-01-10-03": 10,
		"2025-01-01-10-04": 0, // No reminds at last minute
	}

	input := SmoothingInput{
		TotalCount:      60,
		CountByMinute:   countByMinute,
		CurrentByMinute: map[string]int{},
		CapPerMinute:    100,
	}

	allocations, err := strategy.CalculateTargets(ctx, start, end, input)

	if err != nil {
		t.Errorf("CalculateTargets() error = %v, want nil", err)
	}
	if allocations == nil {
		t.Fatal("CalculateTargets() = nil, want allocations")
	}
	if len(allocations) != 5 {
		t.Errorf("CalculateTargets() len = %d, want 5", len(allocations))
	}

	// Verify total equals TotalCount (60)
	totalTarget := 0
	for _, alloc := range allocations {
		totalTarget += alloc.Target
	}
	if totalTarget != 60 {
		t.Errorf("Total target = %d, want 60", totalTarget)
	}

	// Verify first minute has higher target due to higher count
	if allocations[0].Target <= allocations[1].Target {
		t.Errorf("First minute target (%d) should be higher than second (%d) due to higher count in distribution",
			allocations[0].Target, allocations[1].Target)
	}
}

func TestPassthroughStrategy_CalculateTargets_WithCapPerMinute(t *testing.T) {
	strategy := NewPassthroughStrategy()

	ctx := context.Background()
	start := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	end := start.Add(3 * time.Minute)

	countByMinute := map[string]int{
		"2025-01-01-10-00": 50,
		"2025-01-01-10-01": 10,
		"2025-01-01-10-02": 10,
	}

	input := SmoothingInput{
		TotalCount:      70,
		CountByMinute:   countByMinute,
		CurrentByMinute: map[string]int{"2025-01-01-10-00": 40}, // Already 40 at first minute
		CapPerMinute:    60,                                     // Cap at 60
	}

	allocations, err := strategy.CalculateTargets(ctx, start, end, input)

	if err != nil {
		t.Errorf("CalculateTargets() error = %v, want nil", err)
	}
	if allocations == nil {
		t.Fatal("CalculateTargets() = nil, want allocations")
	}

	// First minute should have Available capped at 20 (60 - 40 = 20)
	if allocations[0].Available > 20 {
		t.Errorf("First minute Available = %d, want <= 20 (capped by capacity)", allocations[0].Available)
	}
}

func TestPassthroughStrategy_ImplementsStrategy(t *testing.T) {
	// This test verifies the interface implementation at compile time
	var _ Strategy = (*PassthroughStrategy)(nil)
	var _ Strategy = NewPassthroughStrategy()
}
