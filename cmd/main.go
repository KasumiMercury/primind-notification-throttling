package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/KasumiMercury/primind-notification-throttling/internal/client"
	"github.com/KasumiMercury/primind-notification-throttling/internal/config"
	"github.com/KasumiMercury/primind-notification-throttling/internal/handler"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service"
)

func main() {
	os.Exit(run())
}

func run() int {
	// Load configuration
	cfg := config.Load()

	// Initialize slog with JSON handler
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))
	slog.SetDefault(logger)

	// Validate configuration
	if cfg.RemindTimeManagementURL == "" {
		slog.Error("REMIND_TIME_MANAGEMENT_URL environment variable is required")
		return 1
	}

	if err := cfg.TaskQueue.Validate(); err != nil {
		slog.Error("task queue configuration error", slog.String("error", err.Error()))
		return 1
	}

	// Create cancellable context for cleanup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize dependencies
	remindTimeClient := client.NewRemindTimeClient(cfg.RemindTimeManagementURL)

	// Initialize task queue
	taskQueue, cleanup, err := initTaskQueue(ctx, cfg)
	if err != nil {
		slog.Error("failed to initialize task queue", slog.String("error", err.Error()))
		return 1
	}
	if cleanup != nil {
		defer func() {
			if err := cleanup(); err != nil {
				slog.Error("task queue cleanup error", slog.String("error", err.Error()))
			}
		}()
	}

	// Create services and handlers
	throttleService := service.NewThrottleService(remindTimeClient, taskQueue)
	throttleHandler := handler.NewThrottleHandler(throttleService, cfg)
	remindCancelHandler := handler.NewRemindCancelHandler(throttleService)

	// Setup router
	r := gin.Default()

	// API routes
	v1 := r.Group("/api/v1")
	{
		v1.POST("/throttle", throttleHandler.HandleThrottle)
		v1.POST("/remind/cancel", remindCancelHandler.HandleRemindCancel)
	}

	// Create HTTP server
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	// Start server in goroutine
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("starting server",
			slog.String("port", cfg.Port),
			slog.String("remind_time_management_url", cfg.RemindTimeManagementURL),
			slog.Int("time_range_minutes", cfg.TimeRangeMinutes),
		)
		serverErr <- srv.ListenAndServe()
	}()

	// Wait for shutdown signal or server error
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		slog.Info("shutdown signal received", slog.String("signal", sig.String()))
		cancel()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("failed to shutdown server", slog.String("error", err.Error()))
			return 1
		}

		slog.Info("server exited properly")
		return 0

	case err := <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			return 0
		}
		slog.Error("server exited with error", slog.String("error", err.Error()))
		return 1
	}
}
