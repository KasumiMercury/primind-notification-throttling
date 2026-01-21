package plan

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
	"github.com/KasumiMercury/primind-notification-throttling/internal/infra/timemgmt"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service/lane"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service/sliding"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service/slot"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service/smoothing"
	"go.uber.org/mock/gomock"
)

func createTestService(
	remindClient timemgmt.RemindTimeRepository,
	throttleRepo domain.ThrottleRepository,
	requestCapPerMinute int,
) *Service {
	laneClassifier := lane.NewClassifier()
	slotCounter := slot.NewCounter(throttleRepo)
	slotCalculator := slot.NewCalculator(slotCounter, requestCapPerMinute)
	smoothingStrategy := smoothing.NewPassthroughStrategy()
	slideDiscovery := sliding.NewBestFitDiscovery()
	return NewService(remindClient, throttleRepo, laneClassifier, slotCalculator, slotCounter, smoothingStrategy, slideDiscovery, nil, requestCapPerMinute)
}

func TestPlanRemindsSuccess(t *testing.T) {
	tests := []struct {
		name        string
		reminds     []timemgmt.RemindResponse
		wantPlanned int
		wantSkipped int
		wantShifted int
	}{
		{
			name:        "empty reminds returns empty plan",
			reminds:     []timemgmt.RemindResponse{},
			wantPlanned: 0,
			wantSkipped: 0,
			wantShifted: 0,
		},
		{
			name: "single strict remind is planned at original time",
			reminds: []timemgmt.RemindResponse{
				{
					ID:               "remind-1",
					TaskID:           "task-1",
					TaskType:         "short",
					Time:             time.Now().Add(10 * time.Minute),
					SlideWindowWidth: 30,
					Throttled:        false,
				},
			},
			wantPlanned: 1,
			wantSkipped: 0,
			wantShifted: 0,
		},
		{
			name: "single loose remind is planned at original time when under cap",
			reminds: []timemgmt.RemindResponse{
				{
					ID:               "remind-1",
					TaskID:           "task-1",
					TaskType:         "near",
					Time:             time.Now().Add(10 * time.Minute),
					SlideWindowWidth: 300,
					Throttled:        false,
				},
			},
			wantPlanned: 1,
			wantSkipped: 0,
			wantShifted: 0,
		},
		{
			name: "throttled reminds are filtered out",
			reminds: []timemgmt.RemindResponse{
				{
					ID:               "remind-1",
					TaskID:           "task-1",
					TaskType:         "near",
					Time:             time.Now().Add(10 * time.Minute),
					SlideWindowWidth: 300,
					Throttled:        true,
				},
			},
			wantPlanned: 0,
			wantSkipped: 0,
			wantShifted: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRemindClient := timemgmt.NewMockRemindTimeRepository(ctrl)
			mockThrottleRepo := domain.NewMockThrottleRepository(ctrl)

			// Setup expectations
			mockRemindClient.EXPECT().
				GetRemindsByTimeRange(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(&timemgmt.RemindsResponse{
					Reminds: tt.reminds,
					Count:   len(tt.reminds),
				}, nil)

			// Refactored service always calls getCurrentCountsByMinute for entire window
			mockThrottleRepo.EXPECT().
				GetPacketCountForMinute(gomock.Any(), gomock.Any()).
				Return(0, nil).AnyTimes()
			mockThrottleRepo.EXPECT().
				GetPlannedPacketCount(gomock.Any(), gomock.Any()).
				Return(0, nil).AnyTimes()

			// Expect GetPlansInRange for merging with previous plans
			mockThrottleRepo.EXPECT().
				GetPlansInRange(gomock.Any(), gomock.Any(), gomock.Any()).
				Return([]*domain.Plan{}, nil).AnyTimes()

			// For each unthrottled remind, expect IsPacketCommitted call
			// Note: GetPlannedPacket is no longer called as we allow re-planning
			for _, r := range tt.reminds {
				if !r.Throttled {
					mockThrottleRepo.EXPECT().
						IsPacketCommitted(gomock.Any(), r.ID).
						Return(false, nil)
				}
			}

			// Expect SavePlan for any plans created
			if tt.wantPlanned > 0 {
				mockThrottleRepo.EXPECT().
					SavePlan(gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()
			}

			svc := createTestService(mockRemindClient, mockThrottleRepo, 60)

			ctx := context.Background()
			start := time.Now().Add(5 * time.Minute)
			end := time.Now().Add(30 * time.Minute)

			result, err := svc.PlanReminds(ctx, start, end, "")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.PlannedCount != tt.wantPlanned {
				t.Errorf("PlannedCount = %d, want %d", result.PlannedCount, tt.wantPlanned)
			}
			if result.SkippedCount != tt.wantSkipped {
				t.Errorf("SkippedCount = %d, want %d", result.SkippedCount, tt.wantSkipped)
			}
			if result.ShiftedCount != tt.wantShifted {
				t.Errorf("ShiftedCount = %d, want %d", result.ShiftedCount, tt.wantShifted)
			}
		})
	}
}

func TestPlanRemindsError(t *testing.T) {
	tests := []struct {
		name          string
		setupMocks    func(*timemgmt.MockRemindTimeRepository, *domain.MockThrottleRepository)
		expectedError string
	}{
		{
			name: "GetRemindsByTimeRange error",
			setupMocks: func(mockRemind *timemgmt.MockRemindTimeRepository, mockThrottle *domain.MockThrottleRepository) {
				mockRemind.EXPECT().
					GetRemindsByTimeRange(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("connection error"))
			},
			expectedError: "connection error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRemindClient := timemgmt.NewMockRemindTimeRepository(ctrl)
			mockThrottleRepo := domain.NewMockThrottleRepository(ctrl)

			tt.setupMocks(mockRemindClient, mockThrottleRepo)

			svc := createTestService(mockRemindClient, mockThrottleRepo, 60)

			ctx := context.Background()
			start := time.Now().Add(5 * time.Minute)
			end := time.Now().Add(30 * time.Minute)

			_, err := svc.PlanReminds(ctx, start, end, "")
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != tt.expectedError {
				t.Errorf("error = %v, want %v", err.Error(), tt.expectedError)
			}
		})
	}
}

