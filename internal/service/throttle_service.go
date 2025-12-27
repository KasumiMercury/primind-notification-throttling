package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/client"
)

type TaskType string

const (
	TaskTypeUrgent    TaskType = "urgent"
	TaskTypeScheduled TaskType = "scheduled"
	TaskTypeNormal    TaskType = "normal"
	TaskTypeLow       TaskType = "low"
)

type ClassifiedReminds struct {
	Urgency   []client.RemindResponse
	Scheduled []client.RemindResponse
	Normal    []client.RemindResponse
	Low       []client.RemindResponse
}

type ProcessedRemind struct {
	Remind    client.RemindResponse
	FCMTokens []string
}

type ThrottleService struct {
	remindTimeClient client.RemindTimeRepository
	taskQueue        client.TaskQueue
}

func NewThrottleService(remindTimeClient client.RemindTimeRepository, taskQueue client.TaskQueue) *ThrottleService {
	return &ThrottleService{
		remindTimeClient: remindTimeClient,
		taskQueue:        taskQueue,
	}
}

func (s *ThrottleService) ProcessReminds(ctx context.Context, start, end time.Time) (*ThrottleResponse, error) {
	remindsResp, err := s.remindTimeClient.GetRemindsByTimeRange(ctx, start, end)
	if err != nil {
		slog.ErrorContext(ctx, "failed to fetch reminds",
			slog.String("error", err.Error()),
		)
		return nil, err
	}

	slog.DebugContext(ctx, "fetched reminds",
		slog.Int("total_count", remindsResp.Count),
	)

	unthrottledReminds := filterUnthrottled(remindsResp.Reminds)

	slog.DebugContext(ctx, "filtered unthrottled reminds",
		slog.Int("unthrottled_count", len(unthrottledReminds)),
	)

	classified := s.ClassifyByTaskType(unthrottledReminds)

	slog.InfoContext(ctx, "classified reminds",
		slog.Int("urgency", len(classified.Urgency)),
		slog.Int("scheduled", len(classified.Scheduled)),
		slog.Int("normal", len(classified.Normal)),
		slog.Int("low", len(classified.Low)),
	)

	allReminds := flattenClassifiedReminds(classified)

	results := make([]ThrottleResultItem, 0, len(allReminds))
	successCount := 0
	failedCount := 0

	for _, remind := range allReminds {
		fcmTokens := s.ExtractFCMTokens(remind.Devices)

		slog.DebugContext(ctx, "processing remind",
			slog.String("remind_id", remind.ID),
			slog.String("task_id", remind.TaskID),
			slog.String("task_type", remind.TaskType),
			slog.Int("fcm_token_count", len(fcmTokens)),
		)

		result := ThrottleResultItem{
			RemindID:  remind.ID,
			TaskID:    remind.TaskID,
			TaskType:  remind.TaskType,
			FCMTokens: fcmTokens,
			Success:   true,
		}

		if err := s.registerToQueue(ctx, remind, fcmTokens); err != nil {
			slog.ErrorContext(ctx, "failed to register to queue",
				slog.String("remind_id", remind.ID),
				slog.String("error", err.Error()),
			)
			result.Success = false
			result.Error = err.Error()
			failedCount++
			results = append(results, result)
			return &ThrottleResponse{
				ProcessedCount: len(results),
				SuccessCount:   successCount,
				FailedCount:    failedCount,
				Results:        results,
			}, fmt.Errorf("failed to register remind %s to queue: %w", remind.ID, err)
		}

		if err := s.updateThrottledWithRetry(ctx, remind.ID, true); err != nil {
			slog.ErrorContext(ctx, "failed to update throttled flag",
				slog.String("remind_id", remind.ID),
				slog.String("error", err.Error()),
			)
			result.Success = false
			result.Error = err.Error()
			failedCount++
			results = append(results, result)
			return &ThrottleResponse{
				ProcessedCount: len(results),
				SuccessCount:   successCount,
				FailedCount:    failedCount,
				Results:        results,
			}, fmt.Errorf("failed to update throttled flag for remind %s: %w", remind.ID, err)
		}

		successCount++
		results = append(results, result)
	}

	return &ThrottleResponse{
		ProcessedCount: len(allReminds),
		SuccessCount:   successCount,
		FailedCount:    failedCount,
		Results:        results,
	}, nil
}

