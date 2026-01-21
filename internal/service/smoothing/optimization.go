package smoothing

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"time"
)

var _ Strategy = (*OptimizationStrategy)(nil)

// OptimizationConfig holds parameters for the optimization strategy.
type OptimizationConfig struct {
	LambdaTargetSmooth float64 // Smoothness strength for target s (default: 10.0)
	WeightFit          float64 // Weight for fitting y to s (default: 1.0)
	WeightMove         float64 // Weight for movement cost (default: 1.0)
	MovePower          int     // 1: |d|, 2: d^2 (default: 1)
	CostScale          int     // Float to int conversion multiplier (default: 100)
	MaxIterations      int     // Max iterations for gradient descent (default: 4000)
	DefaultRadius      int     // Default radius when RadiusBatches is nil (default: 5)
	WarnTotal          int     // Warning threshold for large inputs (default: 5000, 0 to disable)
}

// DefaultOptimizationConfig returns default optimization parameters.
func DefaultOptimizationConfig() OptimizationConfig {
	return OptimizationConfig{
		LambdaTargetSmooth: 10.0,
		WeightFit:          1.0,
		WeightMove:         1.0,
		MovePower:          1,
		CostScale:          100,
		MaxIterations:      4000,
		DefaultRadius:      5,
		WarnTotal:          5000,
	}
}

// OptimizationStrategy implements smoothing.Strategy using minimum cost flow optimization.
type OptimizationStrategy struct {
	config OptimizationConfig
}

// NewOptimizationStrategy creates a new OptimizationStrategy with the given config.
func NewOptimizationStrategy(config *OptimizationConfig) *OptimizationStrategy {
	if config == nil {
		defaultCfg := DefaultOptimizationConfig()
		config = &defaultCfg
	}
	return &OptimizationStrategy{
		config: *config,
	}
}

// CalculateTargets computes smoothed per-minute target allocations using optimization.
func (o *OptimizationStrategy) CalculateTargets(
	ctx context.Context,
	start, end time.Time,
	input SmoothingInput,
) ([]TargetAllocation, error) {
	window := NewTimeWindow(start, end)
	if window == nil {
		return nil, nil
	}

	n := window.NumMinutes
	if n == 0 {
		return nil, nil
	}

	// If RadiusBatches is nil or empty, fall back to triangular kernel behavior
	if len(input.RadiusBatches) == 0 {
		return o.calculateWithDefaultRadius(ctx, window, input)
	}

	// Build matrix input from RadiusBatches
	matrixInput, err := o.buildMatrixInput(input.RadiusBatches, n)
	if err != nil {
		slog.WarnContext(ctx, "failed to build matrix input, falling back to default",
			slog.String("error", err.Error()),
		)
		return o.calculateWithDefaultRadius(ctx, window, input)
	}

	// Run optimization
	result, err := o.runOptimization(ctx, matrixInput, input.StartValue)
	if err != nil {
		slog.WarnContext(ctx, "optimization failed, falling back to default",
			slog.String("error", err.Error()),
		)
		return o.calculateWithDefaultRadius(ctx, window, input)
	}

	// Build target allocations from optimization result
	allocations := o.buildAllocationsFromResult(window, result.Y, input)

	slog.DebugContext(ctx, "optimization strategy completed",
		slog.Int("num_minutes", n),
		slog.Int("total_count", input.TotalCount),
		slog.Int("radius_batches", len(input.RadiusBatches)),
	)

	return allocations, nil
}

// calculateWithDefaultRadius falls back to standard smoothing when RadiusBatches is not available.
func (o *OptimizationStrategy) calculateWithDefaultRadius(
	ctx context.Context,
	window *TimeWindow,
	input SmoothingInput,
) ([]TargetAllocation, error) {
	n := window.NumMinutes

	// Build batches with default radius
	batches := make([]RadiusBatch, 0)
	for i, key := range window.MinuteKeys {
		if count, ok := input.CountByMinute[key]; ok && count > 0 {
			batches = append(batches, RadiusBatch{
				MinuteKey: key,
				Minute:    i,
				Count:     count,
				Radius:    o.config.DefaultRadius,
			})
		}
	}

	if len(batches) == 0 {
		// No notifications, return empty targets
		return buildTargetAllocations(window, make([]int, n), input), nil
	}

	matrixInput, err := o.buildMatrixInput(batches, n)
	if err != nil {
		slog.WarnContext(ctx, "failed to build matrix input with default radius",
			slog.String("error", err.Error()),
		)
		// Fall back to passthrough
		return o.buildPassthroughAllocations(window, input), nil
	}

	result, err := o.runOptimization(ctx, matrixInput, input.StartValue)
	if err != nil {
		slog.WarnContext(ctx, "optimization with default radius failed",
			slog.String("error", err.Error()),
		)
		return o.buildPassthroughAllocations(window, input), nil
	}

	allocations := o.buildAllocationsFromResult(window, result.Y, input)

	slog.DebugContext(ctx, "optimization strategy with default radius completed",
		slog.Int("num_minutes", n),
		slog.Int("total_count", input.TotalCount),
		slog.Int("default_radius", o.config.DefaultRadius),
	)

	return allocations, nil
}

