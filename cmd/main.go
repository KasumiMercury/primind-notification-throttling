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
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"

	"github.com/KasumiMercury/primind-notification-throttling/internal/config"
	"github.com/KasumiMercury/primind-notification-throttling/internal/handler"
	"github.com/KasumiMercury/primind-notification-throttling/internal/health"
	"github.com/KasumiMercury/primind-notification-throttling/internal/infra/repository"
	"github.com/KasumiMercury/primind-notification-throttling/internal/infra/throttlerecorder"
	"github.com/KasumiMercury/primind-notification-throttling/internal/infra/timemgmt"
	"github.com/KasumiMercury/primind-notification-throttling/internal/observability/logging"
	"github.com/KasumiMercury/primind-notification-throttling/internal/observability/metrics"
	"github.com/KasumiMercury/primind-notification-throttling/internal/observability/middleware"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service/lane"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service/plan"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service/sliding"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service/slot"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service/smoothing"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service/throttle"
)

// Version is set via ldflags at build time
var Version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	obs, err := initObservability(ctx)
	if err != nil {
		slog.Error("failed to initialize observability", slog.String("error", err.Error()))
		return 1
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := obs.Shutdown(shutdownCtx); err != nil {
			slog.Warn("observability shutdown error", slog.String("error", err.Error()))
		}
	}()

	slog.SetDefault(obs.Logger())

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", slog.String("error", err.Error()))
		return 1
	}

	// Validate configuration
	if err := config.ValidateForRun(cfg); err != nil {
		slog.Error("configuration validation error", slog.String("error", err.Error()))
		return 1
	}

	if err := cfg.TaskQueue.Validate(); err != nil {
		slog.Error("task queue configuration error", slog.String("error", err.Error()))
		return 1
	}

	httpMetrics, err := metrics.NewHTTPMetrics()
	if err != nil {
		slog.Error("failed to initialize HTTP metrics", slog.String("error", err.Error()))
		return 1
	}

	throttleMetrics, err := metrics.NewThrottleMetrics()
	if err != nil {
		slog.Error("failed to initialize throttle metrics", slog.String("error", err.Error()))
		return 1
	}

	// Initialize throttle result recorder (InfluxDB for local, BigQuery for gcloud)
	resultRecorderCfg := throttlerecorder.LoadConfig()
	resultRecorder, err := throttlerecorder.NewRecorder(ctx, resultRecorderCfg)
	if err != nil {
		slog.Error("failed to initialize throttle result recorder", slog.String("error", err.Error()))
		return 1
	}
	defer func() {
		if err := resultRecorder.Close(); err != nil {
			slog.Warn("failed to close throttle result recorder", slog.String("error", err.Error()))
		}
	}()

	// Initialize dependencies
	remindTimeClient := timemgmt.NewClient(cfg.RemindTimeManagementURL)

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

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	if err := redisotel.InstrumentTracing(redisClient); err != nil {
		slog.Error("failed to instrument redis tracing",
			slog.String("event", "redis.otel.tracing.fail"),
			slog.String("error", err.Error()),
		)
		return 1
	}

	if err := redisotel.InstrumentMetrics(redisClient); err != nil {
		slog.Error("failed to instrument redis metrics",
			slog.String("event", "redis.otel.metrics.fail"),
			slog.String("error", err.Error()),
		)
		return 1
	}

	if err := redisClient.Ping(ctx).Err(); err != nil {
		slog.Error("failed to connect redis",
			slog.String("event", "redis.connect.fail"),
			slog.String("error", err.Error()),
		)
		return 1
	}

	defer func() {
		if err := redisClient.Close(); err != nil {
			slog.Warn("failed to close redis client", slog.String("error", err.Error()))
		}
	}()

	slog.Info("redis connected",
		slog.String("addr", cfg.Redis.Addr),
	)

	throttleRepo := repository.NewThrottleRepository(redisClient)

	laneClassifier := lane.NewClassifier()
	slotCounter := slot.NewCounter(throttleRepo)
	slotCalculator := slot.NewCalculator(slotCounter, cfg.Throttle.RequestCapPerMinute)

	smoothingStrategy := smoothing.NewStrategy(cfg.Smoothing)
	slideDiscovery := sliding.NewDiscovery(cfg.Sliding)

	throttleService := throttle.NewService(
		remindTimeClient,
		taskQueue,
		throttleRepo,
		laneClassifier,
		slotCalculator,
		throttleMetrics,
	)
	planService := plan.NewService(
		remindTimeClient,
		throttleRepo,
		laneClassifier,
		slotCalculator,
		slotCounter,
		smoothingStrategy,
		slideDiscovery,
		throttleMetrics,
		cfg.Throttle.RequestCapPerMinute,
	)
	throttleHandler := handler.NewThrottleHandler(throttleService, planService, cfg, throttleMetrics, resultRecorder)
	remindCancelHandler := handler.NewRemindCancelHandler(throttleService)

	// Setup router with observability middleware
	r := gin.New()
	r.Use(middleware.Gin(middleware.GinConfig{
		SkipPaths:  []string{"/health", "/health/live", "/health/ready", "/metrics"},
		Module:     logging.Module("notification-throttling"),
		Worker:     true,
		TracerName: "github.com/KasumiMercury/primind-notification-throttling/internal/observability/middleware",
		JobNameResolver: func(c *gin.Context) string {
			if messageType := c.Request.Header.Get("message_type"); messageType != "" {
				return messageType
			}
			if eventType := c.Request.Header.Get("event_type"); eventType != "" {
				return eventType
			}
			return c.Request.URL.Path
		},
		HTTPMetrics: httpMetrics,
	}))
	r.Use(middleware.PanicRecoveryGin())

	// Health check endpoints
	healthChecker := health.NewChecker(redisClient, Version)
	r.GET("/health/live", healthChecker.LiveHandler())
	r.GET("/health/ready", healthChecker.ReadyHandler())
	r.GET("/health", healthChecker.ReadyHandler())

	// API routes
	v1 := r.Group("/api/v1")
	{
		v1.POST("/throttle", throttleHandler.HandleThrottle)
		v1.POST("/throttle/batch", throttleHandler.HandleThrottleBatch)
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
			slog.Int("confirm_window_minutes", cfg.Throttle.ConfirmWindowMinutes),
			slog.Int("request_cap_per_minute", cfg.Throttle.RequestCapPerMinute),
			slog.Int("planning_window_minutes", cfg.Throttle.PlanningWindowMinutes),
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
