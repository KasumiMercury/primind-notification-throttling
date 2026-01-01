package smoothing

import (
	"math"
	"sort"
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
)

func triangularWeights(radius int) []float64 {
	if radius < 0 {
		radius = 0
	}
	size := 2*radius + 1
	weights := make([]float64, size)
	for i := 0; i < size; i++ {
		// Distance from center
		distFromCenter := i - radius
		if distFromCenter < 0 {
			distFromCenter = -distFromCenter
		}
		// Weight is (radius + 1)
		weights[i] = float64(radius + 1 - distFromCenter)
	}
	return weights
}

func sumFloat64(values []float64) float64 {
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum
}

func grevilleEdgeKernel(position, length, radius int) (weights []float64, startIdx int) {
	fullKernel := triangularWeights(radius)
	fullSum := sumFloat64(fullKernel)

	// Calculate available range in data
	leftBound := position - radius
	if leftBound < 0 {
		leftBound = 0
	}
	rightBound := position + radius
	if rightBound >= length {
		rightBound = length - 1
	}

	// Calculate which portion of the kernel to use
	kernelStart := radius - (position - leftBound)
	kernelEnd := radius + (rightBound - position)

	if kernelStart < 0 {
		kernelStart = 0
	}
	if kernelEnd >= len(fullKernel) {
		kernelEnd = len(fullKernel) - 1
	}

	// Extract the valid portion of kernel
	truncated := fullKernel[kernelStart : kernelEnd+1]
	truncatedSum := sumFloat64(truncated)

	// Rescale to preserve sum property (Greville's method)
	scaled := make([]float64, len(truncated))
	if truncatedSum > 0 {
		scaleFactor := fullSum / truncatedSum
		for i, w := range truncated {
			scaled[i] = w * scaleFactor
		}
	} else {
		// Use uniform weights if truncated sum is zero
		for i := range scaled {
			scaled[i] = 1.0
		}
	}

	return scaled, leftBound
}

func convolveWithGreville(values []float64, radius int) []float64 {
	length := len(values)
	if length == 0 {
		return nil
	}

	result := make([]float64, length)
	fullKernel := triangularWeights(radius)
	fullSum := sumFloat64(fullKernel)

	for i := 0; i < length; i++ {
		var weightedSum float64
		var weightSum float64

		// Determine if we're at an edge or in the interior
		if i < radius || i >= length-radius {
			// Use Greville edge-corrected kernel
			weights, startIdx := grevilleEdgeKernel(i, length, radius)
			for j, w := range weights {
				if startIdx+j < length {
					weightedSum += values[startIdx+j] * w
					weightSum += w
				}
			}
		} else {
			// Use full kernel
			for k := -radius; k <= radius; k++ {
				idx := i + k
				kernelIdx := k + radius
				weightedSum += values[idx] * fullKernel[kernelIdx]
				weightSum += fullKernel[kernelIdx]
			}
		}

		if weightSum > 0 {
			result[i] = weightedSum / weightSum * (weightSum / fullSum) * fullSum / weightSum
			result[i] = weightedSum / weightSum
		}
	}

	inputSum := sumFloat64(values)
	outputSum := sumFloat64(result)
	if outputSum > 0 && inputSum > 0 {
		scaleFactor := inputSum / outputSum
		for i := range result {
			result[i] *= scaleFactor
		}
	}

	return result
}

