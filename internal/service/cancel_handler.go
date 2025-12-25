package service

import (
	"context"
	"fmt"
	"log/slog"

	throttlev1 "github.com/KasumiMercury/primind-notification-throttling/internal/gen/throttle/v1"
)

func (s *ThrottleService) HandleRemindCancelled(ctx context.Context, req *throttlev1.CancelRemindRequest) error {
	logAttrs := []any{
		slog.String("task_id", req.TaskId),
		slog.String("user_id", req.UserId),
		slog.Int64("deleted_count", req.DeletedCount),
		slog.Int("remind_count", len(req.RemindIds)),
	}
	if req.CancelledAt != nil {
		logAttrs = append(logAttrs, slog.Time("cancelled_at", req.CancelledAt.AsTime()))
	}
	slog.Info("remind cancelled event received", logAttrs...)

	if s.taskQueue == nil {
		slog.Warn("task queue not configured, skipping task deletion",
			slog.String("task_id", req.TaskId),
		)
		return nil
	}

	if len(req.RemindIds) == 0 {
		slog.Debug("no remind_ids provided, skipping task deletion",
			slog.String("task_id", req.TaskId),
		)
		return nil
	}

	var errs []error
	for _, remindID := range req.RemindIds {
		if err := s.taskQueue.DeleteTask(ctx, remindID); err != nil {
			slog.Error("failed to delete queued notification task",
				slog.String("remind_id", remindID),
				slog.String("task_id", req.TaskId),
				slog.String("error", err.Error()),
			)
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to delete %d of %d tasks", len(errs), len(req.RemindIds))
	}

	slog.Info("successfully deleted queued notification tasks",
		slog.String("task_id", req.TaskId),
		slog.Int("deleted_count", len(req.RemindIds)),
	)

	return nil
}
