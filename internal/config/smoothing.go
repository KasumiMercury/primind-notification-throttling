package config

import (
	"os"
	"strconv"
)

const (
	smoothingStrategyEnv     = "SMOOTHING_STRATEGY"
	smoothingKernelRadiusEnv = "SMOOTHING_KERNEL_RADIUS"

	defaultSmoothingStrategy     = "triangular"
	defaultSmoothingKernelRadius = 5
)

type SmoothingStrategy string

const (
	SmoothingStrategyPassthrough SmoothingStrategy = "passthrough"
	SmoothingStrategyTriangular  SmoothingStrategy = "triangular"
)

type SmoothingConfig struct {
	Strategy     SmoothingStrategy
	KernelRadius int
}

func LoadSmoothingConfig() *SmoothingConfig {
	strategy := SmoothingStrategy(os.Getenv(smoothingStrategyEnv))
	if strategy == "" {
		strategy = defaultSmoothingStrategy
	}

	if strategy != SmoothingStrategyPassthrough && strategy != SmoothingStrategyTriangular {
		strategy = defaultSmoothingStrategy
	}

	kernelRadius := defaultSmoothingKernelRadius
	if v := os.Getenv(smoothingKernelRadiusEnv); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			kernelRadius = parsed
		}
	}

	return &SmoothingConfig{
		Strategy:     strategy,
		KernelRadius: kernelRadius,
	}
}
