//go:build !gcloud

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

	if err := cfg.TaskQueue.Validate(); err != nil {
		slog.Error("task queue configuration error", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Initialize dependencies
	remindTimeClient := client.NewRemindTimeClient(cfg.RemindTimeManagementURL)

	// Initialize Primind Tasks client
	var taskQueue client.TaskQueue
	if cfg.TaskQueue.PrimindTasksURL != "" {
		taskQueue = client.NewPrimindTasksClient(
			cfg.TaskQueue.PrimindTasksURL,
			cfg.TaskQueue.QueueName,
			cfg.TaskQueue.MaxRetries,
		)
		slog.Info("task queue initialized",
			slog.String("type", "primind_tasks"),
			slog.String("url", cfg.TaskQueue.PrimindTasksURL),
			slog.String("queue", cfg.TaskQueue.QueueName),
		)
	} else {
		slog.Warn("PRIMIND_TASKS_URL not set, task queue registration disabled")
	}

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

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	// Start server
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("starting server",
			slog.String("port", cfg.Port),
			slog.String("remind_time_management_url", cfg.RemindTimeManagementURL),
			slog.Int("time_range_minutes", cfg.TimeRangeMinutes),
		)
		serverErr <- srv.ListenAndServe()
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		slog.Info("received shutdown signal", slog.String("signal", sig.String()))
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", slog.String("error", err.Error()))
		}
	}

	slog.Info("shutting down server")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("failed to shutdown server", slog.String("error", err.Error()))
	}

	slog.Info("server exited properly")
}