func (s *ThrottleService) updateThrottledWithRetry(ctx context.Context, remindID string, throttled bool) error {
	const maxRetries = 3

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * 100 * time.Millisecond
			slog.DebugContext(ctx, "retrying throttled flag update",
				slog.String("remind_id", remindID),
				slog.Bool("throttled", throttled),
				slog.Int("attempt", attempt+1),
				slog.Duration("backoff", backoff),
			)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		if err := s.remindTimeClient.UpdateThrottled(ctx, remindID, throttled); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}

	return fmt.Errorf("failed to update throttled flag after %d retries: %w", maxRetries, lastErr)
}

func (s *ThrottleService) ClassifyByTaskType(reminds []client.RemindResponse) *ClassifiedReminds {
	classified := &ClassifiedReminds{
		Urgency:   make([]client.RemindResponse, 0),
		Scheduled: make([]client.RemindResponse, 0),
		Normal:    make([]client.RemindResponse, 0),
		Low:       make([]client.RemindResponse, 0),
	}

	for _, remind := range reminds {
		switch TaskType(remind.TaskType) {
		case TaskTypeUrgent:
			classified.Urgency = append(classified.Urgency, remind)
		case TaskTypeScheduled:
			classified.Scheduled = append(classified.Scheduled, remind)
		case TaskTypeNormal:
			classified.Normal = append(classified.Normal, remind)
		case TaskTypeLow:
			classified.Low = append(classified.Low, remind)
		default:
			classified.Normal = append(classified.Normal, remind)
		}
	}

	return classified
}

func (s *ThrottleService) ExtractFCMTokens(devices []client.DeviceResponse) []string {
	tokens := make([]string, 0, len(devices))
	for _, device := range devices {
		if device.FCMToken != "" {
			tokens = append(tokens, device.FCMToken)
		}
	}
	return tokens
}

func (s *ThrottleService) registerToQueue(ctx context.Context, remind client.RemindResponse, fcmTokens []string) error {
	if s.taskQueue == nil {
		slog.WarnContext(ctx, "task queue not configured, skipping queue registration",
			slog.String("remind_id", remind.ID),
		)
		return nil
	}

	if len(fcmTokens) == 0 {
		slog.DebugContext(ctx, "no FCM tokens, skipping queue registration",
			slog.String("remind_id", remind.ID),
		)
		return nil
	}

	task := &client.NotificationTask{
		RemindID:   remind.ID,
		UserID:     remind.UserID,
		TaskID:     remind.TaskID,
		TaskType:   remind.TaskType,
		FCMTokens:  fcmTokens,
		ScheduleAt: remind.Time,
	}

	resp, err := s.taskQueue.RegisterNotification(ctx, task)
	if err != nil {
		return err
	}

	slog.DebugContext(ctx, "notification registered to queue",
		slog.String("remind_id", remind.ID),
		slog.String("task_name", resp.Name),
		slog.Time("schedule_time", resp.ScheduleTime),
	)

	return nil
}

func filterUnthrottled(reminds []client.RemindResponse) []client.RemindResponse {
	filtered := make([]client.RemindResponse, 0, len(reminds))
	for _, remind := range reminds {
		if !remind.Throttled {
			filtered = append(filtered, remind)
		}
	}
	return filtered
}

func flattenClassifiedReminds(classified *ClassifiedReminds) []client.RemindResponse {
	total := len(classified.Urgency) + len(classified.Scheduled) + len(classified.Normal) + len(classified.Low)
	result := make([]client.RemindResponse, 0, total)

	result = append(result, classified.Urgency...)
	result = append(result, classified.Scheduled...)
	result = append(result, classified.Normal...)
	result = append(result, classified.Low...)

	return result
}
