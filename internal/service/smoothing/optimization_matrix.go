package smoothing

import "math"

// MatrixInput holds the converted input matrices for optimization.
// Counts[t][k] is the number of notifications at minute t with radius type k.
// Radii[k] is the radius value for type k.
type MatrixInput struct {
	Counts [][]int64 // (n, K) matrix
	Radii  []int64   // (K,) array
	N      int       // number of time slots
	K      int       // number of radius types
}

// SmoothTargetContinuous creates a smooth continuous target distribution.
// It minimizes ||s-x||^2 + lambda*||D2*s||^2 subject to:
// - s >= 0 (non-negativity)
// - sum(s) = sum(x) (total preservation)
// - s[0] = startValue (initial value constraint)
//
// This uses projected gradient descent to solve the constrained optimization.
func SmoothTargetContinuous(x []float64, startValue int, lambdaSmooth float64, maxIter int) []float64 {
	n := len(x)
	total := 0.0
	for _, v := range x {
		total += v
	}

	// Edge case: single element
	if n == 1 {
		return []float64{float64(startValue)}
	}

	// Build D2'*D2 matrix directly as a 2D slice
	// D2 is (n-2) x n matrix where D2[t,:] = [0..0, 1, -2, 1, 0..0]
	// D2'D2 is a symmetric banded matrix
	dtd := make([][]float64, n)
	for i := range dtd {
		dtd[i] = make([]float64, n)
	}

	if n >= 3 {
		// Compute D2' * D2 directly
		// Each row t of D2 contributes: D2[t,:]' * D2[t,:]
		for t := range n - 2 {
			// D2[t, t] = 1, D2[t, t+1] = -2, D2[t, t+2] = 1
			// Outer product contribution to D2'D2:
			dtd[t][t] += 1
			dtd[t][t+1] += -2
			dtd[t][t+2] += 1
			dtd[t+1][t] += -2
			dtd[t+1][t+1] += 4
			dtd[t+1][t+2] += -2
			dtd[t+2][t] += 1
			dtd[t+2][t+1] += -2
			dtd[t+2][t+2] += 1
		}
	}

	// Compute step size from maximum eigenvalue of H = I + lambda*D2'D2
	step := estimateStepSize(n, lambdaSmooth)

	// Initialize s with projected x
	s := make([]float64, n)
	copy(s, x)
	s = projectConstraints(s, float64(startValue), total)

	var lastObj *float64

	// Projected gradient descent loop
	for range maxIter {
		// Compute gradient: grad = (s - x) + lambda * D2'D2 * s
		grad := make([]float64, n)
		for i := range n {
			grad[i] = s[i] - x[i]
			// Add lambda * (D2'D2 * s)[i]
			for j := range n {
				grad[i] += lambdaSmooth * dtd[i][j] * s[j]
			}
		}

		// Gradient step: s2 = s - step * grad
		s2 := make([]float64, n)
		for i := range n {
			s2[i] = s[i] - step*grad[i]
		}

		// Project onto constraint set
		s2 = projectConstraints(s2, float64(startValue), total)

		// Compute objective for convergence check
		obj := 0.0
		for i := range n {
			diff := s2[i] - x[i]
			obj += 0.5 * diff * diff
		}

		// Check convergence
		if lastObj != nil && math.Abs(*lastObj-obj)/(math.Abs(*lastObj)+1e-9) < 1e-10 {
			s = s2
			break
		}

		s = s2
		objCopy := obj
		lastObj = &objCopy
	}

	return s
}

// estimateStepSize returns a conservative step size for gradient descent.
// Based on the maximum eigenvalue of H = I + lambda*D2'D2.
func estimateStepSize(n int, lambda float64) float64 {
	// For second-order difference, max eigenvalue of D2'D2 is approximately 16
	// So max eigenvalue of H is approximately 1 + 16*lambda
	lmax := 1.0 + 16.0*lambda
	return 1.0 / math.Max(lmax, 1e-9)
}

// projectConstraints projects a vector onto the constraint set:
// - s[0] = startValue
// - sum(s) = total
// - s >= 0
func projectConstraints(v []float64, startValue, total float64) []float64 {
	n := len(v)
	y := make([]float64, n)
	copy(y, v)

	y[0] = startValue

	// Distribute sum constraint over indices 1..n-1
	restTarget := total - startValue
	sumRest := 0.0
	for i := 1; i < n; i++ {
		sumRest += y[i]
	}
	adjustment := (restTarget - sumRest) / float64(n-1)
	for i := 1; i < n; i++ {
		y[i] += adjustment
	}

	// Apply non-negativity and readjust sum (iterate a few times)
	for i := range n {
		if y[i] < 0 {
			y[i] = 0
		}
	}

	for range 3 {
		y[0] = startValue
		restTarget = total - startValue

		// Find positive elements in 1..n-1
		sumPos := 0.0
		countPos := 0
		for i := 1; i < n; i++ {
			if y[i] > 0 {
				sumPos += y[i]
				countPos++
			}
		}

		if countPos == 0 {
			// All zeros, distribute evenly
			for i := 1; i < n; i++ {
				y[i] = restTarget / float64(n-1)
			}
			break
		}

		err := restTarget - sumPos
		for i := 1; i < n; i++ {
			if y[i] > 0 {
				y[i] += err / float64(countPos)
			}
		}

		for i := range n {
			if y[i] < 0 {
				y[i] = 0
			}
		}
	}

	// Final sum adjustment
	y[0] = startValue
	restTarget = total - startValue
	sumRest = 0.0
	for i := 1; i < n; i++ {
		sumRest += y[i]
	}
	adjustment = (restTarget - sumRest) / float64(n-1)
	for i := 1; i < n; i++ {
		y[i] += adjustment
	}
	for i := range n {
		if y[i] < 0 {
			y[i] = 0
		}
	}

	return y
}

// computeY calculates y[j] = sum of F[k][t][j] over all k and t.
func computeY(F [][][]int64, n, K int) []int64 {
	y := make([]int64, n)
	for k := range K {
		for t := range n {
			for j := range n {
				y[j] += F[k][t][j]
			}
		}
	}
	return y
}

// absInt returns the absolute value of x.
func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// minInt returns the minimum of a and b.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// maxInt returns the maximum of a and b.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// min64 returns the minimum of a and b.
func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