// buildMatrixInput converts RadiusBatches to matrix form for optimization.
func (o *OptimizationStrategy) buildMatrixInput(batches []RadiusBatch, n int) (*MatrixInput, error) {
	// Collect and sort unique radii
	radiiSet := make(map[int]struct{})
	for _, b := range batches {
		if b.Radius < 0 {
			return nil, fmt.Errorf("radius must be >= 0, got %d", b.Radius)
		}
		radiiSet[b.Radius] = struct{}{}
	}

	// Sort radii to create deterministic type indices
	radiiSorted := make([]int, 0, len(radiiSet))
	for r := range radiiSet {
		radiiSorted = append(radiiSorted, r)
	}
	sort.Ints(radiiSorted)

	// Create radius to type index mapping
	rToK := make(map[int]int)
	for i, r := range radiiSorted {
		rToK[r] = i
	}

	K := len(radiiSorted)
	counts := make([][]int64, n)
	for t := range counts {
		counts[t] = make([]int64, K)
	}

	// Populate counts matrix
	for _, b := range batches {
		if b.Minute < 0 || b.Minute >= n {
			return nil, fmt.Errorf("minute out of range: %d (expected 0..%d)", b.Minute, n-1)
		}
		if b.Count < 0 {
			return nil, fmt.Errorf("count must be non-negative, got %d", b.Count)
		}
		k := rToK[b.Radius]
		counts[b.Minute][k] += int64(b.Count)
	}

	// Convert radii to int64 slice
	radii := make([]int64, K)
	for i, r := range radiiSorted {
		radii[i] = int64(r)
	}

	return &MatrixInput{
		Counts: counts,
		Radii:  radii,
		N:      n,
		K:      K,
	}, nil
}

// optimizationResult contains the output of the optimization.
type optimizationResult struct {
	Y           []int64     // Final histogram (n,)
	F           [][][]int64 // Assignment matrix F[k][t][j]
	Diagnostics diagnostics // Intermediate information
}

// diagnostics holds intermediate computation details.
type diagnostics struct {
	Total                   int
	StartValue              int
	TotalRemainingOptimized int
	CostScaled              int
	TargetSContinuous       []float64
	Radii                   []int64
	Note                    string
}

