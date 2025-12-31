package throttle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
	"github.com/KasumiMercury/primind-notification-throttling/internal/infra/taskqueue"
	"github.com/KasumiMercury/primind-notification-throttling/internal/infra/timemgmt"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service/lane"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service/slot"
	"go.uber.org/mock/gomock"
)

// createTestService creates a Service with the necessary shared components for testing.
func createTestService(
	remindClient timemgmt.RemindTimeRepository,
	tq taskqueue.TaskQueue,
	throttleRepo domain.ThrottleRepository,
	requestCapPerMinute int,
) *Service {
	laneClassifier := lane.NewClassifier()
	slotCounter := slot.NewCounter(throttleRepo)
	slotCalculator := slot.NewCalculator(slotCounter, requestCapPerMinute, nil)
	return NewService(remindClient, tq, throttleRepo, laneClassifier, slotCalculator, nil)
}

func TestExtractFCMTokens(t *testing.T) {
	svc := &Service{}

	tests := []struct {
		name     string
		devices  []timemgmt.DeviceResponse
		expected []string
	}{
		{
			name:     "empty devices",
			devices:  []timemgmt.DeviceResponse{},
			expected: []string{},
		},
		{
			name: "extract all tokens",
			devices: []timemgmt.DeviceResponse{
				{DeviceID: "d1", FCMToken: "token1"},
				{DeviceID: "d2", FCMToken: "token2"},
			},
			expected: []string{"token1", "token2"},
		},
		{
			name: "skip empty tokens",
			devices: []timemgmt.DeviceResponse{
				{DeviceID: "d1", FCMToken: "token1"},
				{DeviceID: "d2", FCMToken: ""},
				{DeviceID: "d3", FCMToken: "token3"},
			},
			expected: []string{"token1", "token3"},
		},
		{
			name: "all empty tokens",
			devices: []timemgmt.DeviceResponse{
				{DeviceID: "d1", FCMToken: ""},
				{DeviceID: "d2", FCMToken: ""},
			},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := svc.ExtractFCMTokens(tt.devices)

			if len(result) != len(tt.expected) {
				t.Errorf("got %d tokens, want %d", len(result), len(tt.expected))
				return
			}
			for i, token := range result {
				if token != tt.expected[i] {
					t.Errorf("token[%d]: got %q, want %q", i, token, tt.expected[i])
				}
			}
		})
	}
}

func TestProcessReminds_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRemindRepo := timemgmt.NewMockRemindTimeRepository(ctrl)
	mockTaskQueue := taskqueue.NewMockTaskQueue(ctrl)

	now := time.Now()
	remind := timemgmt.RemindResponse{
		ID:        "remind-1",
		Time:      now.Add(1 * time.Hour),
		UserID:    "user-1",
		TaskID:    "task-1",
		TaskType:  "near",
		Throttled: false,
		Devices: []timemgmt.DeviceResponse{
			{DeviceID: "device-1", FCMToken: "fcm-token-1"},
		},
	}

	mockRemindRepo.EXPECT().
		GetRemindsByTimeRange(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&timemgmt.RemindsResponse{
			Reminds: []timemgmt.RemindResponse{remind},
			Count:   1,
		}, nil)

	mockTaskQueue.EXPECT().
		RegisterNotification(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, task *taskqueue.NotificationTask) (*taskqueue.TaskResponse, error) {
			if task.RemindID != "remind-1" {
				t.Errorf("unexpected remind_id: got %q, want %q", task.RemindID, "remind-1")
			}
			if task.UserID != "user-1" {
				t.Errorf("unexpected user_id: got %q, want %q", task.UserID, "user-1")
			}
			if len(task.FCMTokens) != 1 || task.FCMTokens[0] != "fcm-token-1" {
				t.Errorf("unexpected fcm_tokens: got %v", task.FCMTokens)
			}
			return &taskqueue.TaskResponse{
				Name:         "task-name-1",
				ScheduleTime: now.Add(1 * time.Hour),
				CreateTime:   now,
			}, nil
		})

	mockRemindRepo.EXPECT().
		UpdateThrottled(gomock.Any(), "remind-1", true).
		Return(nil)

	svc := createTestService(mockRemindRepo, mockTaskQueue, nil, 60)
	ctx := context.Background()
	start := now
	end := now.Add(2 * time.Hour)

	resp, err := svc.ProcessReminds(ctx, start, end, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ProcessedCount != 1 {
		t.Errorf("ProcessedCount: got %d, want 1", resp.ProcessedCount)
	}
	if resp.SuccessCount != 1 {
		t.Errorf("SuccessCount: got %d, want 1", resp.SuccessCount)
	}
	if resp.FailedCount != 0 {
		t.Errorf("FailedCount: got %d, want 0", resp.FailedCount)
	}
}

