//go:build gcloud

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

	if cfg.RemindTimeManagementURL == "" {
		slog.Error("REMIND_TIME_MANAGEMENT_URL environment variable is required")
		return 1
	}

	if err := cfg.TaskQueue.Validate(); err != nil {
		slog.Error("task queue configuration error", slog.String("error", err.Error()))
		return 1
	}

	ctx := context.Background()

	// Initialize dependencies
	remindTimeClient := client.NewRemindTimeClient(cfg.RemindTimeManagementURL)

	// Initialize Cloud Tasks client
	cloudTasksClient, err := client.NewCloudTasksClient(ctx, client.CloudTasksConfig{
		ProjectID:  cfg.TaskQueue.GCloudProjectID,
		LocationID: cfg.TaskQueue.GCloudLocationID,
		QueueID:    cfg.TaskQueue.GCloudQueueID,
		TargetURL:  cfg.TaskQueue.GCloudTargetURL,
		MaxRetries: cfg.TaskQueue.MaxRetries,
	})
	if err != nil {
		slog.Error("failed to create cloud tasks client", slog.String("error", err.Error()))
		return 1
	}
	defer func() {
		if err := cloudTasksClient.Close(); err != nil {
			slog.Warn("failed to close cloud tasks client", slog.String("error", err.Error()))
		}
	}()

	slog.Info("task queue initialized",
		slog.String("type", "cloud_tasks"),
		slog.String("project", cfg.TaskQueue.GCloudProjectID),
		slog.String("location", cfg.TaskQueue.GCloudLocationID),
		slog.String("queue", cfg.TaskQueue.GCloudQueueID),
	)

	var taskQueue client.TaskQueue = cloudTasksClient

	throttleService := service.NewThrottleService(remindTimeClient, taskQueue)
	throttleHandler := handler.NewThrottleHandler(throttleService, cfg)

	// Setup router
	r := gin.Default()

	// API routes
	v1 := r.Group("/api/v1")
	{
		v1.POST("/throttle", throttleHandler.HandleThrottle)
	}

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: r,
	}

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- srv.ListenAndServe()
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	slog.Info("starting server",
		slog.String("port", cfg.Port),
		slog.String("remind_time_management_url", cfg.RemindTimeManagementURL),
		slog.Int("time_range_minutes", cfg.TimeRangeMinutes),
	)

	select {
	case sig := <-quit:
		slog.Info("shutdown signal received", slog.String("signal", sig.String()))

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("failed to shutdown server", slog.String("error", err.Error()))
			return 1
		}
		return 0

	case err := <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			return 0
		}
		slog.Error("server exited with error", slog.String("error", err.Error()))
		return 1
	}
}
