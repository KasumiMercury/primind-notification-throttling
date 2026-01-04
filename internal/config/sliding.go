package config

import (
	"os"
)

const (
	slidingStrategyEnv = "SLIDING_DISCOVERY_STRATEGY"

	defaultSlidingStrategy = "bestfit"
)

type SlidingStrategy string

const (
	SlidingStrategyGreedy   SlidingStrategy = "greedy"
	SlidingStrategyBestFit  SlidingStrategy = "bestfit"
	SlidingStrategyPriority SlidingStrategy = "priority"
)

type SlidingConfig struct {
	Strategy SlidingStrategy
}

func LoadSlidingConfig() *SlidingConfig {
	strategy := SlidingStrategy(os.Getenv(slidingStrategyEnv))
	if strategy == "" {
		strategy = defaultSlidingStrategy
	}

	if strategy != SlidingStrategyGreedy && strategy != SlidingStrategyBestFit && strategy != SlidingStrategyPriority {
		strategy = defaultSlidingStrategy
	}

	return &SlidingConfig{
		Strategy: strategy,
	}
}
