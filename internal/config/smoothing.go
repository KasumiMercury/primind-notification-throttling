package config

import (
	"os"
	"strconv"
)

const (
	smoothingStrategyEnv        = "SMOOTHING_STRATEGY"
	smoothingKernelRadiusEnv    = "SMOOTHING_KERNEL_RADIUS"
	smoothingLambdaEnv          = "SMOOTHING_LAMBDA"
	smoothingDefaultRadiusEnv   = "SMOOTHING_DEFAULT_RADIUS"

	defaultSmoothingStrategy      = "optimization"
	defaultSmoothingKernelRadius  = 5
	defaultSmoothingLambda        = 10.0
	defaultSmoothingDefaultRadius = 5
)

type SmoothingStrategy string

const (
	SmoothingStrategyPassthrough  SmoothingStrategy = "passthrough"
	SmoothingStrategyTriangular   SmoothingStrategy = "triangular"
	SmoothingStrategyOptimization SmoothingStrategy = "optimization"
)

type SmoothingConfig struct {
	Strategy      SmoothingStrategy
	KernelRadius  int     // Used by triangular kernel
	Lambda        float64 // Smoothness parameter for optimization
	DefaultRadius int     // Default radius for optimization when RadiusBatches is nil
}

func LoadSmoothingConfig() *SmoothingConfig {
	strategy := SmoothingStrategy(os.Getenv(smoothingStrategyEnv))
	if strategy == "" {
		strategy = defaultSmoothingStrategy
	}

	validStrategies := map[SmoothingStrategy]bool{
		SmoothingStrategyPassthrough:  true,
		SmoothingStrategyTriangular:   true,
		SmoothingStrategyOptimization: true,
	}
	if !validStrategies[strategy] {
		strategy = defaultSmoothingStrategy
	}

	kernelRadius := defaultSmoothingKernelRadius
	if v := os.Getenv(smoothingKernelRadiusEnv); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			kernelRadius = parsed
		}
	}

	lambda := defaultSmoothingLambda
	if v := os.Getenv(smoothingLambdaEnv); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil && parsed >= 0 {
			lambda = parsed
		}
	}

	defaultRadius := defaultSmoothingDefaultRadius
	if v := os.Getenv(smoothingDefaultRadiusEnv); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
			defaultRadius = parsed
		}
	}

	return &SmoothingConfig{
		Strategy:      strategy,
		KernelRadius:  kernelRadius,
		Lambda:        lambda,
		DefaultRadius: defaultRadius,
	}
}
