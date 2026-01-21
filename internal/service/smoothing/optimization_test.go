package smoothing

import (
	"context"
	"testing"
	"time"
)

func TestOptimizationStrategy_CalculateTargets_EmptyInput(t *testing.T) {
	strategy := NewOptimizationStrategy(nil)
	start := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	end := time.Date(2024, 1, 1, 10, 30, 0, 0, time.UTC)

	input := SmoothingInput{
		TotalCount:      0,
		CountByMinute:   map[string]int{},
		CurrentByMinute: map[string]int{},
		CapPerMinute:    100,
	}

	targets, err := strategy.CalculateTargets(context.Background(), start, end, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(targets) != 30 {
		t.Errorf("expected 30 targets, got %d", len(targets))
	}

	for _, target := range targets {
		if target.Target != 0 {
			t.Errorf("expected target 0 for minute %s, got %d", target.MinuteKey, target.Target)
		}
	}
}

func TestOptimizationStrategy_CalculateTargets_WithRadiusBatches(t *testing.T) {
	strategy := NewOptimizationStrategy(nil)
	start := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	end := time.Date(2024, 1, 1, 10, 30, 0, 0, time.UTC)

	// Create radius batches
	batches := []RadiusBatch{
		{MinuteKey: "2024-01-01-10-05", Minute: 5, Count: 50, Radius: 2},
		{MinuteKey: "2024-01-01-10-10", Minute: 10, Count: 30, Radius: 3},
		{MinuteKey: "2024-01-01-10-00", Minute: 0, Count: 10, Radius: 0}, // Fixed
	}

	input := SmoothingInput{
		TotalCount: 90,
		CountByMinute: map[string]int{
			"2024-01-01-10-00": 10,
			"2024-01-01-10-05": 50,
			"2024-01-01-10-10": 30,
		},
		CurrentByMinute: map[string]int{},
		CapPerMinute:    100,
		RadiusBatches:   batches,
	}

	targets, err := strategy.CalculateTargets(context.Background(), start, end, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(targets) != 30 {
		t.Errorf("expected 30 targets, got %d", len(targets))
	}

	// Verify total is preserved
	totalTarget := 0
	for _, target := range targets {
		totalTarget += target.Target
	}
	if totalTarget != 90 {
		t.Errorf("expected total target 90, got %d", totalTarget)
	}
}

func TestOptimizationStrategy_CalculateTargets_FixedRadius0(t *testing.T) {
	strategy := NewOptimizationStrategy(nil)
	start := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	end := time.Date(2024, 1, 1, 10, 10, 0, 0, time.UTC)

	// All notifications have radius 0 (cannot move)
	batches := []RadiusBatch{
		{MinuteKey: "2024-01-01-10-02", Minute: 2, Count: 5, Radius: 0},
		{MinuteKey: "2024-01-01-10-05", Minute: 5, Count: 10, Radius: 0},
	}

	input := SmoothingInput{
		TotalCount: 15,
		CountByMinute: map[string]int{
			"2024-01-01-10-02": 5,
			"2024-01-01-10-05": 10,
		},
		CurrentByMinute: map[string]int{},
		CapPerMinute:    100,
		RadiusBatches:   batches,
	}

	targets, err := strategy.CalculateTargets(context.Background(), start, end, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With radius 0, notifications should not move from their original minute
	foundMinute2 := false
	foundMinute5 := false
	for _, target := range targets {
		if target.MinuteKey == "2024-01-01-10-02" {
			foundMinute2 = true
			if target.Target != 5 {
				t.Errorf("expected minute 2 target to be 5, got %d", target.Target)
			}
		}
		if target.MinuteKey == "2024-01-01-10-05" {
			foundMinute5 = true
			if target.Target != 10 {
				t.Errorf("expected minute 5 target to be 10, got %d", target.Target)
			}
		}
	}
	if !foundMinute2 || !foundMinute5 {
		t.Errorf("missing expected minute targets")
	}
}

func TestOptimizationStrategy_CalculateTargets_FallbackToDefault(t *testing.T) {
	strategy := NewOptimizationStrategy(nil)
	start := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	end := time.Date(2024, 1, 1, 10, 30, 0, 0, time.UTC)

	// No RadiusBatches, should fall back to default radius behavior
	input := SmoothingInput{
		TotalCount: 100,
		CountByMinute: map[string]int{
			"2024-01-01-10-05": 50,
			"2024-01-01-10-10": 30,
			"2024-01-01-10-15": 20,
		},
		CurrentByMinute: map[string]int{},
		CapPerMinute:    100,
		RadiusBatches:   nil, // No radius batches
	}

	targets, err := strategy.CalculateTargets(context.Background(), start, end, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(targets) != 30 {
		t.Errorf("expected 30 targets, got %d", len(targets))
	}

	// Verify total is preserved
	totalTarget := 0
	for _, target := range targets {
		totalTarget += target.Target
	}
	if totalTarget != 100 {
		t.Errorf("expected total target 100, got %d", totalTarget)
	}
}

func TestBuildMatrixInput(t *testing.T) {
	strategy := NewOptimizationStrategy(nil)

	batches := []RadiusBatch{
		{MinuteKey: "2024-01-01-10-05", Minute: 5, Count: 50, Radius: 2},
		{MinuteKey: "2024-01-01-10-10", Minute: 10, Count: 30, Radius: 3},
		{MinuteKey: "2024-01-01-10-00", Minute: 0, Count: 10, Radius: 0},
	}

	matrixInput, err := strategy.buildMatrixInput(batches, 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if matrixInput.N != 30 {
		t.Errorf("expected N=30, got %d", matrixInput.N)
	}

	// Should have 3 unique radii: 0, 2, 3
	if matrixInput.K != 3 {
		t.Errorf("expected K=3 (unique radii), got %d", matrixInput.K)
	}

	// Radii should be sorted
	expectedRadii := []int64{0, 2, 3}
	for i, r := range expectedRadii {
		if matrixInput.Radii[i] != r {
			t.Errorf("expected radii[%d]=%d, got %d", i, r, matrixInput.Radii[i])
		}
	}
}

func TestBuildMatrixInput_InvalidRadius(t *testing.T) {
	strategy := NewOptimizationStrategy(nil)

	batches := []RadiusBatch{
		{MinuteKey: "2024-01-01-10-05", Minute: 5, Count: 50, Radius: -1}, // Invalid
	}

	_, err := strategy.buildMatrixInput(batches, 30)
	if err == nil {
		t.Error("expected error for negative radius")
	}
}

func TestBuildMatrixInput_InvalidMinute(t *testing.T) {
	strategy := NewOptimizationStrategy(nil)

	batches := []RadiusBatch{
		{MinuteKey: "2024-01-01-10-05", Minute: 50, Count: 50, Radius: 2}, // Out of range
	}

	_, err := strategy.buildMatrixInput(batches, 30)
	if err == nil {
		t.Error("expected error for minute out of range")
	}
}

func TestSmoothTargetContinuous(t *testing.T) {
	x := []float64{0, 0, 0, 0, 50, 0, 0, 0, 0, 30, 0, 0, 0, 0, 0}
	startValue := 0
	lambda := 10.0
	maxIter := 4000

	s := SmoothTargetContinuous(x, startValue, lambda, maxIter)

	// Check sum is preserved
	totalX := 0.0
	totalS := 0.0
	for i := range x {
		totalX += x[i]
		totalS += s[i]
	}

	if abs(totalS-totalX) > 1e-6 {
		t.Errorf("sum not preserved: expected %f, got %f", totalX, totalS)
	}

	// Check start value is preserved
	if abs(s[0]-float64(startValue)) > 1e-6 {
		t.Errorf("start value not preserved: expected %f, got %f", float64(startValue), s[0])
	}

	// Check smoothness (s should be smoother than x)
	// Second derivative should be smaller for s
}

func TestSmoothTargetContinuous_SingleElement(t *testing.T) {
	x := []float64{10}
	s := SmoothTargetContinuous(x, 10, 10.0, 4000)

	if len(s) != 1 {
		t.Errorf("expected 1 element, got %d", len(s))
	}
	if s[0] != 10 {
		t.Errorf("expected s[0]=10, got %f", s[0])
	}
}

func TestMinCostFlow_SimpleFlow(t *testing.T) {
	// Simple graph: src -> a -> sink
	mcf := NewMinCostFlow(3)
	mcf.AddEdge(0, 1, 10, 1) // src -> a, cap=10, cost=1
	mcf.AddEdge(1, 2, 10, 2) // a -> sink, cap=10, cost=2

	flow, cost := mcf.Flow(0, 2, 5)

	if flow != 5 {
		t.Errorf("expected flow=5, got %d", flow)
	}
	if cost != 15 { // 5 * (1 + 2) = 15
		t.Errorf("expected cost=15, got %d", cost)
	}
}

func TestMinCostFlow_NoPath(t *testing.T) {
	// No path from src to sink
	mcf := NewMinCostFlow(3)
	mcf.AddEdge(0, 1, 10, 1) // src -> a (no edge to sink)

	flow, _ := mcf.Flow(0, 2, 5)

	if flow != 0 {
		t.Errorf("expected flow=0, got %d", flow)
	}
}

func TestMinCostFlow_MultiPath(t *testing.T) {
	// Two paths with different costs
	mcf := NewMinCostFlow(4)
	mcf.AddEdge(0, 1, 5, 1) // src -> a, cost=1
	mcf.AddEdge(0, 2, 5, 2) // src -> b, cost=2
	mcf.AddEdge(1, 3, 5, 1) // a -> sink, cost=1
	mcf.AddEdge(2, 3, 5, 1) // b -> sink, cost=1

	flow, cost := mcf.Flow(0, 3, 7)

	if flow != 7 {
		t.Errorf("expected flow=7, got %d", flow)
	}
	// Cheapest path (via a) costs 2 per unit, next cheapest (via b) costs 3 per unit
	// First 5 units go via a (cost = 5*2 = 10), next 2 via b (cost = 2*3 = 6)
	expectedCost := 10 + 6
	if cost != expectedCost {
		t.Errorf("expected cost=%d, got %d", expectedCost, cost)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
