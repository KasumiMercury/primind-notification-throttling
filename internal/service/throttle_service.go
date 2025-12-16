package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/client"
)

type TaskType string

const (
	TaskTypeUrgency   TaskType = "Urgency"
	TaskTypeScheduled TaskType = "Scheduled"
	TaskTypeNormal    TaskType = "Normal"
	TaskTypeLow       TaskType = "Low"
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
	remindTimeClient *client.RemindTimeClient
}

func NewThrottleService(remindTimeClient *client.RemindTimeClient) *ThrottleService {
	return &ThrottleService{
		remindTimeClient: remindTimeClient,
	}
}

func (s *ThrottleService) ProcessReminds(ctx context.Context, start, end time.Time) (*ThrottleResponse, error) {
	remindsResp, err := s.remindTimeClient.GetRemindsByTimeRange(ctx, start, end)
	if err != nil {
		slog.Error("failed to fetch reminds",
			slog.String("error", err.Error()),
		)
		return nil, err
	}

	slog.Debug("fetched reminds",
		slog.Int("total_count", remindsResp.Count),
	)

	unthrottledReminds := filterUnthrottled(remindsResp.Reminds)

	slog.Debug("filtered unthrottled reminds",
		slog.Int("unthrottled_count", len(unthrottledReminds)),
	)

	classified := s.ClassifyByTaskType(unthrottledReminds)

	slog.Info("classified reminds",
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

		slog.Debug("processing remind",
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

		if err := s.remindTimeClient.UpdateThrottled(ctx, remind.ID, true); err != nil {
			slog.Error("failed to update throttled flag",
				slog.String("remind_id", remind.ID),
				slog.String("error", err.Error()),
			)
			result.Success = false
			result.Error = err.Error()
			failedCount++
		} else {
			successCount++
		}

		s.registerToQueue(ctx, remind, fcmTokens)

		results = append(results, result)
	}

	return &ThrottleResponse{
		ProcessedCount: len(allReminds),
		SuccessCount:   successCount,
		FailedCount:    failedCount,
		Results:        results,
	}, nil
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
		case TaskTypeUrgency:
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
	// TODO: Implement Job Queue registration
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
