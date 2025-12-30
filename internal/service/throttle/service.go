package throttle

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
	"github.com/KasumiMercury/primind-notification-throttling/internal/infra/taskqueue"
	"github.com/KasumiMercury/primind-notification-throttling/internal/infra/timemgmt"
	"github.com/KasumiMercury/primind-notification-throttling/internal/observability/metrics"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service/lane"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service/slot"
)

type ProcessedRemind struct {
	Remind    timemgmt.RemindResponse
	FCMTokens []string
}

type Service struct {
	remindTimeClient timemgmt.RemindTimeRepository
	taskQueue        taskqueue.TaskQueue
	throttleRepo     domain.ThrottleRepository
	laneClassifier   *lane.Classifier
	slotCalculator   *slot.Calculator
	throttleMetrics  *metrics.ThrottleMetrics
}

func NewService(
	remindTimeClient timemgmt.RemindTimeRepository,
	taskQueue taskqueue.TaskQueue,
	throttleRepo domain.ThrottleRepository,
	laneClassifier *lane.Classifier,
	slotCalculator *slot.Calculator,
	throttleMetrics *metrics.ThrottleMetrics,
) *Service {
	return &Service{
		remindTimeClient: remindTimeClient,
		taskQueue:        taskQueue,
		throttleRepo:     throttleRepo,
		laneClassifier:   laneClassifier,
		slotCalculator:   slotCalculator,
		throttleMetrics:  throttleMetrics,
	}
}

func (s *Service) ProcessReminds(ctx context.Context, start, end time.Time) (*Response, error) {
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

	slog.InfoContext(ctx, "filtered unthrottled reminds",
		slog.Int("unthrottled_count", len(unthrottledReminds)),
	)

	results := make([]ResultItem, 0, len(unthrottledReminds))
	successCount := 0
	failedCount := 0
	skippedCount := 0
	shiftedCount := 0

	for _, remind := range unthrottledReminds {
		fcmTokens := s.ExtractFCMTokens(remind.Devices)
		classifiedLane := s.laneClassifier.Classify(remind)

		// Record lane distribution
		if s.throttleMetrics != nil {
			s.throttleMetrics.RecordLaneDistribution(ctx, classifiedLane.String())
		}

		slog.DebugContext(ctx, "processing remind",
			slog.String("remind_id", remind.ID),
			slog.String("task_id", remind.TaskID),
			slog.String("task_type", remind.TaskType),
			slog.String("lane", classifiedLane.String()),
			slog.Int("fcm_token_count", len(fcmTokens)),
			slog.Int("slide_window_width", int(remind.SlideWindowWidth)),
		)

		result := ResultItem{
			RemindID:      remind.ID,
			TaskID:        remind.TaskID,
			TaskType:      remind.TaskType,
			FCMTokens:     fcmTokens,
			Lane:          classifiedLane,
			OriginalTime:  remind.Time,
			ScheduledTime: remind.Time,
			Success:       true,
		}

		// Check idempotency: skip if already committed
		if s.throttleRepo != nil {
			committed, err := s.throttleRepo.IsPacketCommitted(ctx, remind.ID)
			if err != nil {
				slog.WarnContext(ctx, "failed to check packet committed status",
					slog.String("remind_id", remind.ID),
					slog.String("error", err.Error()),
				)
				// Continue processing - treat as not committed
			} else if committed {
				slog.DebugContext(ctx, "skipping already committed remind",
					slog.String("remind_id", remind.ID),
				)
				result.Skipped = true
				result.SkipReason = "already committed"
				skippedCount++
				if s.throttleMetrics != nil {
					s.throttleMetrics.RecordPacketProcessed(ctx, "commit", classifiedLane.String(), "skipped")
				}
				results = append(results, result)
				continue
			}
		}

		// Skip if no FCM tokens
		if len(fcmTokens) == 0 {
			slog.DebugContext(ctx, "skipping remind without FCM tokens",
				slog.String("remind_id", remind.ID),
			)
			result.Skipped = true
			result.SkipReason = "no FCM tokens"
			skippedCount++
			if s.throttleMetrics != nil {
				s.throttleMetrics.RecordPacketProcessed(ctx, "commit", classifiedLane.String(), "skipped")
			}
			results = append(results, result)
			continue
		}

		// Calculate scheduled time
		scheduledTime, wasShifted, err := s.calculateScheduledTime(ctx, remind, classifiedLane)
		if err != nil {
			slog.ErrorContext(ctx, "failed to calculate scheduled time",
				slog.String("remind_id", remind.ID),
				slog.String("error", err.Error()),
			)
			// Use original time as fallback
			scheduledTime = remind.Time
			wasShifted = false
		}

		result.ScheduledTime = scheduledTime
		result.WasShifted = wasShifted

		if wasShifted {
			shiftedCount++
			if s.throttleMetrics != nil {
				s.throttleMetrics.RecordPacketShifted(ctx, "commit", classifiedLane.String())
			}
			slog.InfoContext(ctx, "reminder shifted",
				slog.String("remind_id", remind.ID),
				slog.Time("original_time", remind.Time),
				slog.Time("scheduled_time", scheduledTime),
				slog.Duration("shift", scheduledTime.Sub(remind.Time)),
			)
		}

		// Register to task queue with the scheduled time
		if err := s.registerToQueueWithTime(ctx, remind, fcmTokens, scheduledTime); err != nil {
			slog.ErrorContext(ctx, "failed to register to queue",
				slog.String("remind_id", remind.ID),
				slog.String("error", err.Error()),
			)
			result.Success = false
			result.Error = err.Error()
			failedCount++
			if s.throttleMetrics != nil {
				s.throttleMetrics.RecordPacketProcessed(ctx, "commit", classifiedLane.String(), "failed")
			}
			results = append(results, result)
			return &Response{
				ProcessedCount: len(results),
				SuccessCount:   successCount,
				FailedCount:    failedCount,
				SkippedCount:   skippedCount,
				ShiftedCount:   shiftedCount,
				Results:        results,
			}, fmt.Errorf("failed to register remind %s to queue: %w", remind.ID, err)
		}

		// Commit packet to Redis
		if s.throttleRepo != nil {
			packet := domain.NewPacket(remind.ID, remind.Time, scheduledTime, classifiedLane)
			if err := s.throttleRepo.SaveCommittedPacket(ctx, packet); err != nil {
				slog.WarnContext(ctx, "failed to save committed packet",
					slog.String("remind_id", remind.ID),
					slog.String("error", err.Error()),
				)
				// Continue
			}

			// Increment packet count for the scheduled minute
			minuteKey := domain.MinuteKey(scheduledTime)
			if err := s.throttleRepo.IncrementPacketCount(ctx, minuteKey, 1); err != nil {
				slog.WarnContext(ctx, "failed to increment packet count",
					slog.String("remind_id", remind.ID),
					slog.String("minute_key", minuteKey),
					slog.String("error", err.Error()),
				)
				// Continue
			}
		}

		// Update throttled flag in time-mgmt
		if err := s.updateThrottledWithRetry(ctx, remind.ID, true); err != nil {
			slog.ErrorContext(ctx, "failed to update throttled flag",
				slog.String("remind_id", remind.ID),
				slog.String("error", err.Error()),
			)
			result.Success = false
			result.Error = err.Error()
			failedCount++
			if s.throttleMetrics != nil {
				s.throttleMetrics.RecordPacketProcessed(ctx, "commit", classifiedLane.String(), "failed")
			}
			results = append(results, result)
			return &Response{
				ProcessedCount: len(results),
				SuccessCount:   successCount,
				FailedCount:    failedCount,
				SkippedCount:   skippedCount,
				ShiftedCount:   shiftedCount,
				Results:        results,
			}, fmt.Errorf("failed to update throttled flag for remind %s: %w", remind.ID, err)
		}

		successCount++
		if s.throttleMetrics != nil {
			s.throttleMetrics.RecordPacketProcessed(ctx, "commit", classifiedLane.String(), "success")
		}
		results = append(results, result)
	}

	return &Response{
		ProcessedCount: len(unthrottledReminds),
		SuccessCount:   successCount,
		FailedCount:    failedCount,
		SkippedCount:   skippedCount,
		ShiftedCount:   shiftedCount,
		Results:        results,
	}, nil
}

