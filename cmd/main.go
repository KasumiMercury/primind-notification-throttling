package main

import (
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/KasumiMercury/primind-notification-throttling/internal/client"
	"github.com/KasumiMercury/primind-notification-throttling/internal/config"
	"github.com/KasumiMercury/primind-notification-throttling/internal/handler"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Initialize slog with JSON handler
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))
	slog.SetDefault(logger)

	if cfg.RemindTimeManagementURL == "" {
		slog.Error("REMIND_TIME_MANAGEMENT_URL environment variable is required")
		os.Exit(1)
	}

	// Initialize dependencies
	remindTimeClient := client.NewRemindTimeClient(cfg.RemindTimeManagementURL)
	throttleService := service.NewThrottleService(remindTimeClient)
	throttleHandler := handler.NewThrottleHandler(throttleService, cfg)

	// Setup router
	r := gin.Default()

	// API routes
	v1 := r.Group("/api/v1")
	{
		v1.POST("/throttle", throttleHandler.HandleThrottle)
	}

	// Start server
	slog.Info("starting server",
		slog.String("port", cfg.Port),
		slog.String("remind_time_management_url", cfg.RemindTimeManagementURL),
		slog.Int("time_range_minutes", cfg.TimeRangeMinutes),
	)
	if err := r.Run(":" + cfg.Port); err != nil {
		slog.Error("failed to start server", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
