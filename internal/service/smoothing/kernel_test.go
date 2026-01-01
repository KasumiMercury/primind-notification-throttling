package smoothing

import (
	"math"
	"testing"
)

func TestTriangularWeights(t *testing.T) {
	tests := []struct {
		name     string
		radius   int
		expected []float64
	}{
		{
			name:     "radius 0 gives single weight",
			radius:   0,
			expected: []float64{1},
		},
		{
			name:     "radius 1 gives triangle [1, 2, 1]",
			radius:   1,
			expected: []float64{1, 2, 1},
		},
		{
			name:     "radius 2 gives triangle [1, 2, 3, 2, 1]",
			radius:   2,
			expected: []float64{1, 2, 3, 2, 1},
		},
		{
			name:     "radius 5 gives triangle [1, 2, 3, 4, 5, 6, 5, 4, 3, 2, 1]",
			radius:   5,
			expected: []float64{1, 2, 3, 4, 5, 6, 5, 4, 3, 2, 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := triangularWeights(tt.radius)
			if len(result) != len(tt.expected) {
				t.Errorf("triangularWeights(%d) len = %d, want %d", tt.radius, len(result), len(tt.expected))
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("triangularWeights(%d)[%d] = %f, want %f", tt.radius, i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestTriangularWeights_NegativeRadius(t *testing.T) {
	result := triangularWeights(-1)
	// Should handle gracefully by treating as radius 0
	if len(result) != 1 {
		t.Errorf("triangularWeights(-1) len = %d, want 1", len(result))
	}
}

func TestSumFloat64(t *testing.T) {
	tests := []struct {
		name     string
		values   []float64
		expected float64
	}{
		{
			name:     "empty slice",
			values:   []float64{},
			expected: 0,
		},
		{
			name:     "single value",
			values:   []float64{5.0},
			expected: 5.0,
		},
		{
			name:     "multiple values",
			values:   []float64{1, 2, 3, 4, 5},
			expected: 15.0,
		},
		{
			name:     "triangle sum for radius 5",
			values:   []float64{1, 2, 3, 4, 5, 6, 5, 4, 3, 2, 1},
			expected: 36.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sumFloat64(tt.values)
			if result != tt.expected {
				t.Errorf("sumFloat64(%v) = %f, want %f", tt.values, result, tt.expected)
			}
		})
	}
}

func TestGrevilleEdgeKernel_Interior(t *testing.T) {
	// Position in the middle should return full kernel weights
	radius := 2
	length := 10
	position := 5 // Interior position

	weights, startIdx := grevilleEdgeKernel(position, length, radius)

	expectedLen := 2*radius + 1
	if len(weights) != expectedLen {
		t.Errorf("grevilleEdgeKernel interior: len = %d, want %d", len(weights), expectedLen)
	}
	if startIdx != position-radius {
		t.Errorf("grevilleEdgeKernel interior: startIdx = %d, want %d", startIdx, position-radius)
	}
}

func TestGrevilleEdgeKernel_LeftEdge(t *testing.T) {
	// Position at the left edge
	radius := 2
	length := 10
	position := 0 // Left edge

	weights, startIdx := grevilleEdgeKernel(position, length, radius)

	// At position 0 with radius 2, we can only access positions 0, 1, 2
	// So we get 3 weights instead of 5
	if startIdx != 0 {
		t.Errorf("grevilleEdgeKernel left edge: startIdx = %d, want 0", startIdx)
	}
	if len(weights) > 2*radius+1 {
		t.Errorf("grevilleEdgeKernel left edge: len = %d, should be <= %d", len(weights), 2*radius+1)
	}

	// Weights should sum to the same as full kernel (Greville property)
	fullKernel := triangularWeights(radius)
	fullSum := sumFloat64(fullKernel)
	edgeSum := sumFloat64(weights)

	// Allow small floating point error
	if math.Abs(edgeSum-fullSum) > 0.001 {
		t.Errorf("grevilleEdgeKernel left edge: sum = %f, want ~%f (Greville sum preservation)", edgeSum, fullSum)
	}
}

func TestGrevilleEdgeKernel_RightEdge(t *testing.T) {
	// Position at the right edge
	radius := 2
	length := 10
	position := 9 // Right edge

	weights, startIdx := grevilleEdgeKernel(position, length, radius)

	// At position 9 (last) with radius 2, we can only access positions 7, 8, 9
	if startIdx > position-radius {
		t.Errorf("grevilleEdgeKernel right edge: startIdx = %d, should be <= %d", startIdx, position-radius)
	}
	if len(weights) > 2*radius+1 {
		t.Errorf("grevilleEdgeKernel right edge: len = %d, should be <= %d", len(weights), 2*radius+1)
	}

	// Weights should sum to the same as full kernel (Greville property)
	fullKernel := triangularWeights(radius)
	fullSum := sumFloat64(fullKernel)
	edgeSum := sumFloat64(weights)

	if math.Abs(edgeSum-fullSum) > 0.001 {
		t.Errorf("grevilleEdgeKernel right edge: sum = %f, want ~%f (Greville sum preservation)", edgeSum, fullSum)
	}
}

func TestConvolveWithGreville_PreservesSum(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		radius int
	}{
		{
			name:   "uniform distribution",
			values: []float64{10, 10, 10, 10, 10, 10, 10, 10, 10, 10},
			radius: 2,
		},
		{
			name:   "non-uniform distribution",
			values: []float64{5, 10, 15, 20, 25, 20, 15, 10, 5, 0},
			radius: 2,
		},
		{
			name:   "spike distribution",
			values: []float64{0, 0, 0, 100, 0, 0, 0, 0, 0, 0},
			radius: 3,
		},
		{
			name:   "increasing distribution",
			values: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			radius: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convolveWithGreville(tt.values, tt.radius)

			inputSum := sumFloat64(tt.values)
			outputSum := sumFloat64(result)

			// Sum should be preserved (critical for no-loss guarantee)
			if math.Abs(inputSum-outputSum) > 0.001 {
				t.Errorf("convolveWithGreville: input sum = %f, output sum = %f (should be equal)", inputSum, outputSum)
			}
		})
	}
}

func TestConvolveWithGreville_EmptyInput(t *testing.T) {
	result := convolveWithGreville(nil, 2)
	if result != nil {
		t.Errorf("convolveWithGreville(nil) = %v, want nil", result)
	}

	result = convolveWithGreville([]float64{}, 2)
	if result != nil {
		t.Errorf("convolveWithGreville([]) = %v, want nil", result)
	}
}

func TestNormalizeToSum(t *testing.T) {
	tests := []struct {
		name      string
		values    []float64
		targetSum int
	}{
		{
			name:      "even distribution",
			values:    []float64{10, 10, 10, 10},
			targetSum: 100,
		},
		{
			name:      "uneven distribution",
			values:    []float64{1, 2, 3, 4, 5},
			targetSum: 50,
		},
		{
			name:      "with zeros",
			values:    []float64{0, 5, 0, 5, 0},
			targetSum: 20,
		},
		{
			name:      "all zeros",
			values:    []float64{0, 0, 0, 0},
			targetSum: 12,
		},
		{
			name:      "single value",
			values:    []float64{100},
			targetSum: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeToSum(tt.values, tt.targetSum)

			// Check that sum equals target
			actualSum := 0
			for _, v := range result {
				actualSum += v
			}
			if actualSum != tt.targetSum {
				t.Errorf("normalizeToSum: sum = %d, want %d", actualSum, tt.targetSum)
			}

			// Check that no value is negative
			for i, v := range result {
				if v < 0 {
					t.Errorf("normalizeToSum: result[%d] = %d, should be >= 0", i, v)
				}
			}
		})
	}
}

func TestNormalizeToSum_ZeroTarget(t *testing.T) {
	result := normalizeToSum([]float64{1, 2, 3}, 0)
	for i, v := range result {
		if v != 0 {
			t.Errorf("normalizeToSum with zero target: result[%d] = %d, want 0", i, v)
		}
	}
}

func TestNormalizeToSum_EmptyInput(t *testing.T) {
	result := normalizeToSum([]float64{}, 10)
	if len(result) != 0 {
		t.Errorf("normalizeToSum empty: len = %d, want 0", len(result))
	}
}

func TestValidateNoLoss(t *testing.T) {
	tests := []struct {
		name         string
		allocations  []MinuteAllocation
		expectedLose int
		wantValid    bool
	}{
		{
			name: "exact match",
			allocations: []MinuteAllocation{
				{Target: 10},
				{Target: 20},
				{Target: 30},
			},
			expectedLose: 60,
			wantValid:    true,
		},
		{
			name: "sum less than expected",
			allocations: []MinuteAllocation{
				{Target: 10},
				{Target: 20},
			},
			expectedLose: 60,
			wantValid:    false,
		},
		{
			name: "sum more than expected",
			allocations: []MinuteAllocation{
				{Target: 50},
				{Target: 50},
			},
			expectedLose: 60,
			wantValid:    false,
		},
		{
			name:         "empty allocations",
			allocations:  []MinuteAllocation{},
			expectedLose: 0,
			wantValid:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := validateNoLoss(tt.allocations, tt.expectedLose)
			if valid != tt.wantValid {
				t.Errorf("validateNoLoss = %v, want %v", valid, tt.wantValid)
			}
		})
	}
}
