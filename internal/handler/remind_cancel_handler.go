package handler

import (
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	throttlev1 "github.com/KasumiMercury/primind-notification-throttling/internal/gen/throttle/v1"
	pjson "github.com/KasumiMercury/primind-notification-throttling/internal/proto"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service"
)

type RemindCancelHandler struct {
	throttleService *service.ThrottleService
}

func NewRemindCancelHandler(throttleService *service.ThrottleService) *RemindCancelHandler {
	return &RemindCancelHandler{
		throttleService: throttleService,
	}
}

func (h *RemindCancelHandler) HandleRemindCancel(c *gin.Context) {
	ctx := c.Request.Context()

	slog.InfoContext(ctx, "handling remind cancel request",
		slog.String("method", c.Request.Method),
		slog.String("path", c.Request.URL.Path),
	)

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		slog.ErrorContext(ctx, "failed to read request body", slog.String("error", err.Error()))
		respondCancelError(c, http.StatusBadRequest, "read_error", "failed to read request body")
		return
	}

	var req throttlev1.CancelRemindRequest
	if err := pjson.Unmarshal(body, &req); err != nil {
		slog.WarnContext(ctx, "request unmarshal failed",
			slog.String("error", err.Error()),
			slog.String("path", c.Request.URL.Path),
		)
		respondCancelError(c, http.StatusBadRequest, "validation_error", err.Error())
		return
	}

	if err := pjson.Validate(&req); err != nil {
		slog.WarnContext(ctx, "request validation failed",
			slog.String("error", err.Error()),
			slog.String("path", c.Request.URL.Path),
		)
		respondCancelError(c, http.StatusBadRequest, "validation_error", err.Error())
		return
	}

	slog.InfoContext(ctx, "processing remind cancelled event",
		slog.String("task_id", req.TaskId),
		slog.String("user_id", req.UserId),
		slog.Int64("deleted_count", req.DeletedCount),
	)

	if err := h.throttleService.HandleRemindCancelled(ctx, &req); err != nil {
		slog.ErrorContext(ctx, "failed to handle remind cancelled event",
			slog.String("task_id", req.TaskId),
			slog.String("error", err.Error()),
		)
		respondCancelError(c, http.StatusInternalServerError, "processing_error", "failed to process cancellation")
		return
	}

	slog.InfoContext(ctx, "remind cancelled event processed successfully",
		slog.String("task_id", req.TaskId),
	)

	respondCancelSuccess(c, http.StatusOK, "remind cancellation processed successfully")
}

func respondCancelError(c *gin.Context, status int, errType, message string) {
	resp := &throttlev1.ErrorResponse{
		Error:   errType,
		Message: message,
	}

	respBytes, err := pjson.Marshal(resp)
	if err != nil {
		slog.Error("failed to marshal error response", slog.String("error", err.Error()))
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Data(status, "application/json", respBytes)
}

func respondCancelSuccess(c *gin.Context, status int, message string) {
	resp := &throttlev1.CancelRemindResponse{
		Success: true,
		Message: message,
	}

	respBytes, err := pjson.Marshal(resp)
	if err != nil {
		slog.Error("failed to marshal success response", slog.String("error", err.Error()))
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Data(status, "application/json", respBytes)
}
