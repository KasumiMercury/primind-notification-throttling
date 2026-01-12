package health

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// Status represents the health status of a service or dependency.
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusUnhealthy Status = "unhealthy"
)

// CheckResult represents the health check result for a single dependency.
type CheckResult struct {
	Status    Status `json:"status"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

// HealthStatus represents the overall health status of the service.
type HealthStatus struct {
	Status  Status                 `json:"status"`
	Version string                 `json:"version,omitempty"`
	Checks  map[string]CheckResult `json:"checks,omitempty"`
}

// Checker performs health checks on service dependencies.
type Checker struct {
	redisClient *redis.Client
	version     string
}

// NewChecker creates a new health checker with the given dependencies.
func NewChecker(redisClient *redis.Client, version string) *Checker {
	return &Checker{
		redisClient: redisClient,
		version:     version,
	}
}

// Check performs health checks on all dependencies and returns the overall status.
func (c *Checker) Check(ctx context.Context) *HealthStatus {
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	status := &HealthStatus{
		Status:  StatusHealthy,
		Version: c.version,
		Checks:  make(map[string]CheckResult),
	}

	// Redis check
	if c.redisClient != nil {
		start := time.Now()
		if err := c.redisClient.Ping(checkCtx).Err(); err != nil {
			status.Status = StatusUnhealthy
			status.Checks["redis"] = CheckResult{
				Status: StatusUnhealthy,
				Error:  err.Error(),
			}
		} else {
			status.Checks["redis"] = CheckResult{
				Status:    StatusHealthy,
				LatencyMs: time.Since(start).Milliseconds(),
			}
		}
	}

	return status
}

// LiveHandler returns a Gin handler for liveness probes.
func (c *Checker) LiveHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

// ReadyHandler returns a Gin handler for readiness probes.
func (c *Checker) ReadyHandler() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		status := c.Check(ctx.Request.Context())

		httpStatus := http.StatusOK
		if status.Status != StatusHealthy {
			httpStatus = http.StatusServiceUnavailable
		}

		ctx.JSON(httpStatus, status)
	}
}