func TestProcessReminds_GetRemindsByTimeRangeError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRemindRepo := timemgmt.NewMockRemindTimeRepository(ctrl)
	mockTaskQueue := taskqueue.NewMockTaskQueue(ctrl)

	expectedErr := errors.New("connection error")

	mockRemindRepo.EXPECT().
		GetRemindsByTimeRange(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, expectedErr)

	svc := createTestService(mockRemindRepo, mockTaskQueue, nil, 60)
	ctx := context.Background()
	now := time.Now()

	resp, err := svc.ProcessReminds(ctx, now, now.Add(1*time.Hour), "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
	if resp != nil {
		t.Errorf("expected nil response, got %+v", resp)
	}
}

func TestProcessReminds_RegisterNotificationError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRemindRepo := timemgmt.NewMockRemindTimeRepository(ctrl)
	mockTaskQueue := taskqueue.NewMockTaskQueue(ctrl)

	now := time.Now()
	remind := timemgmt.RemindResponse{
		ID:        "remind-1",
		Time:      now.Add(1 * time.Hour),
		UserID:    "user-1",
		TaskID:    "task-1",
		TaskType:  "near",
		Throttled: false,
		Devices: []timemgmt.DeviceResponse{
			{DeviceID: "device-1", FCMToken: "fcm-token-1"},
		},
	}

	mockRemindRepo.EXPECT().
		GetRemindsByTimeRange(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&timemgmt.RemindsResponse{
			Reminds: []timemgmt.RemindResponse{remind},
			Count:   1,
		}, nil)

	queueErr := errors.New("queue registration failed")
	mockTaskQueue.EXPECT().
		RegisterNotification(gomock.Any(), gomock.Any()).
		Return(nil, queueErr)

	svc := createTestService(mockRemindRepo, mockTaskQueue, nil, 60)
	ctx := context.Background()

	resp, err := svc.ProcessReminds(ctx, now, now.Add(1*time.Hour), "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if resp == nil {
		t.Fatal("expected response with failure info, got nil")
	}
	if resp.FailedCount != 1 {
		t.Errorf("FailedCount: got %d, want 1", resp.FailedCount)
	}
	if resp.SuccessCount != 0 {
		t.Errorf("SuccessCount: got %d, want 0", resp.SuccessCount)
	}
}

