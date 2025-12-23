package handler

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/KasumiMercury/primind-notification-throttling/internal/config"
	commonv1 "github.com/KasumiMercury/primind-notification-throttling/internal/gen/common/v1"
	throttlev1 "github.com/KasumiMercury/primind-notification-throttling/internal/gen/throttle/v1"
	pjson "github.com/KasumiMercury/primind-notification-throttling/internal/proto"
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
		respondProtoError(c, http.StatusInternalServerError, err.Error())
		return
	}

	slog.Info("throttle completed",
		slog.Int("processed", result.ProcessedCount),
		slog.Int("success", result.SuccessCount),
		slog.Int("failed", result.FailedCount),
	)

	respondProtoThrottleResponse(c, http.StatusOK, result)
}

func respondProtoError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": message})
}

func respondProtoThrottleResponse(c *gin.Context, status int, result *service.ThrottleResponse) {
	protoResults := make([]*throttlev1.ThrottleResultItem, 0, len(result.Results))
	for _, r := range result.Results {
		protoResults = append(protoResults, &throttlev1.ThrottleResultItem{
			RemindId:  r.RemindID,
			TaskId:    r.TaskID,
			TaskType:  stringToTaskType(r.TaskType),
			FcmTokens: r.FCMTokens,
			Success:   r.Success,
			Error:     r.Error,
		})
	}

	resp := &throttlev1.ThrottleResponse{
		ProcessedCount: int32(result.ProcessedCount),
		SuccessCount:   int32(result.SuccessCount),
		FailedCount:    int32(result.FailedCount),
		Results:        protoResults,
	}
	respBytes, _ := pjson.Marshal(resp)
	c.Data(status, "application/json", respBytes)
}

// stringToTaskType converts a string to a proto TaskType enum
func stringToTaskType(s string) commonv1.TaskType {
	upper := "TASK_TYPE_" + strings.ToUpper(s)
	if v, ok := commonv1.TaskType_value[upper]; ok {
		return commonv1.TaskType(v)
	}
	return commonv1.TaskType_TASK_TYPE_UNSPECIFIED
}
