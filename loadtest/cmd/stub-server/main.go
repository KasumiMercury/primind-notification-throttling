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

	"github.com/KasumiMercury/primind-notification-throttling/loadtest/internal/stub"
)

func main() {
	os.Exit(run())
}

func run() int {
	logLevel := slog.LevelInfo
	if l := os.Getenv("LOG_LEVEL"); l == "debug" {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	storage := stub.NewBucketStorage()
	handler := stub.NewHandler(storage)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(requestLogger())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	r.POST("/test/reset", handler.HandleReset)
	r.POST("/test/seed", handler.HandleSeed)

	v1 := r.Group("/api/v1")
	{
		v1.GET("/reminds", handler.HandleGetReminds)
		v1.POST("/reminds/:id/throttled", handler.HandleUpdateThrottled)
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("starting stub-server",
			slog.String("port", port),
		)
		serverErr <- srv.ListenAndServe()
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		slog.Info("shutdown signal received", slog.String("signal", sig.String()))

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("failed to shutdown server", slog.String("error", err.Error()))
			return 1
		}

		slog.Info("stub-server exited properly")
		return 0

	case err := <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			return 0
		}
		slog.Error("server exited with error", slog.String("error", err.Error()))
		return 1
	}
}

func requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		if path == "/health" {
			return
		}

		slog.Info("request",
			slog.String("method", c.Request.Method),
			slog.String("path", path),
			slog.Int("status", status),
			slog.Duration("latency", latency),
		)
	}
}