func TestProcessReminds_UpdateThrottledError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRemindRepo := timemgmt.NewMockRemindTimeRepository(ctrl)
	mockTaskQueue := taskqueue.NewMockTaskQueue(ctrl)

	now := time.Now()
	remind := timemgmt.RemindResponse{
		ID:        "remind-1",
		Time:      now.Add(1 * time.Hour),
		UserID:    "user-1",
		TaskID:    "task-1",
		TaskType:  "near",
		Throttled: false,
		Devices: []timemgmt.DeviceResponse{
			{DeviceID: "device-1", FCMToken: "fcm-token-1"},
		},
	}

	mockRemindRepo.EXPECT().
		GetRemindsByTimeRange(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&timemgmt.RemindsResponse{
			Reminds: []timemgmt.RemindResponse{remind},
			Count:   1,
		}, nil)

	mockTaskQueue.EXPECT().
		RegisterNotification(gomock.Any(), gomock.Any()).
		Return(&taskqueue.TaskResponse{Name: "task-1"}, nil)

	updateErr := errors.New("update failed")
	mockRemindRepo.EXPECT().
		UpdateThrottled(gomock.Any(), "remind-1", true).
		Return(updateErr).
		Times(3)

	svc := createTestService(mockRemindRepo, mockTaskQueue, nil, 60)
	ctx := context.Background()

	resp, err := svc.ProcessReminds(ctx, now, now.Add(1*time.Hour), "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if resp == nil {
		t.Fatal("expected response with failure info, got nil")
	}
	if resp.FailedCount != 1 {
		t.Errorf("FailedCount: got %d, want 1", resp.FailedCount)
	}
}