// runOptimization performs the main minimum cost flow optimization.
func (o *OptimizationStrategy) runOptimization(
	ctx context.Context,
	input *MatrixInput,
	startValue *int,
) (*optimizationResult, error) {
	n := input.N
	K := input.K
	counts := input.Counts
	radii := input.Radii

	// Compute x[t] = sum of all notifications at time t
	x := make([]int64, n)
	for t := range n {
		for k := range K {
			x[t] += counts[t][k]
		}
	}

	// Total number of notifications
	total := int64(0)
	for t := range n {
		total += x[t]
	}

	// Handle empty case
	if total == 0 {
		y := make([]int64, n)
		F := make([][][]int64, K)
		for k := range K {
			F[k] = make([][]int64, n)
			for t := range n {
				F[k][t] = make([]int64, n)
			}
		}
		return &optimizationResult{
			Y: y,
			F: F,
			Diagnostics: diagnostics{
				Note: "all zero",
			},
		}, nil
	}

	// Determine left (y[0] constraint value)
	left := int(x[0])
	if startValue != nil {
		left = *startValue
	}
	if left < 0 {
		left = 0
	}
	if int64(left) > total {
		left = int(total)
	}

	// Phase 2: Create continuous smooth target
	xFloat := make([]float64, n)
	for i, v := range x {
		xFloat[i] = float64(v)
	}
	s := SmoothTargetContinuous(xFloat, left, o.config.LambdaTargetSmooth, o.config.MaxIterations)

	// Cost functions
	moveCost := func(t, j int) int {
		d := absInt(t - j)
		var base int
		if o.config.MovePower == 2 {
			base = d * d
		} else {
			base = d
		}
		return int(math.Round(float64(o.config.CostScale) * o.config.WeightMove * float64(base)))
	}

	// Marginal cost for placing kth item in bin j: 2k - 1 - 2*s[j]
	fitMarginalCost := func(binJ, kth int) int {
		mc := o.config.WeightFit * (2.0*float64(kth) - 1.0 - 2.0*s[binJ])
		return int(math.Round(float64(o.config.CostScale) * mc))
	}

	// Phase 3a: Pre-allocate y[0] = left
	can0 := 0
	for t := range n {
		for k := range K {
			cnt := int(counts[t][k])
			if cnt <= 0 {
				continue
			}
			if absInt(t-0) <= int(radii[k]) {
				can0 += cnt
			}
		}
	}

	if can0 < left {
		return nil, fmt.Errorf("infeasible: cannot satisfy y[0]=%d (reachable=%d)", left, can0)
	}

	// Allocate to y[0] prioritizing closer sources
	type candidate struct {
		dist int
		t    int
		k    int
	}
	candidates := make([]candidate, 0)
	for t := range n {
		for k := range K {
			cnt := int(counts[t][k])
			if cnt <= 0 {
				continue
			}
			if absInt(t) <= int(radii[k]) {
				candidates = append(candidates, candidate{dist: absInt(t), t: t, k: k})
			}
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].dist < candidates[j].dist
	})

	// Working copy of counts
	cWork := make([][]int64, n)
	for t := range n {
		cWork[t] = make([]int64, K)
		copy(cWork[t], counts[t])
	}

	// take0[t][k] = how many we took from (t,k) to assign to slot 0
	take0 := make([][]int64, n)
	for t := range n {
		take0[t] = make([]int64, K)
	}

	need0 := left
	for _, c := range candidates {
		if need0 <= 0 {
			break
		}
		cnt := int(cWork[c.t][c.k])
		use := minInt(cnt, need0)
		take0[c.t][c.k] = int64(use)
		cWork[c.t][c.k] -= int64(use)
		need0 -= use
	}

	// Remaining total to optimize
	totalRem := int64(0)
	for t := range n {
		for k := range K {
			totalRem += cWork[t][k]
		}
	}

	// Handle case where all flow is consumed by y[0]
	if totalRem == 0 {
		F := make([][][]int64, K)
		for k := range K {
			F[k] = make([][]int64, n)
			for t := range n {
				F[k][t] = make([]int64, n)
				F[k][t][0] = take0[t][k]
			}
		}
		y := computeY(F, n, K)
		return &optimizationResult{
			Y: y,
			F: F,
			Diagnostics: diagnostics{
				Total:             int(total),
				StartValue:        left,
				Note:              "all flow consumed by start_value",
				TargetSContinuous: s,
			},
		}, nil
	}

	// Warn if problem is large
	if o.config.WarnTotal > 0 && totalRem > int64(o.config.WarnTotal) {
		slog.WarnContext(ctx, "large optimization problem",
			slog.Int64("total_remaining", totalRem),
			slog.Int("num_minutes", n),
			slog.Int("num_types", K),
		)
	}

	// Phase 3b: Build minimum cost flow graph
	SRC := 0
	SUP0 := 1
	SUPCNT := n * K
	DEM0 := SUP0 + SUPCNT

	// Compute reach[j] = how many can reach slot j
	reach := make([]int64, n)
	for t := range n {
		for k := range K {
			cap := cWork[t][k]
			if cap <= 0 {
				continue
			}
			rad := int(radii[k])
			lo := maxInt(0, t-rad)
			hi := minInt(n-1, t+rad)
			for j := lo; j <= hi; j++ {
				reach[j] += cap
			}
		}
	}
	reach[0] = 0 // slot 0 is already fixed

	// Cap per slot
	capJ := make([]int64, n)
	for j := range n {
		capJ[j] = min64(reach[j], totalRem)
	}

	sumCapJ := int64(0)
	for j := range n {
		sumCapJ += capJ[j]
	}
	if sumCapJ < totalRem {
		return nil, fmt.Errorf("infeasible: not enough demand capacity after fixing y[0]")
	}

	SLOT0 := DEM0 + n
	SLOTCNT := int64(0)
	for j := range n {
		SLOTCNT += capJ[j]
	}
	SNK := SLOT0 + int(SLOTCNT)

	mcf := NewMinCostFlow(SNK + 1)

	// Helper to compute supply node ID
	supID := func(t, k int) int {
		return SUP0 + (t*K + k)
	}

	// SRC -> SUP edges
	for t := range n {
		for k := range K {
			cap := int(cWork[t][k])
			if cap > 0 {
				mcf.AddEdge(SRC, supID(t, k), cap, 0)
			}
		}
	}

	// SUP -> DEM edges (only within movement radius)
	for t := range n {
		for k := range K {
			cap := int(cWork[t][k])
			if cap <= 0 {
				continue
			}
			rad := int(radii[k])
			lo := maxInt(0, t-rad)
			hi := minInt(n-1, t+rad)
			for j := lo; j <= hi; j++ {
				if j == 0 {
					continue // slot 0 is pre-allocated
				}
				mcf.AddEdge(supID(t, k), DEM0+j, cap, moveCost(t, j))
			}
		}
	}

	// DEM -> SLOT edges (marginal cost) and SLOT -> SNK
	slotBase := make([]int64, n)
	cur := int64(0)
	for j := range n {
		slotBase[j] = cur
		cur += capJ[j]
	}

	for j := range n {
		cj := int(capJ[j])
		if cj <= 0 {
			continue
		}
		for m := 1; m <= cj; m++ {
			sidx := int(slotBase[j]) + (m - 1)
			mc := fitMarginalCost(j, m)
			mcf.AddEdge(DEM0+j, SLOT0+sidx, 1, mc)
			mcf.AddEdge(SLOT0+sidx, SNK, 1, 0)
		}
	}

	// Phase 3c: Solve min cost flow
	flowed, cost := mcf.Flow(SRC, SNK, int(totalRem))
	if flowed < int(totalRem) {
		return nil, fmt.Errorf("infeasible: could only send %d/%d units", flowed, totalRem)
	}

	// Phase 4: Restore flow assignments F[k][t][j]
	F := make([][][]int64, K)
	for k := range K {
		F[k] = make([][]int64, n)
		for t := range n {
			F[k][t] = make([]int64, n)
		}
	}

	// Add pre-allocated flow to slot 0
	for t := range n {
		for k := range K {
			if take0[t][k] > 0 {
				F[k][t][0] += take0[t][k]
			}
		}
	}

	// Extract flow from SUP -> DEM reverse edges
	for t := range n {
		for k := range K {
			u := supID(t, k)
			for _, e := range mcf.g[u] {
				if e.To >= DEM0 && e.To < DEM0+n {
					j := e.To - DEM0
					// Flow on this edge = capacity of reverse edge
					rev := mcf.g[e.To][e.Rev]
					fij := rev.Cap
					if fij > 0 {
						F[k][t][j] += int64(fij)
					}
				}
			}
		}
	}

	y := computeY(F, n, K)

	// Sanity checks
	sumY := int64(0)
	for _, v := range y {
		sumY += v
	}
	if sumY != total {
		return nil, fmt.Errorf("sum mismatch: got %d, expected %d", sumY, total)
	}
	if int(y[0]) != left {
		return nil, fmt.Errorf("anchor mismatch: y[0]=%d, expected %d", y[0], left)
	}

	return &optimizationResult{
		Y: y,
		F: F,
		Diagnostics: diagnostics{
			Total:                   int(total),
			StartValue:              left,
			TotalRemainingOptimized: int(totalRem),
			CostScaled:              cost,
			TargetSContinuous:       s,
			Radii:                   radii,
		},
	}, nil
}

