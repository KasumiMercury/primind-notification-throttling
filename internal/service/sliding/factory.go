package sliding

import (
	"log/slog"

	"github.com/KasumiMercury/primind-notification-throttling/internal/config"
)

// NewDiscovery creates a new Discovery based on the configuration.
// If cfg is nil, it defaults to BestFitDiscovery.
func NewDiscovery(cfg *config.SlidingConfig) Discovery {
	if cfg == nil {
		slog.Info("sliding config is nil, using default bestfit discovery")
		return NewBestFitDiscovery()
	}

	switch cfg.Strategy {
	case config.SlidingStrategyPriority:
		slog.Info("using priority sliding discovery")
		return NewPriorityDiscovery()
	case config.SlidingStrategyGreedy:
		slog.Info("using greedy sliding discovery")
		return NewGreedyDiscovery()
	case config.SlidingStrategyBestFit:
		fallthrough
	default:
		slog.Info("using bestfit sliding discovery")
		return NewBestFitDiscovery()
	}
}