func normalizeToSum(values []float64, targetSum int) []int {
	if len(values) == 0 || targetSum <= 0 {
		return make([]int, len(values))
	}

	totalValue := sumFloat64(values)
	if totalValue <= 0 {
		// Distribute evenly if all values are zero
		result := make([]int, len(values))
		if len(values) > 0 {
			base := targetSum / len(values)
			remainder := targetSum % len(values)
			for i := range result {
				result[i] = base
				if i < remainder {
					result[i]++
				}
			}
		}
		return result
	}

	// Scale values to target sum
	scaled := make([]float64, len(values))
	for i, v := range values {
		scaled[i] = v * float64(targetSum) / totalValue
	}

	// Floor values and track remainders
	result := make([]int, len(values))
	type remainder struct {
		index     int
		remainder float64
	}
	remainders := make([]remainder, len(values))

	currentSum := 0
	for i, v := range scaled {
		floored := int(math.Floor(v))
		result[i] = floored
		currentSum += floored
		remainders[i] = remainder{
			index:     i,
			remainder: v - float64(floored),
		}
	}

	// Sort by remainder descending (largest remainder method)
	sort.Slice(remainders, func(i, j int) bool {
		return remainders[i].remainder > remainders[j].remainder
	})

	// Distribute remaining units to entries with largest remainders
	toDistribute := targetSum - currentSum
	for i := 0; i < toDistribute && i < len(remainders); i++ {
		result[remainders[i].index]++
	}

	return result
}

func validateNoLoss(allocations []MinuteAllocation, expectedLooseCount int) bool {
	actualTotal := 0
	for _, alloc := range allocations {
		actualTotal += alloc.Target
	}
	return actualTotal == expectedLooseCount
}

type TimeWindow struct {
	Start       time.Time
	End         time.Time
	NumMinutes  int
	MinuteKeys  []string
	MinuteTimes []time.Time
}

func NewTimeWindow(start, end time.Time) *TimeWindow {
	start = start.UTC().Truncate(time.Minute)
	end = end.UTC().Truncate(time.Minute)

	duration := end.Sub(start)
	numMinutes := int(duration.Minutes())
	if numMinutes <= 0 {
		return nil
	}

	minuteKeys := make([]string, numMinutes)
	minuteTimes := make([]time.Time, numMinutes)
	for i := 0; i < numMinutes; i++ {
		t := start.Add(time.Duration(i) * time.Minute)
		minuteKeys[i] = domain.MinuteKey(t)
		minuteTimes[i] = t
	}

	return &TimeWindow{
		Start:       start,
		End:         end,
		NumMinutes:  numMinutes,
		MinuteKeys:  minuteKeys,
		MinuteTimes: minuteTimes,
	}
}

func BuildRawDistribution(window *TimeWindow, input AllocationInput) []float64 {
	raw := make([]float64, window.NumMinutes)

	for i, key := range window.MinuteKeys {
		if count, ok := input.StrictByMinute[key]; ok {
			raw[i] = float64(count)
		}
	}

	if input.LooseCount > 0 && window.NumMinutes > 0 {
		uniformLoose := float64(input.LooseCount) / float64(window.NumMinutes)
		for i := range raw {
			raw[i] += uniformLoose
		}
	}

	return raw
}

func BuildAllocations(window *TimeWindow, normalizedTargets []int, input AllocationInput) []MinuteAllocation {
	allocations := make([]MinuteAllocation, window.NumMinutes)

	for i := 0; i < window.NumMinutes; i++ {
		key := window.MinuteKeys[i]

		// Get current count (committed + planned)
		currentCount := 0
		if count, ok := input.CurrentByMinute[key]; ok {
			currentCount = count
		}

		// Add strict allocation to current count for capacity calculation
		if strictCount, ok := input.StrictByMinute[key]; ok {
			currentCount += strictCount
		}

		target := normalizedTargets[i]
		available := target

		// Apply capacity constraint if set
		if input.CapPerMinute > 0 {
			availableCapacity := input.CapPerMinute - currentCount
			if availableCapacity < 0 {
				availableCapacity = 0
			}
			if available > availableCapacity {
				available = availableCapacity
			}
		}

		allocations[i] = MinuteAllocation{
			MinuteKey:    key,
			MinuteTime:   window.MinuteTimes[i],
			Target:       target,
			CurrentCount: currentCount,
			Available:    available,
		}
	}

	return allocations
}