// buildAllocationsFromResult builds target allocations from optimization result.
func (o *OptimizationStrategy) buildAllocationsFromResult(
	window *TimeWindow,
	y []int64,
	input SmoothingInput,
) []TargetAllocation {
	n := window.NumMinutes
	allocations := make([]TargetAllocation, n)

	for i := range n {
		key := window.MinuteKeys[i]

		currentCount := 0
		if count, ok := input.CurrentByMinute[key]; ok {
			currentCount = count
		}

		target := int(y[i])
		available := target - currentCount

		// Apply capacity constraint if set
		if input.CapPerMinute > 0 && currentCount+available > input.CapPerMinute {
			available = input.CapPerMinute - currentCount
		}
		if available < 0 {
			available = 0
		}

		allocations[i] = TargetAllocation{
			MinuteKey:    key,
			MinuteTime:   window.MinuteTimes[i],
			Target:       target,
			CurrentCount: currentCount,
			Available:    available,
		}
	}

	return allocations
}

// buildPassthroughAllocations builds passthrough allocations as ultimate fallback.
func (o *OptimizationStrategy) buildPassthroughAllocations(
	window *TimeWindow,
	input SmoothingInput,
) []TargetAllocation {
	n := window.NumMinutes

	// Build raw distribution from CountByMinute (no smoothing)
	raw := make([]float64, n)
	for i, key := range window.MinuteKeys {
		if count, ok := input.CountByMinute[key]; ok {
			raw[i] = float64(count)
		}
	}

	// Normalize to preserve total count
	normalized := normalizeToSum(raw, input.TotalCount)

	return buildTargetAllocations(window, normalized, input)
}
