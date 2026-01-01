package smoothing

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
)

func variance(values []float64) float64 {
	n := float64(len(values))
	if n == 0 {
		return 0
	}
	mean := sumFloat64(values) / n
	sumSquaredDiff := 0.0
	for _, v := range values {
		diff := v - mean
		sumSquaredDiff += diff * diff
	}
	return sumSquaredDiff / n
}

func stdDev(values []float64) float64 {
	return math.Sqrt(variance(values))
}

func centroid(values []float64) float64 {
	sum := sumFloat64(values)
	if sum == 0 {
		return 0
	}
	weightedSum := 0.0
	for i, v := range values {
		weightedSum += float64(i) * v
	}
	return weightedSum / sum
}

func maxValue(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	max := values[0]
	for _, v := range values[1:] {
		if v > max {
			max = v
		}
	}
	return max
}

func formatArray(values []float64) string {
	if len(values) == 0 {
		return "[]"
	}
	result := "["
	for i, v := range values {
		if i > 0 {
			result += ", "
		}
		result += formatFloat(v)
	}
	result += "]"
	return result
}

func formatFloat(v float64) string {
	if v == float64(int(v)) {
		return fmt.Sprintf("%d", int(v))
	}
	return fmt.Sprintf("%.2f", v)
}

func formatIntArray(values []int) string {
	if len(values) == 0 {
		return "[]"
	}
	result := "["
	for i, v := range values {
		if i > 0 {
			result += ", "
		}
		result += fmt.Sprintf("%d", v)
	}
	result += "]"
	return result
}

func logBeforeAfter(t *testing.T, name string, before, after []float64, radius int) {
	t.Helper()
	t.Logf("=== %s (radius=%d) ===", name, radius)
	t.Logf("  Before: %s", formatArray(before))
	t.Logf("  After:  %s", formatArray(after))
	t.Logf("  Sum: %.2f -> %.2f", sumFloat64(before), sumFloat64(after))
	t.Logf("  Variance: %.2f -> %.2f (reduction: %.1f%%)",
		variance(before), variance(after),
		(1-variance(after)/variance(before))*100)
	t.Logf("  Max: %.2f -> %.2f", maxValue(before), maxValue(after))
}

func TestConvolveWithGreville_VarianceReduction(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		radius int
	}{
		{
			name:   "single spike",
			values: []float64{0, 0, 0, 100, 0, 0, 0, 0, 0, 0},
			radius: 2,
		},
		{
			name:   "double spike at edges",
			values: []float64{50, 0, 0, 0, 0, 0, 0, 0, 0, 50},
			radius: 2,
		},
		{
			name:   "high contrast alternating",
			values: []float64{100, 0, 100, 0, 100, 0, 100, 0, 100, 0},
			radius: 2,
		},
		{
			name:   "multiple spikes",
			values: []float64{0, 0, 50, 0, 0, 0, 0, 80, 0, 0},
			radius: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convolveWithGreville(tt.values, tt.radius)

			logBeforeAfter(t, tt.name, tt.values, result, tt.radius)

			inputVariance := variance(tt.values)
			outputVariance := variance(result)
			if outputVariance >= inputVariance {
				t.Errorf("Variance should decrease after smoothing: input=%.2f, output=%.2f",
					inputVariance, outputVariance)
			}
		})
	}
}

func TestConvolveWithGreville_StdDevReduction(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		radius int
	}{
		{
			name:   "center spike",
			values: []float64{0, 0, 0, 0, 100, 0, 0, 0, 0, 0},
			radius: 2,
		},
		{
			name:   "increasing then decreasing",
			values: []float64{0, 10, 30, 60, 100, 60, 30, 10, 0, 0},
			radius: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convolveWithGreville(tt.values, tt.radius)

			logBeforeAfter(t, tt.name, tt.values, result, tt.radius)

			inputStdDev := stdDev(tt.values)
			outputStdDev := stdDev(result)
			if outputStdDev >= inputStdDev {
				t.Errorf("Standard deviation should decrease after smoothing: input=%.2f, output=%.2f",
					inputStdDev, outputStdDev)
			}
		})
	}
}

