package middleware

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/observability/logging"
	"github.com/KasumiMercury/primind-notification-throttling/internal/observability/metrics"
	"github.com/KasumiMercury/primind-notification-throttling/internal/observability/tracing"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
)

type GinConfig struct {
	// SkipPaths are paths that skip observability
	SkipPaths []string
	Module    logging.Module
	// ModuleResolver returns a module for the request when module depends on path
	ModuleResolver func(*gin.Context) logging.Module
	Worker         bool
	// JobNameResolver returns a job name for worker-style logging
	JobNameResolver func(*gin.Context) string
	TracerName      string
	// HTTPMetrics records HTTP request metrics
	HTTPMetrics *metrics.HTTPMetrics
}

func Gin(cfg GinConfig) gin.HandlerFunc {
	skipSet := make(map[string]struct{}, len(cfg.SkipPaths))
	for _, p := range cfg.SkipPaths {
		skipSet[p] = struct{}{}
	}

	return func(c *gin.Context) {
		// Skip observability for configured paths
		if _, skip := skipSet[c.Request.URL.Path]; skip {
			c.Next()

			return
		}

		start := time.Now()

		requestID := logging.ValidateAndExtractRequestID(c.Request.Header.Get("x-request-id"))
		ctx := logging.WithRequestID(c.Request.Context(), requestID)
		module := cfg.Module
		if cfg.ModuleResolver != nil {
			module = cfg.ModuleResolver(c)
		}
		if module != "" {
			ctx = logging.WithModule(ctx, module)
		}

		ctx = tracing.ExtractFromHTTPRequest(ctx, c.Request)

		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		tracer := otel.Tracer(cfg.TracerName)
		ctx, span := tracer.Start(ctx, fmt.Sprintf("%s %s", c.Request.Method, path))
		defer span.End()

		c.Request = c.Request.WithContext(ctx)

		c.Header("x-request-id", requestID)
		c.Request.Header.Set("x-request-id", requestID)

		finishEvent := "http.request.finish"
		finishMessage := "request completed"
		jobName := ""
		if cfg.Worker {
			finishEvent = "job.finish"
			finishMessage = "job finished"
			if cfg.JobNameResolver != nil {
				jobName = cfg.JobNameResolver(c)
			}
			if jobName == "" {
				jobName = c.Request.URL.Path
			}

			startAttrs := []slog.Attr{
				slog.String("event", "job.start"),
				slog.String("method", c.Request.Method),
				slog.String("path", c.Request.URL.Path),
				slog.String("remote_addr", c.ClientIP()),
				slog.String("job.name", jobName),
				slog.String("job.id", requestID),
			}
			slog.LogAttrs(ctx, slog.LevelInfo, "job started", startAttrs...)
		}

		c.Next()

		duration := time.Since(start)
		status := c.Writer.Status()

		if cfg.HTTPMetrics != nil {
			cfg.HTTPMetrics.Record(ctx, c.Request.Method, path, status, duration)
		}

		finishAttrs := []slog.Attr{
			slog.String("event", finishEvent),
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.String("remote_addr", c.ClientIP()),
			slog.Int("status", status),
			slog.Duration("duration", duration),
		}
		if cfg.Worker {
			finishAttrs = append(finishAttrs,
				slog.String("job.name", jobName),
				slog.String("job.id", requestID),
			)
		}
		slog.LogAttrs(ctx, slog.LevelInfo, finishMessage, finishAttrs...)
	}
}