func TestPlanReminds_SkipsAlreadyCommittedPackets(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRemindClient := timemgmt.NewMockRemindTimeRepository(ctrl)
	mockThrottleRepo := domain.NewMockThrottleRepository(ctrl)

	reminds := []timemgmt.RemindResponse{
		{
			ID:               "remind-committed",
			TaskID:           "task-1",
			TaskType:         "near",
			Time:             time.Now().Add(10 * time.Minute),
			SlideWindowWidth: 300,
			Throttled:        false,
		},
	}

	mockRemindClient.EXPECT().
		GetRemindsByTimeRange(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&timemgmt.RemindsResponse{
			Reminds: reminds,
			Count:   len(reminds),
		}, nil)

	// Refactored service always calls getCurrentCountsByMinute for entire window
	mockThrottleRepo.EXPECT().
		GetPacketCountForMinute(gomock.Any(), gomock.Any()).
		Return(0, nil).AnyTimes()
	mockThrottleRepo.EXPECT().
		GetPlannedPacketCount(gomock.Any(), gomock.Any()).
		Return(0, nil).AnyTimes()

	// Expect GetPlansInRange for merging with previous plans
	mockThrottleRepo.EXPECT().
		GetPlansInRange(gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*domain.Plan{}, nil).AnyTimes()

	mockThrottleRepo.EXPECT().
		IsPacketCommitted(gomock.Any(), "remind-committed").
		Return(true, nil)

	svc := createTestService(mockRemindClient, mockThrottleRepo, 60)

	ctx := context.Background()
	start := time.Now().Add(5 * time.Minute)
	end := time.Now().Add(30 * time.Minute)

	result, err := svc.PlanReminds(ctx, start, end, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.PlannedCount != 0 {
		t.Errorf("PlannedCount = %d, want 0", result.PlannedCount)
	}
	if result.SkippedCount != 1 {
		t.Errorf("SkippedCount = %d, want 1", result.SkippedCount)
	}
	if len(result.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1", len(result.Results))
	}
	if result.Results[0].SkipReason != "already committed" {
		t.Errorf("SkipReason = %q, want %q", result.Results[0].SkipReason, "already committed")
	}
}

func TestPlanReminds_ReplansAlreadyPlannedPackets(t *testing.T) {
	// Previously planned reminds should be re-planned (not skipped) to allow
	// smoothing recalculation and plan updates on each iteration.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRemindClient := timemgmt.NewMockRemindTimeRepository(ctrl)
	mockThrottleRepo := domain.NewMockThrottleRepository(ctrl)

	now := time.Now()
	reminds := []timemgmt.RemindResponse{
		{
			ID:               "remind-planned",
			TaskID:           "task-1",
			TaskType:         "near",
			Time:             now.Add(10 * time.Minute),
			SlideWindowWidth: 300,
			Throttled:        false,
		},
	}

	mockRemindClient.EXPECT().
		GetRemindsByTimeRange(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&timemgmt.RemindsResponse{
			Reminds: reminds,
			Count:   len(reminds),
		}, nil)

	// Refactored service always calls getCurrentCountsByMinute for entire window
	mockThrottleRepo.EXPECT().
		GetPacketCountForMinute(gomock.Any(), gomock.Any()).
		Return(0, nil).AnyTimes()
	mockThrottleRepo.EXPECT().
		GetPlannedPacketCount(gomock.Any(), gomock.Any()).
		Return(0, nil).AnyTimes()

	// Expect GetPlansInRange for merging with previous plans
	mockThrottleRepo.EXPECT().
		GetPlansInRange(gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*domain.Plan{}, nil).AnyTimes()

	// Only check committed status, not planned status
	mockThrottleRepo.EXPECT().
		IsPacketCommitted(gomock.Any(), "remind-planned").
		Return(false, nil)

	// Expect SavePlan since the remind will be (re-)planned
	mockThrottleRepo.EXPECT().
		SavePlan(gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	svc := createTestService(mockRemindClient, mockThrottleRepo, 60)

	ctx := context.Background()
	start := time.Now().Add(5 * time.Minute)
	end := time.Now().Add(30 * time.Minute)

	result, err := svc.PlanReminds(ctx, start, end, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Previously this test expected 0 planned and 1 skipped
	// Now we expect 1 planned and 0 skipped (re-planning is allowed)
	if result.PlannedCount != 1 {
		t.Errorf("PlannedCount = %d, want 1", result.PlannedCount)
	}
	if result.SkippedCount != 0 {
		t.Errorf("SkippedCount = %d, want 0", result.SkippedCount)
	}
}

func TestPlanReminds_StrictLaneUsesOriginalTimeWhenUnderCap(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRemindClient := timemgmt.NewMockRemindTimeRepository(ctrl)
	mockThrottleRepo := domain.NewMockThrottleRepository(ctrl)

	now := time.Now().Truncate(time.Minute)
	originalTime := now.Add(10 * time.Minute)

	reminds := []timemgmt.RemindResponse{
		{
			ID:               "remind-strict",
			TaskID:           "task-1",
			TaskType:         "near",
			Time:             originalTime,
			SlideWindowWidth: 30, // Narrow window -> Strict lane
			Throttled:        false,
		},
	}

	mockRemindClient.EXPECT().
		GetRemindsByTimeRange(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&timemgmt.RemindsResponse{
			Reminds: reminds,
			Count:   len(reminds),
		}, nil)

	// Refactored service always calls getCurrentCountsByMinute for entire window
	mockThrottleRepo.EXPECT().
		GetPacketCountForMinute(gomock.Any(), gomock.Any()).
		Return(0, nil).AnyTimes()
	mockThrottleRepo.EXPECT().
		GetPlannedPacketCount(gomock.Any(), gomock.Any()).
		Return(0, nil).AnyTimes()

	// Expect GetPlansInRange for merging with previous plans
	mockThrottleRepo.EXPECT().
		GetPlansInRange(gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*domain.Plan{}, nil).AnyTimes()

	mockThrottleRepo.EXPECT().
		IsPacketCommitted(gomock.Any(), "remind-strict").
		Return(false, nil)

	mockThrottleRepo.EXPECT().
		SavePlan(gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	svc := createTestService(mockRemindClient, mockThrottleRepo, 60)

	ctx := context.Background()
	start := now.Add(5 * time.Minute)
	end := now.Add(30 * time.Minute)

	result, err := svc.PlanReminds(ctx, start, end, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.PlannedCount != 1 {
		t.Errorf("PlannedCount = %d, want 1", result.PlannedCount)
	}
	if result.ShiftedCount != 0 {
		t.Errorf("ShiftedCount = %d, want 0", result.ShiftedCount)
	}
	if len(result.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1", len(result.Results))
	}
	if result.Results[0].WasShifted {
		t.Error("expected WasShifted = false for strict lane under cap")
	}
	if !result.Results[0].PlannedTime.Equal(originalTime) {
		t.Errorf("PlannedTime = %v, want %v (original time)", result.Results[0].PlannedTime, originalTime)
	}
}

func TestPlanReminds_StrictLaneNoShiftWhenSlideWindowUnder60Seconds(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRemindClient := timemgmt.NewMockRemindTimeRepository(ctrl)
	mockThrottleRepo := domain.NewMockThrottleRepository(ctrl)

	now := time.Now().Truncate(time.Minute)
	originalTime := now.Add(10 * time.Minute)

	reminds := []timemgmt.RemindResponse{
		{
			ID:               "remind-strict",
			TaskID:           "task-1",
			TaskType:         "near",
			Time:             originalTime,
			SlideWindowWidth: 30, // Narrow window (< 60s) -> No shifting possible
			Throttled:        false,
		},
	}

	mockRemindClient.EXPECT().
		GetRemindsByTimeRange(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&timemgmt.RemindsResponse{
			Reminds: reminds,
			Count:   len(reminds),
		}, nil)

	// Refactored service always calls getCurrentCountsByMinute for entire window
	mockThrottleRepo.EXPECT().
		GetPacketCountForMinute(gomock.Any(), gomock.Any()).
		Return(0, nil).AnyTimes()
	mockThrottleRepo.EXPECT().
		GetPlannedPacketCount(gomock.Any(), gomock.Any()).
		Return(0, nil).AnyTimes()

	// Expect GetPlansInRange for merging with previous plans
	mockThrottleRepo.EXPECT().
		GetPlansInRange(gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*domain.Plan{}, nil).AnyTimes()

	mockThrottleRepo.EXPECT().
		IsPacketCommitted(gomock.Any(), "remind-strict").
		Return(false, nil)

	mockThrottleRepo.EXPECT().
		SavePlan(gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	svc := createTestService(mockRemindClient, mockThrottleRepo, 60)

	ctx := context.Background()
	start := now.Add(5 * time.Minute)
	end := now.Add(30 * time.Minute)

	result, err := svc.PlanReminds(ctx, start, end, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.PlannedCount != 1 {
		t.Errorf("PlannedCount = %d, want 1", result.PlannedCount)
	}
	if result.ShiftedCount != 0 {
		t.Errorf("ShiftedCount = %d, want 0 (no shift when slideWindow < 60s)", result.ShiftedCount)
	}
	if len(result.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1", len(result.Results))
	}
	if result.Results[0].WasShifted {
		t.Error("expected WasShifted = false when slideWindow < 60s")
	}
	if !result.Results[0].PlannedTime.Equal(originalTime) {
		t.Errorf("PlannedTime = %v, want %v (original time, no shift)", result.Results[0].PlannedTime, originalTime)
	}
}

func TestGetStartMinuteTarget(t *testing.T) {
	svc := &Service{}
	ctx := context.Background()
	start := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	if got := svc.getStartMinuteTarget(ctx, start, map[string]int{}); got != nil {
		t.Fatalf("expected nil when no previous plans, got %v", *got)
	}

	otherMinute := start.Add(time.Minute)
	counts := map[string]int{
		domain.MinuteKey(otherMinute): 3,
	}
	if got := svc.getStartMinuteTarget(ctx, start, counts); got != nil {
		t.Fatalf("expected nil when start minute not present, got %v", *got)
	}

	counts[domain.MinuteKey(start)] = 5
	got := svc.getStartMinuteTarget(ctx, start, counts)
	if got == nil || *got != 5 {
		t.Fatalf("expected start value 5, got %v", got)
	}
}