func TestConvolveWithGreville_SpikeFlattenning(t *testing.T) {
	tests := []struct {
		name              string
		values            []float64
		radius            int
		minReductionRatio float64
	}{
		{
			name:              "center spike",
			values:            []float64{0, 0, 0, 0, 100, 0, 0, 0, 0, 0},
			radius:            2,
			minReductionRatio: 0.30,
		},
		{
			name:              "left edge spike",
			values:            []float64{100, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			radius:            2,
			minReductionRatio: 0.20,
		},
		{
			name:              "right edge spike",
			values:            []float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 100},
			radius:            2,
			minReductionRatio: 0.20,
		},
		{
			name:              "large radius center spike",
			values:            []float64{0, 0, 0, 0, 0, 100, 0, 0, 0, 0, 0},
			radius:            4,
			minReductionRatio: 0.50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convolveWithGreville(tt.values, tt.radius)

			logBeforeAfter(t, tt.name, tt.values, result, tt.radius)

			maxInput := maxValue(tt.values)
			maxOutput := maxValue(result)
			reductionRatio := (maxInput - maxOutput) / maxInput

			if maxOutput >= maxInput {
				t.Errorf("Peak should be reduced after smoothing: input max=%.2f, output max=%.2f",
					maxInput, maxOutput)
			}
			if reductionRatio < tt.minReductionRatio {
				t.Errorf("Peak reduction ratio = %.2f, want >= %.2f",
					reductionRatio, tt.minReductionRatio)
			}
		})
	}
}

func TestConvolveWithGreville_CentroidPreservation(t *testing.T) {
	tests := []struct {
		name      string
		values    []float64
		radius    int
		tolerance float64
	}{
		{
			name:      "asymmetric increasing",
			values:    []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			radius:    2,
			tolerance: 0.5,
		},
		{
			name:      "left heavy",
			values:    []float64{50, 30, 20, 10, 5, 3, 2, 1, 1, 1},
			radius:    2,
			tolerance: 0.5,
		},
		{
			name:      "right heavy",
			values:    []float64{1, 1, 1, 2, 3, 5, 10, 20, 30, 50},
			radius:    2,
			tolerance: 0.5,
		},
		{
			name:      "symmetric with peak",
			values:    []float64{5, 10, 15, 20, 25, 20, 15, 10, 5, 0},
			radius:    2,
			tolerance: 0.3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convolveWithGreville(tt.values, tt.radius)

			logBeforeAfter(t, tt.name, tt.values, result, tt.radius)

			inputCentroid := centroid(tt.values)
			outputCentroid := centroid(result)
			diff := math.Abs(inputCentroid - outputCentroid)
			t.Logf("  Centroid: %.2f -> %.2f (diff: %.2f)", inputCentroid, outputCentroid, diff)

			if diff > tt.tolerance {
				t.Errorf("Centroid should be preserved (symmetric kernel): input=%.2f, output=%.2f, diff=%.2f, tolerance=%.2f",
					inputCentroid, outputCentroid, diff, tt.tolerance)
			}
		})
	}
}

func TestConvolveWithGreville_UniformDistributionInvariance(t *testing.T) {
	tests := []struct {
		name      string
		size      int
		value     float64
		radius    int
		tolerance float64
	}{
		{
			name:      "small uniform",
			size:      10,
			value:     10.0,
			radius:    2,
			tolerance: 0.5,
		},
		{
			name:      "medium uniform",
			size:      20,
			value:     10.0,
			radius:    3,
			tolerance: 0.5,
		},
		{
			name:      "large uniform",
			size:      50,
			value:     10.0,
			radius:    5,
			tolerance: 0.5,
		},
		{
			name:      "high value uniform",
			size:      10,
			value:     100.0,
			radius:    2,
			tolerance: 5.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := make([]float64, tt.size)
			for i := range values {
				values[i] = tt.value
			}

			result := convolveWithGreville(values, tt.radius)

			logBeforeAfter(t, tt.name, values, result, tt.radius)

			mean := sumFloat64(result) / float64(len(result))

			for i, v := range result {
				if math.Abs(v-mean) > tt.tolerance {
					t.Errorf("output[%d] = %.2f, want ~%.2f (uniform invariance, tolerance=%.2f)",
						i, v, mean, tt.tolerance)
				}
			}
		})
	}
}

