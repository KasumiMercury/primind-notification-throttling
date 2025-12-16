package handler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/KasumiMercury/primind-notification-throttling/internal/config"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service"
)

type ThrottleHandler struct {
	throttleService *service.ThrottleService
	config          *config.Config
}

func NewThrottleHandler(throttleService *service.ThrottleService, cfg *config.Config) *ThrottleHandler {
	return &ThrottleHandler{
		throttleService: throttleService,
		config:          cfg,
	}
}

func (h *ThrottleHandler) HandleThrottle(c *gin.Context) {
	ctx := c.Request.Context()

	now := time.Now()
	start := now
	end := now.Add(time.Duration(h.config.TimeRangeMinutes) * time.Minute)

	slog.Info("processing throttle request",
		slog.Time("start", start),
		slog.Time("end", end),
	)

	result, err := h.throttleService.ProcessReminds(ctx, start, end)
	if err != nil {
		slog.Error("throttle processing failed",
			slog.String("error", err.Error()),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	slog.Info("throttle completed",
		slog.Int("processed", result.ProcessedCount),
		slog.Int("success", result.SuccessCount),
		slog.Int("failed", result.FailedCount),
	)

	c.JSON(http.StatusOK, result)
}
