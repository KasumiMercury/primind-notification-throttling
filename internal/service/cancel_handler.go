package service

import (
	"context"
	"log/slog"

	throttlev1 "github.com/KasumiMercury/primind-notification-throttling/internal/gen/throttle/v1"
)

func (s *ThrottleService) HandleRemindCancelled(ctx context.Context, req *throttlev1.CancelRemindRequest) error {
	logAttrs := []any{
		slog.String("task_id", req.TaskId),
		slog.String("user_id", req.UserId),
		slog.Int64("deleted_count", req.DeletedCount),
	}
	if req.CancelledAt != nil {
		logAttrs = append(logAttrs, slog.Time("cancelled_at", req.CancelledAt.AsTime()))
	}
	slog.Info("remind cancelled event received", logAttrs...)

	// TODO: Implement cancellation logic for queued notifications.

	return nil
}