func TestConvolveWithGreville_RadiusSensitivity(t *testing.T) {
	// single spike
	input := []float64{0, 0, 0, 0, 0, 100, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}

	t.Logf("Input: %s", formatArray(input))

	radii := []int{1, 2, 3, 5, 7}
	prevVariance := variance(input)
	prevMax := maxValue(input)

	t.Logf("Input - Variance: %.2f, Max: %.2f", prevVariance, prevMax)

	for _, radius := range radii {
		output := convolveWithGreville(input, radius)
		currentVariance := variance(output)
		currentMax := maxValue(output)

		t.Logf("Radius %d: %s", radius, formatArray(output))
		t.Logf("  Variance: %.2f, Max: %.2f", currentVariance, currentMax)

		if currentVariance > prevVariance*1.05 { // 5% tolerance for numerical errors
			t.Errorf("radius=%d: variance=%.2f increased from previous=%.2f",
				radius, currentVariance, prevVariance)
		}

		if currentMax > prevMax*1.05 {
			t.Errorf("radius=%d: max=%.2f should not exceed previous max=%.2f",
				radius, currentMax, prevMax)
		}

		prevVariance = currentVariance
		prevMax = currentMax
	}
}

func TestConvolveWithGreville_RadiusZeroIsPassthrough(t *testing.T) {
	input := []float64{1, 5, 10, 50, 100, 50, 10, 5, 1}
	output := convolveWithGreville(input, 0)

	t.Logf("Input:  %s", formatArray(input))
	t.Logf("Output: %s", formatArray(output))

	if len(output) != len(input) {
		t.Fatalf("output length = %d, want %d", len(output), len(input))
	}

	for i := range input {
		if math.Abs(output[i]-input[i]) > 0.001 {
			t.Errorf("output[%d] = %.4f, want %.4f (passthrough for radius=0)",
				i, output[i], input[i])
		}
	}
}

func TestConvolveWithGreville_LargerRadiusStrongerSmoothing(t *testing.T) {
	input := []float64{0, 0, 0, 0, 100, 0, 0, 0, 0, 0}

	smallRadius := 1
	largeRadius := 3

	smallResult := convolveWithGreville(input, smallRadius)
	largeResult := convolveWithGreville(input, largeRadius)

	t.Logf("Input:  %s", formatArray(input))
	t.Logf("Radius %d: %s", smallRadius, formatArray(smallResult))
	t.Logf("Radius %d: %s", largeRadius, formatArray(largeResult))

	smallVariance := variance(smallResult)
	largeVariance := variance(largeResult)

	t.Logf("Small radius (%d) variance: %.2f", smallRadius, smallVariance)
	t.Logf("Large radius (%d) variance: %.2f", largeRadius, largeVariance)

	if largeVariance >= smallVariance {
		t.Errorf("Larger radius should produce lower variance: radius=%d variance=%.2f, radius=%d variance=%.2f",
			smallRadius, smallVariance, largeRadius, largeVariance)
	}
}

func TestTriangularKernelStrategy_RushHourScenario(t *testing.T) {
	strategy := NewTriangularKernelStrategy(5)
	ctx := context.Background()

	start := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 1, 10, 30, 0, 0, time.UTC)

	// add heavy load
	countByMinute := make(map[string]int)
	totalCount := 0
	for i := 0; i < 30; i++ {
		minuteTime := start.Add(time.Duration(i) * time.Minute)
		key := domain.MinuteKey(minuteTime)
		if i >= 12 && i < 18 {
			countByMinute[key] = 40
		} else {
			countByMinute[key] = 5
		}
		totalCount += countByMinute[key]
	}

	input := SmoothingInput{
		TotalCount:      totalCount,
		CountByMinute:   countByMinute,
		CurrentByMinute: map[string]int{},
		CapPerMinute:    100,
	}

	allocations, err := strategy.CalculateTargets(ctx, start, end, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	targets := make([]int, len(allocations))
	rawValues := make([]int, len(allocations))
	for i, alloc := range allocations {
		targets[i] = alloc.Target
		rawValues[i] = countByMinute[alloc.MinuteKey]
	}

	t.Logf("Raw (before):     %s", formatIntArray(rawValues))
	t.Logf("Smoothed (after): %s", formatIntArray(targets))

	// Verify sum preservation
	actualTotal := 0
	for _, alloc := range allocations {
		actualTotal += alloc.Target
	}
	if actualTotal != totalCount {
		t.Errorf("total target = %d, want %d", actualTotal, totalCount)
	}

	rushHasPositiveTargets := false
	for i := 12; i < 18; i++ {
		if allocations[i].Target > 0 {
			rushHasPositiveTargets = true
			break
		}
	}
	if !rushHasPositiveTargets {
		t.Error("Rush period should have positive targets after smoothing, not be emptied")
	}

	// Verify variance reduction
	targetsFloat := make([]float64, len(targets))
	rawFloat := make([]float64, len(rawValues))
	for i := range targets {
		targetsFloat[i] = float64(targets[i])
		rawFloat[i] = float64(rawValues[i])
	}

	inputVariance := variance(rawFloat)
	outputVariance := variance(targetsFloat)
	t.Logf("Variance: input=%.2f, output=%.2f (reduction: %.1f%%)",
		inputVariance, outputVariance, (1-outputVariance/inputVariance)*100)

	if outputVariance >= inputVariance {
		t.Errorf("Smoothing should reduce variance: input=%.2f, output=%.2f",
			inputVariance, outputVariance)
	}
}

