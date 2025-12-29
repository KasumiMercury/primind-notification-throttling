package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
	"go.uber.org/mock/gomock"
)

func TestSlotCounter_GetTotalCountForMinute_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := domain.NewMockThrottleRepository(ctrl)
	counter := NewSlotCounter(mockRepo)

	ctx := context.Background()
	minuteKey := "2025-01-01-12-00"

	mockRepo.EXPECT().GetPacketCountForMinute(ctx, minuteKey).Return(5, nil)
	mockRepo.EXPECT().GetPlannedPacketCount(ctx, minuteKey).Return(3, nil)

	count, err := counter.GetTotalCountForMinute(ctx, minuteKey)

	if err != nil {
		t.Errorf("GetTotalCountForMinute() error = %v, want nil", err)
	}
	if count != 8 {
		t.Errorf("GetTotalCountForMinute() count = %d, want 8", count)
	}
}

func TestSlotCounter_GetTotalCountForMinute_CommittedError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := domain.NewMockThrottleRepository(ctrl)
	counter := NewSlotCounter(mockRepo)

	ctx := context.Background()
	minuteKey := "2025-01-01-12-00"

	mockRepo.EXPECT().GetPacketCountForMinute(ctx, minuteKey).Return(0, errors.New("redis connection error"))
	mockRepo.EXPECT().GetPlannedPacketCount(ctx, minuteKey).Return(3, nil)

	count, err := counter.GetTotalCountForMinute(ctx, minuteKey)

	if err == nil {
		t.Error("GetTotalCountForMinute() error = nil, want error")
	}
	// Should return partial count (planned only)
	if count != 3 {
		t.Errorf("GetTotalCountForMinute() count = %d, want 3 (partial)", count)
	}
	// Error should contain context
	errStr := err.Error()
	if !strings.Contains(errStr, "committed count") || !strings.Contains(errStr, minuteKey) {
		t.Errorf("error message should contain context: %v", err)
	}
}

func TestSlotCounter_GetTotalCountForMinute_PlannedError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := domain.NewMockThrottleRepository(ctrl)
	counter := NewSlotCounter(mockRepo)

	ctx := context.Background()
	minuteKey := "2025-01-01-12-00"

	mockRepo.EXPECT().GetPacketCountForMinute(ctx, minuteKey).Return(5, nil)
	mockRepo.EXPECT().GetPlannedPacketCount(ctx, minuteKey).Return(0, errors.New("redis timeout"))

	count, err := counter.GetTotalCountForMinute(ctx, minuteKey)

	if err == nil {
		t.Error("GetTotalCountForMinute() error = nil, want error")
	}
	// Should return partial count (committed only)
	if count != 5 {
		t.Errorf("GetTotalCountForMinute() count = %d, want 5 (partial)", count)
	}
	// Error should contain context
	errStr := err.Error()
	if !strings.Contains(errStr, "planned count") || !strings.Contains(errStr, minuteKey) {
		t.Errorf("error message should contain context: %v", err)
	}
}

func TestSlotCounter_GetTotalCountForMinute_BothErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := domain.NewMockThrottleRepository(ctrl)
	counter := NewSlotCounter(mockRepo)

	ctx := context.Background()
	minuteKey := "2025-01-01-12-00"

	mockRepo.EXPECT().GetPacketCountForMinute(ctx, minuteKey).Return(0, errors.New("committed error"))
	mockRepo.EXPECT().GetPlannedPacketCount(ctx, minuteKey).Return(0, errors.New("planned error"))

	count, err := counter.GetTotalCountForMinute(ctx, minuteKey)

	if err == nil {
		t.Error("GetTotalCountForMinute() error = nil, want error")
	}
	// Should return 0 when both fail
	if count != 0 {
		t.Errorf("GetTotalCountForMinute() count = %d, want 0", count)
	}
	// Error should contain both error contexts
	errStr := err.Error()
	if !strings.Contains(errStr, "committed count") || !strings.Contains(errStr, "planned count") {
		t.Errorf("error message should contain both error contexts: %v", err)
	}
}

func TestSlotCounter_GetTotalCountForMinute_ZeroCounts(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := domain.NewMockThrottleRepository(ctrl)
	counter := NewSlotCounter(mockRepo)

	ctx := context.Background()
	minuteKey := "2025-01-01-12-00"

	mockRepo.EXPECT().GetPacketCountForMinute(ctx, minuteKey).Return(0, nil)
	mockRepo.EXPECT().GetPlannedPacketCount(ctx, minuteKey).Return(0, nil)

	count, err := counter.GetTotalCountForMinute(ctx, minuteKey)

	if err != nil {
		t.Errorf("GetTotalCountForMinute() error = %v, want nil", err)
	}
	if count != 0 {
		t.Errorf("GetTotalCountForMinute() count = %d, want 0", count)
	}
}
