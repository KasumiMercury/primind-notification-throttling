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
	planService     *service.PlanService
	config          *config.Config
}

func NewThrottleHandler(
	throttleService *service.ThrottleService,
	planService *service.PlanService,
	cfg *config.Config,
) *ThrottleHandler {
	return &ThrottleHandler{
		throttleService: throttleService,
		planService:     planService,
		config:          cfg,
	}
}

func (h *ThrottleHandler) HandleThrottle(c *gin.Context) {
	ctx := c.Request.Context()

	now := time.Now().Truncate(time.Minute)

	if h.planService != nil {
		planStart := now.Add(time.Duration(h.config.Throttle.ConfirmWindowMinutes) * time.Minute)
		planEnd := now.Add(time.Duration(h.config.Throttle.PlanningWindowMinutes) * time.Minute)

		slog.InfoContext(ctx, "starting planning phase",
			slog.Time("plan_start", planStart),
			slog.Time("plan_end", planEnd),
		)

		planResult, err := h.planService.PlanReminds(ctx, planStart, planEnd)
		if err != nil {
			slog.WarnContext(ctx, "planning phase failed, continuing to commit phase",
				slog.String("error", err.Error()),
			)
			// Continue to commit phase even if planning fails
		} else {
			slog.InfoContext(ctx, "planning phase completed",
				slog.Int("planned_count", planResult.PlannedCount),
				slog.Int("skipped_count", planResult.SkippedCount),
				slog.Int("shifted_count", planResult.ShiftedCount),
			)
		}
	}

	commitStart := now
	commitEnd := now.Add(time.Duration(h.config.Throttle.ConfirmWindowMinutes) * time.Minute)

	slog.InfoContext(ctx, "starting commit phase",
		slog.Time("commit_start", commitStart),
		slog.Time("commit_end", commitEnd),
	)

	result, err := h.throttleService.ProcessReminds(ctx, commitStart, commitEnd)
	if err != nil {
		slog.ErrorContext(ctx, "commit phase failed",
			slog.String("error", err.Error()),
		)
		respondProtoError(c, http.StatusInternalServerError, err.Error())
		return
	}

	slog.InfoContext(ctx, "commit phase completed",
		slog.Int("processed", result.ProcessedCount),
		slog.Int("success", result.SuccessCount),
		slog.Int("failed", result.FailedCount),
		slog.Int("skipped", result.SkippedCount),
		slog.Int("shifted", result.ShiftedCount),
	)

	respondProtoThrottleResponse(c, http.StatusOK, result)
}

func respondProtoError(c *gin.Context, status int, message string) {
	resp := &throttlev1.ErrorResponse{
		Error:   "processing_error",
		Message: message,
	}

	respBytes, err := pjson.Marshal(resp)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Data(status, "application/json", respBytes)
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