func (s *Service) calculateScheduledTime(
	ctx context.Context,
	remind timemgmt.RemindResponse,
	classifiedLane domain.Lane,
) (time.Time, bool, error) {
	originalTime := remind.Time

	if s.throttleRepo == nil {
		return originalTime, false, nil
	}

	// Check if there's a planned time from the planning phase
	plannedPacket, err := s.throttleRepo.GetPlannedPacket(ctx, remind.ID)
	if err == nil && plannedPacket != nil {
		// Use the pre-calculated planned time
		wasShifted := plannedPacket.WasShifted()
		slog.DebugContext(ctx, "using planned time from planning phase",
			slog.String("remind_id", remind.ID),
			slog.Time("original_time", originalTime),
			slog.Time("planned_time", plannedPacket.PlannedTime),
			slog.Bool("was_shifted", wasShifted),
		)
		return plannedPacket.PlannedTime, wasShifted, nil
	}

	// No plan found
	scheduledTime, wasShifted := s.slotCalculator.FindSlot(
		ctx,
		originalTime,
		int(remind.SlideWindowWidth),
		classifiedLane,
	)

	return scheduledTime, wasShifted, nil
}

func (s *Service) updateThrottledWithRetry(ctx context.Context, remindID string, throttled bool) error {
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

func (s *Service) ExtractFCMTokens(devices []timemgmt.DeviceResponse) []string {
	tokens := make([]string, 0, len(devices))
	for _, device := range devices {
		if device.FCMToken != "" {
			tokens = append(tokens, device.FCMToken)
		}
	}
	return tokens
}

// registerToQueueWithTime registers a remind to the task queue with a specific schedule time.
func (s *Service) registerToQueueWithTime(
	ctx context.Context,
	remind timemgmt.RemindResponse,
	fcmTokens []string,
	scheduleAt time.Time,
) error {
	if s.taskQueue == nil {
		slog.WarnContext(ctx, "task queue not configured, skipping queue registration",
			slog.String("remind_id", remind.ID),
		)
		return nil
	}

	task := &taskqueue.NotificationTask{
		RemindID:   remind.ID,
		UserID:     remind.UserID,
		TaskID:     remind.TaskID,
		TaskType:   remind.TaskType,
		FCMTokens:  fcmTokens,
		ScheduleAt: scheduleAt,
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

func filterUnthrottled(reminds []timemgmt.RemindResponse) []timemgmt.RemindResponse {
	filtered := make([]timemgmt.RemindResponse, 0, len(reminds))
	for _, remind := range reminds {
		if !remind.Throttled {
			filtered = append(filtered, remind)
		}
	}
	return filtered
}