func TestTriangularKernelStrategy_GradualBuildupPattern(t *testing.T) {
	strategy := NewTriangularKernelStrategy(3)
	ctx := context.Background()

	start := time.Date(2025, 1, 1, 8, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 1, 8, 30, 0, 0, time.UTC)

	// Gradually increasing load
	countByMinute := make(map[string]int)
	totalCount := 0
	for i := 0; i < 30; i++ {
		minuteTime := start.Add(time.Duration(i) * time.Minute)
		key := domain.MinuteKey(minuteTime)
		countByMinute[key] = i * 2 // Increasing from 0 to 58
		totalCount += i * 2
	}

	input := SmoothingInput{
		TotalCount:      totalCount,
		CountByMinute:   countByMinute,
		CurrentByMinute: map[string]int{},
		CapPerMinute:    100,
	}

	allocations, err := strategy.CalculateTargets(ctx, start, end, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Extract targets
	targets := make([]int, len(allocations))
	for i, alloc := range allocations {
		targets[i] = alloc.Target
	}

	// Raw values
	rawValues := make([]int, len(allocations))
	for i, alloc := range allocations {
		rawValues[i] = countByMinute[alloc.MinuteKey]
	}

	t.Logf("Raw (before):     %s", formatIntArray(rawValues))
	t.Logf("Smoothed (after): %s", formatIntArray(targets))

	// Verify sum preservation
	actualTotal := 0
	for _, alloc := range allocations {
		actualTotal += alloc.Target
	}
	if actualTotal != totalCount {
		t.Errorf("total target = %d, want %d", actualTotal, totalCount)
	}

	// Verify that all minutes got some allocation
	emptyCount := 0
	for _, alloc := range allocations {
		if alloc.Target == 0 {
			emptyCount++
		}
	}
	emptyPercentage := float64(emptyCount) / float64(len(allocations)) * 100
	t.Logf("Empty allocation percentage: %.1f%%", emptyPercentage)

	if emptyPercentage > 30 {
		t.Errorf("%.1f%% of minutes have no allocation, smoothing should distribute better",
			emptyPercentage)
	}
}

func TestTriangularKernelStrategy_DoubleSpikesPattern(t *testing.T) {
	strategy := NewTriangularKernelStrategy(5)
	ctx := context.Background()

	start := time.Date(2025, 1, 1, 8, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 1, 8, 30, 0, 0, time.UTC)

	// Two spikes
	countByMinute := make(map[string]int)
	totalCount := 0
	numMinutes := 30
	basePerMinute := 3

	for i := 0; i < numMinutes; i++ {
		minuteTime := start.Add(time.Duration(i) * time.Minute)
		key := domain.MinuteKey(minuteTime)
		switch i {
		case 7, 22:
			countByMinute[key] = 80 // Spike
		default:
			countByMinute[key] = basePerMinute // Base load
		}
		totalCount += countByMinute[key]
	}

	input := SmoothingInput{
		TotalCount:      totalCount,
		CountByMinute:   countByMinute,
		CurrentByMinute: map[string]int{},
		CapPerMinute:    50,
	}

	allocations, err := strategy.CalculateTargets(ctx, start, end, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Extract targets
	targets := make([]int, len(allocations))
	for i, alloc := range allocations {
		targets[i] = alloc.Target
	}

	// Raw values
	rawValues := make([]int, len(allocations))
	for i, alloc := range allocations {
		rawValues[i] = countByMinute[alloc.MinuteKey]
	}

	t.Logf("Raw (before):     %s", formatIntArray(rawValues))
	t.Logf("Smoothed (after): %s", formatIntArray(targets))

	// Verify sum preservation
	totalTarget := 0
	maxAlloc := 0
	for _, alloc := range allocations {
		totalTarget += alloc.Target
		if alloc.Target > maxAlloc {
			maxAlloc = alloc.Target
		}
	}
	if totalTarget != totalCount {
		t.Errorf("total target = %d, want %d", totalTarget, totalCount)
	}

	uniformPerMinute := float64(totalCount) / float64(len(allocations))
	t.Logf("Max allocation: %d (uniform would be ~%.1f)", maxAlloc, uniformPerMinute)

	// Verify gap between spikes got some allocations (spikes at 7 and 22)
	gapSum := 0
	for i := 10; i < 20; i++ {
		gapSum += allocations[i].Target
	}
	t.Logf("Gap between spikes (minutes 10-19) total: %d", gapSum)

	if gapSum == 0 {
		t.Error("Gap between spikes should receive some allocations due to smoothing")
	}
}

func TestConvolveWithGreville_SmallArrays(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		radius int
	}{
		{
			name:   "array smaller than kernel",
			values: []float64{10, 20, 30},
			radius: 5,
		},
		{
			name:   "single element",
			values: []float64{100},
			radius: 3,
		},
		{
			name:   "two elements",
			values: []float64{50, 50},
			radius: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convolveWithGreville(tt.values, tt.radius)

			logBeforeAfter(t, tt.name, tt.values, result, tt.radius)

			if len(result) != len(tt.values) {
				t.Errorf("result length = %d, want %d", len(result), len(tt.values))
			}

			inputSum := sumFloat64(tt.values)
			outputSum := sumFloat64(result)
			if math.Abs(inputSum-outputSum) > 0.01 {
				t.Errorf("sum not preserved: input=%.2f, output=%.2f", inputSum, outputSum)
			}
		})
	}
}