func TestProcessReminds_EmptyReminds(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRemindRepo := timemgmt.NewMockRemindTimeRepository(ctrl)
	mockTaskQueue := taskqueue.NewMockTaskQueue(ctrl)

	mockRemindRepo.EXPECT().
		GetRemindsByTimeRange(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&timemgmt.RemindsResponse{
			Reminds: []timemgmt.RemindResponse{},
			Count:   0,
		}, nil)

	svc := createTestService(mockRemindRepo, mockTaskQueue, nil, 60)
	ctx := context.Background()
	now := time.Now()

	resp, err := svc.ProcessReminds(ctx, now, now.Add(1*time.Hour), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ProcessedCount != 0 {
		t.Errorf("ProcessedCount: got %d, want 0", resp.ProcessedCount)
	}
	if resp.SuccessCount != 0 {
		t.Errorf("SuccessCount: got %d, want 0", resp.SuccessCount)
	}
}

func TestProcessReminds_NoFCMTokens(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRemindRepo := timemgmt.NewMockRemindTimeRepository(ctrl)
	mockTaskQueue := taskqueue.NewMockTaskQueue(ctrl)

	now := time.Now()
	remind := timemgmt.RemindResponse{
		ID:        "remind-1",
		Time:      now.Add(1 * time.Hour),
		UserID:    "user-1",
		TaskID:    "task-1",
		TaskType:  "near",
		Throttled: false,
		Devices:   []timemgmt.DeviceResponse{},
	}

	mockRemindRepo.EXPECT().
		GetRemindsByTimeRange(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&timemgmt.RemindsResponse{
			Reminds: []timemgmt.RemindResponse{remind},
			Count:   1,
		}, nil)

	// Note: No UpdateThrottled call expected - reminds without FCM tokens are now skipped

	svc := createTestService(mockRemindRepo, mockTaskQueue, nil, 60)
	ctx := context.Background()

	resp, err := svc.ProcessReminds(ctx, now, now.Add(1*time.Hour), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Reminds without FCM tokens are skipped, not processed as success
	if resp.SkippedCount != 1 {
		t.Errorf("SkippedCount: got %d, want 1", resp.SkippedCount)
	}
	if resp.SuccessCount != 0 {
		t.Errorf("SuccessCount: got %d, want 0", resp.SuccessCount)
	}
}

func TestProcessReminds_NilTaskQueue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRemindRepo := timemgmt.NewMockRemindTimeRepository(ctrl)

	now := time.Now()
	remind := timemgmt.RemindResponse{
		ID:        "remind-1",
		Time:      now.Add(1 * time.Hour),
		UserID:    "user-1",
		TaskID:    "task-1",
		TaskType:  "near",
		Throttled: false,
		Devices: []timemgmt.DeviceResponse{
			{DeviceID: "device-1", FCMToken: "fcm-token-1"},
		},
	}

	mockRemindRepo.EXPECT().
		GetRemindsByTimeRange(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&timemgmt.RemindsResponse{
			Reminds: []timemgmt.RemindResponse{remind},
			Count:   1,
		}, nil)

	mockRemindRepo.EXPECT().
		UpdateThrottled(gomock.Any(), "remind-1", true).
		Return(nil)

	svc := createTestService(mockRemindRepo, nil, nil, 60)
	ctx := context.Background()

	resp, err := svc.ProcessReminds(ctx, now, now.Add(1*time.Hour), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.SuccessCount != 1 {
		t.Errorf("SuccessCount: got %d, want 1", resp.SuccessCount)
	}
}

func TestProcessReminds_FilterThrottled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRemindRepo := timemgmt.NewMockRemindTimeRepository(ctrl)
	mockTaskQueue := taskqueue.NewMockTaskQueue(ctrl)

	now := time.Now()
	reminds := []timemgmt.RemindResponse{
		{
			ID:        "remind-1",
			Time:      now.Add(1 * time.Hour),
			UserID:    "user-1",
			TaskID:    "task-1",
			TaskType:  "near",
			Throttled: true,
			Devices: []timemgmt.DeviceResponse{
				{DeviceID: "device-1", FCMToken: "fcm-token-1"},
			},
		},
		{
			ID:        "remind-2",
			Time:      now.Add(1 * time.Hour),
			UserID:    "user-2",
			TaskID:    "task-2",
			TaskType:  "near",
			Throttled: false,
			Devices: []timemgmt.DeviceResponse{
				{DeviceID: "device-2", FCMToken: "fcm-token-2"},
			},
		},
	}

	mockRemindRepo.EXPECT().
		GetRemindsByTimeRange(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&timemgmt.RemindsResponse{
			Reminds: reminds,
			Count:   2,
		}, nil)

	mockTaskQueue.EXPECT().
		RegisterNotification(gomock.Any(), gomock.Any()).
		Return(&taskqueue.TaskResponse{Name: "task-2"}, nil)

	mockRemindRepo.EXPECT().
		UpdateThrottled(gomock.Any(), "remind-2", true).
		Return(nil)

	svc := createTestService(mockRemindRepo, mockTaskQueue, nil, 60)
	ctx := context.Background()

	resp, err := svc.ProcessReminds(ctx, now, now.Add(1*time.Hour), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ProcessedCount != 1 {
		t.Errorf("ProcessedCount: got %d, want 1 (throttled should be filtered)", resp.ProcessedCount)
	}
	if resp.SuccessCount != 1 {
		t.Errorf("SuccessCount: got %d, want 1", resp.SuccessCount)
	}
}

func TestProcessReminds_MultipleRemindsWithClassification(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRemindRepo := timemgmt.NewMockRemindTimeRepository(ctrl)
	mockTaskQueue := taskqueue.NewMockTaskQueue(ctrl)

	now := time.Now()
	reminds := []timemgmt.RemindResponse{
		{
			ID:        "remind-urgency",
			Time:      now.Add(1 * time.Hour),
			UserID:    "user-1",
			TaskID:    "task-1",
			TaskType:  "short",
			Throttled: false,
			Devices:   []timemgmt.DeviceResponse{{DeviceID: "d1", FCMToken: "token1"}},
		},
		{
			ID:        "remind-scheduled",
			Time:      now.Add(2 * time.Hour),
			UserID:    "user-2",
			TaskID:    "task-2",
			TaskType:  "scheduled",
			Throttled: false,
			Devices:   []timemgmt.DeviceResponse{{DeviceID: "d2", FCMToken: "token2"}},
		},
		{
			ID:        "remind-normal",
			Time:      now.Add(3 * time.Hour),
			UserID:    "user-3",
			TaskID:    "task-3",
			TaskType:  "near",
			Throttled: false,
			Devices:   []timemgmt.DeviceResponse{{DeviceID: "d3", FCMToken: "token3"}},
		},
		{
			ID:        "remind-low",
			Time:      now.Add(4 * time.Hour),
			UserID:    "user-4",
			TaskID:    "task-4",
			TaskType:  "relaxed",
			Throttled: false,
			Devices:   []timemgmt.DeviceResponse{{DeviceID: "d4", FCMToken: "token4"}},
		},
	}

	mockRemindRepo.EXPECT().
		GetRemindsByTimeRange(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&timemgmt.RemindsResponse{
			Reminds: reminds,
			Count:   4,
		}, nil)

	mockTaskQueue.EXPECT().
		RegisterNotification(gomock.Any(), gomock.Any()).
		Return(&taskqueue.TaskResponse{Name: "task"}, nil).
		Times(4)

	mockRemindRepo.EXPECT().
		UpdateThrottled(gomock.Any(), gomock.Any(), true).
		Return(nil).
		Times(4)

	svc := createTestService(mockRemindRepo, mockTaskQueue, nil, 60)
	ctx := context.Background()

	resp, err := svc.ProcessReminds(ctx, now, now.Add(5*time.Hour), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ProcessedCount != 4 {
		t.Errorf("ProcessedCount: got %d, want 4", resp.ProcessedCount)
	}
	if resp.SuccessCount != 4 {
		t.Errorf("SuccessCount: got %d, want 4", resp.SuccessCount)
	}
	if resp.FailedCount != 0 {
		t.Errorf("FailedCount: got %d, want 0", resp.FailedCount)
	}
}

func TestProcessReminds_RetrySuccessOnSecondAttempt(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRemindRepo := timemgmt.NewMockRemindTimeRepository(ctrl)
	mockTaskQueue := taskqueue.NewMockTaskQueue(ctrl)

	now := time.Now()
	remind := timemgmt.RemindResponse{
		ID:        "remind-1",
		Time:      now.Add(1 * time.Hour),
		UserID:    "user-1",
		TaskID:    "task-1",
		TaskType:  "near",
		Throttled: false,
		Devices: []timemgmt.DeviceResponse{
			{DeviceID: "device-1", FCMToken: "fcm-token-1"},
		},
	}

	mockRemindRepo.EXPECT().
		GetRemindsByTimeRange(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&timemgmt.RemindsResponse{
			Reminds: []timemgmt.RemindResponse{remind},
			Count:   1,
		}, nil)

	mockTaskQueue.EXPECT().
		RegisterNotification(gomock.Any(), gomock.Any()).
		Return(&taskqueue.TaskResponse{Name: "task-1"}, nil)

	updateErr := errors.New("temporary error")
	gomock.InOrder(
		mockRemindRepo.EXPECT().
			UpdateThrottled(gomock.Any(), "remind-1", true).
			Return(updateErr),
		mockRemindRepo.EXPECT().
			UpdateThrottled(gomock.Any(), "remind-1", true).
			Return(nil),
	)

	svc := createTestService(mockRemindRepo, mockTaskQueue, nil, 60)
	ctx := context.Background()

	resp, err := svc.ProcessReminds(ctx, now, now.Add(1*time.Hour), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.SuccessCount != 1 {
		t.Errorf("SuccessCount: got %d, want 1", resp.SuccessCount)
	}
}

func TestProcessReminds_ContextCancellation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRemindRepo := timemgmt.NewMockRemindTimeRepository(ctrl)
	mockTaskQueue := taskqueue.NewMockTaskQueue(ctrl)

	ctx, cancel := context.WithCancel(context.Background())

	mockRemindRepo.EXPECT().
		GetRemindsByTimeRange(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, start, end time.Time, runID string) (*timemgmt.RemindsResponse, error) {
			cancel()
			return nil, ctx.Err()
		})

	svc := createTestService(mockRemindRepo, mockTaskQueue, nil, 60)
	now := time.Now()

	resp, err := svc.ProcessReminds(ctx, now, now.Add(1*time.Hour), "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled error, got %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response, got %+v", resp)
	}
}
