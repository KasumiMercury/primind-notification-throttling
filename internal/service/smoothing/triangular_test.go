package smoothing

import (
	"context"
	"testing"
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
)

func TestNewTriangularKernelStrategy(t *testing.T) {
	tests := []struct {
		name           string
		radius         int
		expectedRadius int
	}{
		{
			name:           "valid radius",
			radius:         5,
			expectedRadius: 5,
		},
		{
			name:           "zero radius uses default",
			radius:         0,
			expectedRadius: 5,
		},
		{
			name:           "negative radius uses default",
			radius:         -1,
			expectedRadius: 5,
		},
		{
			name:           "custom radius",
			radius:         10,
			expectedRadius: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strategy := NewTriangularKernelStrategy(tt.radius)
			if strategy.Radius() != tt.expectedRadius {
				t.Errorf("NewTriangularKernelStrategy(%d).Radius() = %d, want %d",
					tt.radius, strategy.Radius(), tt.expectedRadius)
			}
		})
	}
}

func TestTriangularKernelStrategy_ImplementsStrategy(t *testing.T) {
	var _ Strategy = (*TriangularKernelStrategy)(nil)
}

func TestTriangularKernelStrategy_CalculateTargets_PreservesSum(t *testing.T) {
	strategy := NewTriangularKernelStrategy(5)
	ctx := context.Background()

	start := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 1, 10, 30, 0, 0, time.UTC) // 30 minutes

	// Build count by minute
	countByMinute := make(map[string]int)
	totalCount := 0
	for i := 0; i < 30; i++ {
		minuteTime := start.Add(time.Duration(i) * time.Minute)
		key := domain.MinuteKey(minuteTime)
		countByMinute[key] = 2 // 2 per minute
		totalCount += 2
	}

	input := SmoothingInput{
		TotalCount:      totalCount,
		CountByMinute:   countByMinute,
		CurrentByMinute: map[string]int{},
		CapPerMinute:    100,
	}

	allocations, err := strategy.CalculateTargets(ctx, start, end, input)
	if err != nil {
		t.Fatalf("CalculateTargets error: %v", err)
	}

	// Sum of targets should equal TotalCount (no loss)
	actualTotal := 0
	for _, alloc := range allocations {
		actualTotal += alloc.Target
	}
	if actualTotal != totalCount {
		t.Errorf("Total target = %d, want %d (no loss)", actualTotal, totalCount)
	}
}

func TestTriangularKernelStrategy_CalculateTargets_RespectsDistribution(t *testing.T) {
	strategy := NewTriangularKernelStrategy(2)
	ctx := context.Background()

	start := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 1, 10, 10, 0, 0, time.UTC) // 10 minutes

	// Heavy load in first 5 minutes
	countByMinute := make(map[string]int)
	totalCount := 0
	for i := 0; i < 10; i++ {
		minuteTime := start.Add(time.Duration(i) * time.Minute)
		key := domain.MinuteKey(minuteTime)
		if i < 5 {
			countByMinute[key] = 50
			totalCount += 50
		} else {
			countByMinute[key] = 0
		}
	}

	input := SmoothingInput{
		TotalCount:      totalCount,
		CountByMinute:   countByMinute,
		CurrentByMinute: map[string]int{},
		CapPerMinute:    60,
	}

	allocations, err := strategy.CalculateTargets(ctx, start, end, input)
	if err != nil {
		t.Fatalf("CalculateTargets error: %v", err)
	}

	// Verify no loss - total should equal input
	actualTotal := 0
	for _, alloc := range allocations {
		actualTotal += alloc.Target
	}
	if actualTotal != totalCount {
		t.Errorf("Total target = %d, want %d", actualTotal, totalCount)
	}
}

func TestTriangularKernelStrategy_CalculateTargets_EmptyWindow(t *testing.T) {
	strategy := NewTriangularKernelStrategy(5)
	ctx := context.Background()

	start := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	end := start // Same time = 0 minutes

	input := SmoothingInput{
		TotalCount:      10,
		CountByMinute:   map[string]int{},
		CurrentByMinute: map[string]int{},
		CapPerMinute:    60,
	}

	allocations, err := strategy.CalculateTargets(ctx, start, end, input)
	if err != nil {
		t.Fatalf("CalculateTargets error: %v", err)
	}

	if allocations != nil {
		t.Errorf("Expected nil allocations for empty window, got %v", allocations)
	}
}

