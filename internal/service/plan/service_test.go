package plan

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
	"github.com/KasumiMercury/primind-notification-throttling/internal/infra/timemgmt"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service/lane"
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
	slotCalculator := slot.NewCalculator(slotCounter, requestCapPerMinute, nil)
	smoothingStrategy := smoothing.NewPassthroughStrategy()
	return NewService(remindClient, throttleRepo, laneClassifier, slotCalculator, smoothingStrategy, nil)
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

			// For each unthrottled remind, expect these repo calls
			for _, r := range tt.reminds {
				if !r.Throttled {
					mockThrottleRepo.EXPECT().
						IsPacketCommitted(gomock.Any(), r.ID).
						Return(false, nil)
					mockThrottleRepo.EXPECT().
						GetPlannedPacket(gomock.Any(), r.ID).
						Return(nil, domain.ErrPlannedPacketNotFound)
					mockThrottleRepo.EXPECT().
						GetPacketCountForMinute(gomock.Any(), gomock.Any()).
						Return(0, nil).AnyTimes()
					mockThrottleRepo.EXPECT().
						GetPlannedPacketCount(gomock.Any(), gomock.Any()).
						Return(0, nil).AnyTimes()
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

func TestPlanReminds_SkipsAlreadyPlannedPackets(t *testing.T) {
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

	mockThrottleRepo.EXPECT().
		IsPacketCommitted(gomock.Any(), "remind-planned").
		Return(false, nil)

	mockThrottleRepo.EXPECT().
		GetPlannedPacket(gomock.Any(), "remind-planned").
		Return(&domain.PlannedPacket{
			RemindID:     "remind-planned",
			OriginalTime: now.Add(10 * time.Minute),
			PlannedTime:  now.Add(10 * time.Minute),
			Lane:         domain.LaneLoose,
		}, nil)

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
	if result.Results[0].SkipReason != "already planned" {
		t.Errorf("SkipReason = %q, want %q", result.Results[0].SkipReason, "already planned")
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

	mockThrottleRepo.EXPECT().
		IsPacketCommitted(gomock.Any(), "remind-strict").
		Return(false, nil)

	mockThrottleRepo.EXPECT().
		GetPlannedPacket(gomock.Any(), "remind-strict").
		Return(nil, domain.ErrPlannedPacketNotFound)

	// Strict lane now checks cap - return under cap (30 < 60)
	originalMinuteKey := domain.MinuteKey(originalTime)
	mockThrottleRepo.EXPECT().
		GetPacketCountForMinute(gomock.Any(), originalMinuteKey).
		Return(30, nil)
	mockThrottleRepo.EXPECT().
		GetPlannedPacketCount(gomock.Any(), originalMinuteKey).
		Return(0, nil)

	mockThrottleRepo.EXPECT().
		SavePlan(gomock.Any(), gomock.Any()).
		Return(nil)

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

	mockThrottleRepo.EXPECT().
		IsPacketCommitted(gomock.Any(), "remind-strict").
		Return(false, nil)

	mockThrottleRepo.EXPECT().
		GetPlannedPacket(gomock.Any(), "remind-strict").
		Return(nil, domain.ErrPlannedPacketNotFound)

	// Original minute is at cap (60), but with slideWindow < 60s, no shift occurs
	originalMinuteKey := domain.MinuteKey(originalTime)
	mockThrottleRepo.EXPECT().
		GetPacketCountForMinute(gomock.Any(), originalMinuteKey).
		Return(60, nil)
	mockThrottleRepo.EXPECT().
		GetPlannedPacketCount(gomock.Any(), originalMinuteKey).
		Return(0, nil)

	// No candidate minutes are checked because slideWindow < 60s

	mockThrottleRepo.EXPECT().
		SavePlan(gomock.Any(), gomock.Any()).
		Return(nil)

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
