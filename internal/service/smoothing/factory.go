package smoothing

import (
	"log/slog"

	"github.com/KasumiMercury/primind-notification-throttling/internal/config"
)

func NewStrategy(cfg *config.SmoothingConfig) Strategy {
	if cfg == nil {
		slog.Info("smoothing config is nil, using passthrough strategy")
		return NewPassthroughStrategy()
	}

	switch cfg.Strategy {
	case config.SmoothingStrategyOptimization:
		slog.Info("using optimization smoothing strategy",
			slog.Float64("lambda", cfg.Lambda),
			slog.Int("default_radius", cfg.DefaultRadius),
		)
		return NewOptimizationStrategy(&OptimizationConfig{
			LambdaTargetSmooth: cfg.Lambda,
			DefaultRadius:      cfg.DefaultRadius,
		})

	case config.SmoothingStrategyTriangular:
		slog.Info("using triangular kernel smoothing strategy",
			slog.Int("kernel_radius", cfg.KernelRadius),
		)
		return NewTriangularKernelStrategy(cfg.KernelRadius)

	case config.SmoothingStrategyPassthrough:
		fallthrough
	default:
		slog.Info("using passthrough smoothing strategy")
		return NewPassthroughStrategy()
	}
}