func TestTriangularKernelStrategy_CalculateTargets_BasicTest(t *testing.T) {
	strategy := NewTriangularKernelStrategy(5)
	ctx := context.Background()

	start := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 1, 10, 20, 0, 0, time.UTC) // 20 minutes

	// Build count by minute with uniform distribution
	countByMinute := make(map[string]int)
	for i := 0; i < 20; i++ {
		minuteTime := start.Add(time.Duration(i) * time.Minute)
		key := domain.MinuteKey(minuteTime)
		countByMinute[key] = 5 // 5 per minute, total 100
	}

	input := SmoothingInput{
		TotalCount:      100,
		CountByMinute:   countByMinute,
		CurrentByMinute: make(map[string]int),
		CapPerMinute:    0,
	}

	allocations, err := strategy.CalculateTargets(ctx, start, end, input)
	if err != nil {
		t.Fatalf("CalculateTargets error: %v", err)
	}

	// Sum of targets should equal totalCount
	totalTarget := 0
	for _, alloc := range allocations {
		totalTarget += alloc.Target
	}
	if totalTarget != 100 {
		t.Errorf("Total target = %d, want 100", totalTarget)
	}
}

func TestTriangularKernelStrategy_SmoothingEffect(t *testing.T) {
	strategy := NewTriangularKernelStrategy(2)
	ctx := context.Background()

	start := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 1, 10, 10, 0, 0, time.UTC) // 10 minutes

	// Heavy allocation in the middle (spike pattern)
	countByMinute := make(map[string]int)
	totalCount := 0
	for i := 0; i < 10; i++ {
		minuteTime := start.Add(time.Duration(i) * time.Minute)
		key := domain.MinuteKey(minuteTime)
		if i == 4 || i == 5 {
			countByMinute[key] = 30
			totalCount += 30
		} else {
			countByMinute[key] = 6
			totalCount += 6
		}
	}

	input := SmoothingInput{
		TotalCount:      totalCount,
		CountByMinute:   countByMinute,
		CurrentByMinute: map[string]int{},
		CapPerMinute:    100,
	}

	allocations, err := strategy.CalculateTargets(ctx, start, end, input)
	if err != nil {
		t.Fatalf("CalculateTargets error: %v", err)
	}

	// Find allocation for the heavy minutes
	for _, alloc := range allocations {
		// Verify allocations are reasonable
		if alloc.Target < 0 {
			t.Errorf("Allocation for %s is negative: %d", alloc.MinuteKey, alloc.Target)
		}
	}

	// Verify total
	actualTotal := 0
	for _, alloc := range allocations {
		actualTotal += alloc.Target
	}
	if actualTotal != totalCount {
		t.Errorf("Total target = %d, want %d", actualTotal, totalCount)
	}
}

func TestTriangularKernelStrategy_MinuteKeyGeneration(t *testing.T) {
	strategy := NewTriangularKernelStrategy(2)
	ctx := context.Background()

	start := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 1, 10, 5, 0, 0, time.UTC) // 5 minutes

	// Build count by minute
	countByMinute := make(map[string]int)
	for i := 0; i < 5; i++ {
		minuteTime := start.Add(time.Duration(i) * time.Minute)
		key := domain.MinuteKey(minuteTime)
		countByMinute[key] = 2
	}

	input := SmoothingInput{
		TotalCount:      10,
		CountByMinute:   countByMinute,
		CurrentByMinute: map[string]int{},
		CapPerMinute:    60,
	}

	allocations, err := strategy.CalculateTargets(ctx, start, end, input)
	if err != nil {
		t.Fatalf("CalculateTargets error: %v", err)
	}

	// Verify minute keys are generated correctly
	expectedKeys := []string{
		"2025-01-01-10-00",
		"2025-01-01-10-01",
		"2025-01-01-10-02",
		"2025-01-01-10-03",
		"2025-01-01-10-04",
	}

	if len(allocations) != len(expectedKeys) {
		t.Fatalf("len(allocations) = %d, want %d", len(allocations), len(expectedKeys))
	}

	for i, alloc := range allocations {
		if alloc.MinuteKey != expectedKeys[i] {
			t.Errorf("allocations[%d].MinuteKey = %s, want %s", i, alloc.MinuteKey, expectedKeys[i])
		}
		// Verify minute time matches key
		expectedTime, _ := domain.ParseMinuteKey(expectedKeys[i])
		if !alloc.MinuteTime.Equal(expectedTime) {
			t.Errorf("allocations[%d].MinuteTime = %v, want %v", i, alloc.MinuteTime, expectedTime)
		}
	}
}