func TestConvolveWithGreville_AllZeros(t *testing.T) {
	values := []float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	result := convolveWithGreville(values, 2)

	t.Logf("Input:  %s", formatArray(values))
	t.Logf("Output: %s", formatArray(result))

	if len(result) != len(values) {
		t.Errorf("result length = %d, want %d", len(result), len(values))
	}

	for i, v := range result {
		if v != 0 {
			t.Errorf("result[%d] = %.4f, want 0 for all-zero input", i, v)
		}
	}
}

func TestConvolveWithGreville_NearZeroValues(t *testing.T) {
	values := []float64{0.0001, 0.0002, 0.0003, 0.0002, 0.0001}
	result := convolveWithGreville(values, 1)

	t.Logf("Input:  %s", formatArray(values))
	t.Logf("Output: %s", formatArray(result))

	if len(result) != len(values) {
		t.Errorf("result length = %d, want %d", len(result), len(values))
	}

	inputSum := sumFloat64(values)
	outputSum := sumFloat64(result)
	if math.Abs(inputSum-outputSum) > 0.0001 {
		t.Errorf("sum not preserved for small values: input=%.6f, output=%.6f",
			inputSum, outputSum)
	}

	for i, v := range result {
		if v < 0 {
			t.Errorf("result[%d] = %.6f, should not be negative", i, v)
		}
	}
}

func TestConvolveWithGreville_LargeValues(t *testing.T) {
	values := []float64{1e6, 2e6, 3e6, 2e6, 1e6}
	result := convolveWithGreville(values, 1)

	t.Logf("Input:  %s", formatArray(values))
	t.Logf("Output: %s", formatArray(result))

	// Sum should be preserved
	inputSum := sumFloat64(values)
	outputSum := sumFloat64(result)
	tolerance := inputSum * 0.0001 // 0.01% tolerance
	if math.Abs(inputSum-outputSum) > tolerance {
		t.Errorf("sum not preserved for large values: input=%.2f, output=%.2f",
			inputSum, outputSum)
	}
}
