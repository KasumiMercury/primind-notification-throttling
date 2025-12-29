package repository

import (
	"context"
	"testing"
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
	"github.com/KasumiMercury/primind-notification-throttling/internal/testutil"
)

func TestGetPacketCountForMinuteSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	client, cleanup := testutil.SetupRedisContainer(ctx, t)
	defer cleanup()

	repo := NewThrottleRepository(client)

	tests := []struct {
		name      string
		minuteKey string
		setup     func(t *testing.T)
		expected  int
	}{
		{
			name:      "empty key returns zero",
			minuteKey: "2024-01-15-14-30",
			setup:     func(t *testing.T) {},
			expected:  0,
		},
		{
			name:      "existing key returns correct count",
			minuteKey: "2024-01-15-14-31",
			setup: func(t *testing.T) {
				err := client.Set(ctx, "throttle:packets:2024-01-15-14-31", 42, 0).Err()
				if err != nil {
					t.Fatalf("failed to set up test data: %v", err)
				}
			},
			expected: 42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup(t)

			count, err := repo.GetPacketCountForMinute(ctx, tt.minuteKey)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if count != tt.expected {
				t.Errorf("expected count %d, got %d", tt.expected, count)
			}
		})
	}
}

func TestIncrementPacketCountSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	client, cleanup := testutil.SetupRedisContainer(ctx, t)
	defer cleanup()

	repo := NewThrottleRepository(client)

	tests := []struct {
		name      string
		minuteKey string
		delta     int
		setup     func(t *testing.T)
		expected  int
	}{
		{
			name:      "increment new key",
			minuteKey: "2024-01-15-15-00",
			delta:     1,
			setup:     func(t *testing.T) {},
			expected:  1,
		},
		{
			name:      "increment existing key",
			minuteKey: "2024-01-15-15-01",
			delta:     5,
			setup: func(t *testing.T) {
				err := client.Set(ctx, "throttle:packets:2024-01-15-15-01", 10, 0).Err()
				if err != nil {
					t.Fatalf("failed to set up test data: %v", err)
				}
			},
			expected: 15,
		},
		{
			name:      "increment by multiple",
			minuteKey: "2024-01-15-15-02",
			delta:     10,
			setup:     func(t *testing.T) {},
			expected:  10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup(t)

			err := repo.IncrementPacketCount(ctx, tt.minuteKey, tt.delta)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			count, err := repo.GetPacketCountForMinute(ctx, tt.minuteKey)
			if err != nil {
				t.Fatalf("failed to get packet count: %v", err)
			}
			if count != tt.expected {
				t.Errorf("expected count %d, got %d", tt.expected, count)
			}

			// Verify TTL is set
			ttl, err := client.TTL(ctx, "throttle:packets:"+tt.minuteKey).Result()
			if err != nil {
				t.Fatalf("failed to get TTL: %v", err)
			}
			if ttl <= 0 || ttl > time.Hour {
				t.Errorf("expected TTL around 1 hour, got %v", ttl)
			}
		})
	}
}

func TestSaveCommittedPacketSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	client, cleanup := testutil.SetupRedisContainer(ctx, t)
	defer cleanup()

	repo := NewThrottleRepository(client)

	now := time.Now().UTC().Truncate(time.Millisecond)

	tests := []struct {
		name   string
		packet *domain.Packet
	}{
		{
			name: "save strict lane packet",
			packet: &domain.Packet{
				RemindID:      "remind-001",
				OriginalTime:  now,
				ScheduledTime: now,
				Lane:          domain.LaneStrict,
				CommittedAt:   now,
			},
		},
		{
			name: "save loose lane packet with shift",
			packet: &domain.Packet{
				RemindID:      "remind-002",
				OriginalTime:  now,
				ScheduledTime: now.Add(2 * time.Minute),
				Lane:          domain.LaneLoose,
				CommittedAt:   now,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.SaveCommittedPacket(ctx, tt.packet)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify we can retrieve it
			retrieved, err := repo.GetCommittedPacket(ctx, tt.packet.RemindID)
			if err != nil {
				t.Fatalf("failed to get committed packet: %v", err)
			}

			if retrieved.RemindID != tt.packet.RemindID {
				t.Errorf("expected RemindID %s, got %s", tt.packet.RemindID, retrieved.RemindID)
			}
			if !retrieved.OriginalTime.Equal(tt.packet.OriginalTime) {
				t.Errorf("expected OriginalTime %v, got %v", tt.packet.OriginalTime, retrieved.OriginalTime)
			}
			if !retrieved.ScheduledTime.Equal(tt.packet.ScheduledTime) {
				t.Errorf("expected ScheduledTime %v, got %v", tt.packet.ScheduledTime, retrieved.ScheduledTime)
			}
			if retrieved.Lane != tt.packet.Lane {
				t.Errorf("expected Lane %s, got %s", tt.packet.Lane, retrieved.Lane)
			}
		})
	}
}

func TestSaveCommittedPacketError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	client, cleanup := testutil.SetupRedisContainer(ctx, t)
	defer cleanup()

	repo := NewThrottleRepository(client)

	tests := []struct {
		name        string
		packet      *domain.Packet
		expectedErr error
	}{
		{
			name:        "nil packet",
			packet:      nil,
			expectedErr: ErrInvalidPacketData,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.SaveCommittedPacket(ctx, tt.packet)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err != tt.expectedErr {
				t.Errorf("expected error %v, got %v", tt.expectedErr, err)
			}
		})
	}
}

func TestIsPacketCommittedSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	client, cleanup := testutil.SetupRedisContainer(ctx, t)
	defer cleanup()

	repo := NewThrottleRepository(client)

	now := time.Now().UTC()

	// Save a packet first
	packet := &domain.Packet{
		RemindID:      "remind-exists",
		OriginalTime:  now,
		ScheduledTime: now,
		Lane:          domain.LaneStrict,
		CommittedAt:   now,
	}
	if err := repo.SaveCommittedPacket(ctx, packet); err != nil {
		t.Fatalf("failed to save packet: %v", err)
	}

	tests := []struct {
		name     string
		remindID string
		expected bool
	}{
		{
			name:     "existing packet returns true",
			remindID: "remind-exists",
			expected: true,
		},
		{
			name:     "non-existing packet returns false",
			remindID: "remind-not-exists",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			committed, err := repo.IsPacketCommitted(ctx, tt.remindID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if committed != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, committed)
			}
		})
	}
}

func TestGetCommittedPacketSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	client, cleanup := testutil.SetupRedisContainer(ctx, t)
	defer cleanup()

	repo := NewThrottleRepository(client)

	now := time.Now().UTC().Truncate(time.Millisecond)

	// Save a packet first
	packet := &domain.Packet{
		RemindID:      "remind-get-test",
		OriginalTime:  now,
		ScheduledTime: now.Add(time.Minute),
		Lane:          domain.LaneLoose,
		CommittedAt:   now,
	}
	if err := repo.SaveCommittedPacket(ctx, packet); err != nil {
		t.Fatalf("failed to save packet: %v", err)
	}

	tests := []struct {
		name     string
		remindID string
	}{
		{
			name:     "get existing packet",
			remindID: "remind-get-test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retrieved, err := repo.GetCommittedPacket(ctx, tt.remindID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if retrieved.RemindID != packet.RemindID {
				t.Errorf("expected RemindID %s, got %s", packet.RemindID, retrieved.RemindID)
			}
			if retrieved.Lane != packet.Lane {
				t.Errorf("expected Lane %s, got %s", packet.Lane, retrieved.Lane)
			}
			if retrieved.WasShifted() != packet.WasShifted() {
				t.Errorf("expected WasShifted %v, got %v", packet.WasShifted(), retrieved.WasShifted())
			}
		})
	}
}

func TestGetCommittedPacketError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	client, cleanup := testutil.SetupRedisContainer(ctx, t)
	defer cleanup()

	repo := NewThrottleRepository(client)

	tests := []struct {
		name        string
		remindID    string
		expectedErr error
	}{
		{
			name:        "non-existing packet",
			remindID:    "remind-not-found",
			expectedErr: domain.ErrPacketNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := repo.GetCommittedPacket(ctx, tt.remindID)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err != tt.expectedErr {
				t.Errorf("expected error %v, got %v", tt.expectedErr, err)
			}
		})
	}
}

// ===== Plan Tests =====

func TestSavePlanSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	client, cleanup := testutil.SetupRedisContainer(ctx, t)
	defer cleanup()

	repo := NewThrottleRepository(client)

	now := time.Now().UTC().Truncate(time.Millisecond)

	tests := []struct {
		name string
		plan *domain.Plan
	}{
		{
			name: "save plan with single packet",
			plan: &domain.Plan{
				MinuteKey: "2024-01-15-14-30",
				PlannedPackets: []domain.PlannedPacket{
					{
						RemindID:     "remind-plan-001",
						OriginalTime: now,
						PlannedTime:  now,
						Lane:         domain.LaneStrict,
					},
				},
				PlannedAt:    now,
				TotalPlanned: 1,
			},
		},
		{
			name: "save plan with multiple packets",
			plan: &domain.Plan{
				MinuteKey: "2024-01-15-14-31",
				PlannedPackets: []domain.PlannedPacket{
					{
						RemindID:     "remind-plan-002",
						OriginalTime: now,
						PlannedTime:  now,
						Lane:         domain.LaneStrict,
					},
					{
						RemindID:     "remind-plan-003",
						OriginalTime: now,
						PlannedTime:  now.Add(time.Minute),
						Lane:         domain.LaneLoose,
					},
				},
				PlannedAt:    now,
				TotalPlanned: 2,
			},
		},
		{
			name: "save plan with no packets",
			plan: &domain.Plan{
				MinuteKey:      "2024-01-15-14-32",
				PlannedPackets: []domain.PlannedPacket{},
				PlannedAt:      now,
				TotalPlanned:   0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.SavePlan(ctx, tt.plan)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify plan can be retrieved
			retrieved, err := repo.GetPlan(ctx, tt.plan.MinuteKey)
			if err != nil {
				t.Fatalf("failed to get plan: %v", err)
			}

			if retrieved.MinuteKey != tt.plan.MinuteKey {
				t.Errorf("expected MinuteKey %s, got %s", tt.plan.MinuteKey, retrieved.MinuteKey)
			}
			if retrieved.TotalPlanned != tt.plan.TotalPlanned {
				t.Errorf("expected TotalPlanned %d, got %d", tt.plan.TotalPlanned, retrieved.TotalPlanned)
			}
			if len(retrieved.PlannedPackets) != len(tt.plan.PlannedPackets) {
				t.Errorf("expected %d packets, got %d", len(tt.plan.PlannedPackets), len(retrieved.PlannedPackets))
			}

			// Verify TTL is set on plan key
			ttl, err := client.TTL(ctx, "throttle:plan:"+tt.plan.MinuteKey).Result()
			if err != nil {
				t.Fatalf("failed to get TTL: %v", err)
			}
			if ttl <= 0 || ttl > 30*time.Minute {
				t.Errorf("expected TTL around 30 minutes, got %v", ttl)
			}

			// Verify planned packets can be retrieved by remind_id
			for _, packet := range tt.plan.PlannedPackets {
				plannedPacket, err := repo.GetPlannedPacket(ctx, packet.RemindID)
				if err != nil {
					t.Fatalf("failed to get planned packet %s: %v", packet.RemindID, err)
				}
				if plannedPacket.RemindID != packet.RemindID {
					t.Errorf("expected RemindID %s, got %s", packet.RemindID, plannedPacket.RemindID)
				}
			}
		})
	}
}

func TestSavePlanError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	client, cleanup := testutil.SetupRedisContainer(ctx, t)
	defer cleanup()

	repo := NewThrottleRepository(client)

	tests := []struct {
		name        string
		plan        *domain.Plan
		expectedErr error
	}{
		{
			name:        "nil plan",
			plan:        nil,
			expectedErr: ErrInvalidPlanData,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.SavePlan(ctx, tt.plan)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err != tt.expectedErr {
				t.Errorf("expected error %v, got %v", tt.expectedErr, err)
			}
		})
	}
}

func TestGetPlanSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	client, cleanup := testutil.SetupRedisContainer(ctx, t)
	defer cleanup()

	repo := NewThrottleRepository(client)

	now := time.Now().UTC().Truncate(time.Millisecond)

	// Save a plan first
	plan := &domain.Plan{
		MinuteKey: "2024-01-15-15-00",
		PlannedPackets: []domain.PlannedPacket{
			{
				RemindID:     "remind-get-plan-001",
				OriginalTime: now,
				PlannedTime:  now.Add(2 * time.Minute),
				Lane:         domain.LaneLoose,
			},
		},
		PlannedAt:    now,
		TotalPlanned: 1,
	}
	if err := repo.SavePlan(ctx, plan); err != nil {
		t.Fatalf("failed to save plan: %v", err)
	}

	tests := []struct {
		name      string
		minuteKey string
	}{
		{
			name:      "get existing plan",
			minuteKey: "2024-01-15-15-00",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retrieved, err := repo.GetPlan(ctx, tt.minuteKey)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if retrieved.MinuteKey != plan.MinuteKey {
				t.Errorf("expected MinuteKey %s, got %s", plan.MinuteKey, retrieved.MinuteKey)
			}
			if len(retrieved.PlannedPackets) != len(plan.PlannedPackets) {
				t.Errorf("expected %d packets, got %d", len(plan.PlannedPackets), len(retrieved.PlannedPackets))
			}
			if retrieved.TotalPlanned != plan.TotalPlanned {
				t.Errorf("expected TotalPlanned %d, got %d", plan.TotalPlanned, retrieved.TotalPlanned)
			}

			// Verify planned packet data
			if len(retrieved.PlannedPackets) > 0 {
				pp := retrieved.PlannedPackets[0]
				if pp.RemindID != plan.PlannedPackets[0].RemindID {
					t.Errorf("expected RemindID %s, got %s", plan.PlannedPackets[0].RemindID, pp.RemindID)
				}
				if pp.Lane != plan.PlannedPackets[0].Lane {
					t.Errorf("expected Lane %s, got %s", plan.PlannedPackets[0].Lane, pp.Lane)
				}
				if !pp.PlannedTime.Equal(plan.PlannedPackets[0].PlannedTime) {
					t.Errorf("expected PlannedTime %v, got %v", plan.PlannedPackets[0].PlannedTime, pp.PlannedTime)
				}
			}
		})
	}
}

func TestGetPlanError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	client, cleanup := testutil.SetupRedisContainer(ctx, t)
	defer cleanup()

	repo := NewThrottleRepository(client)

	tests := []struct {
		name        string
		minuteKey   string
		expectedErr error
	}{
		{
			name:        "non-existing plan",
			minuteKey:   "2024-01-15-99-99",
			expectedErr: domain.ErrPlanNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := repo.GetPlan(ctx, tt.minuteKey)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err != tt.expectedErr {
				t.Errorf("expected error %v, got %v", tt.expectedErr, err)
			}
		})
	}
}

func TestGetPlansInRangeSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	client, cleanup := testutil.SetupRedisContainer(ctx, t)
	defer cleanup()

	repo := NewThrottleRepository(client)

	now := time.Now().UTC().Truncate(time.Millisecond)

	// Save multiple plans
	plans := []*domain.Plan{
		{
			MinuteKey: "2024-01-15-10-00",
			PlannedPackets: []domain.PlannedPacket{
				{RemindID: "remind-range-001", OriginalTime: now, PlannedTime: now, Lane: domain.LaneStrict},
			},
			PlannedAt:    now,
			TotalPlanned: 1,
		},
		{
			MinuteKey: "2024-01-15-10-05",
			PlannedPackets: []domain.PlannedPacket{
				{RemindID: "remind-range-002", OriginalTime: now, PlannedTime: now, Lane: domain.LaneLoose},
			},
			PlannedAt:    now,
			TotalPlanned: 1,
		},
		{
			MinuteKey: "2024-01-15-10-10",
			PlannedPackets: []domain.PlannedPacket{
				{RemindID: "remind-range-003", OriginalTime: now, PlannedTime: now, Lane: domain.LaneStrict},
			},
			PlannedAt:    now,
			TotalPlanned: 1,
		},
		{
			MinuteKey: "2024-01-15-10-20",
			PlannedPackets: []domain.PlannedPacket{
				{RemindID: "remind-range-004", OriginalTime: now, PlannedTime: now, Lane: domain.LaneLoose},
			},
			PlannedAt:    now,
			TotalPlanned: 1,
		},
	}

	for _, p := range plans {
		if err := repo.SavePlan(ctx, p); err != nil {
			t.Fatalf("failed to save plan: %v", err)
		}
	}

	tests := []struct {
		name          string
		startKey      string
		endKey        string
		expectedCount int
	}{
		{
			name:          "get plans in full range",
			startKey:      "2024-01-15-10-00",
			endKey:        "2024-01-15-10-20",
			expectedCount: 4,
		},
		{
			name:          "get plans in partial range",
			startKey:      "2024-01-15-10-05",
			endKey:        "2024-01-15-10-10",
			expectedCount: 2,
		},
		{
			name:          "get single plan",
			startKey:      "2024-01-15-10-05",
			endKey:        "2024-01-15-10-05",
			expectedCount: 1,
		},
		{
			name:          "empty range returns no plans",
			startKey:      "2024-01-15-11-00",
			endKey:        "2024-01-15-11-30",
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retrieved, err := repo.GetPlansInRange(ctx, tt.startKey, tt.endKey)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(retrieved) != tt.expectedCount {
				t.Errorf("expected %d plans, got %d", tt.expectedCount, len(retrieved))
			}
		})
	}
}

func TestDeletePlanSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	client, cleanup := testutil.SetupRedisContainer(ctx, t)
	defer cleanup()

	repo := NewThrottleRepository(client)

	now := time.Now().UTC().Truncate(time.Millisecond)

	// Save a plan first
	plan := &domain.Plan{
		MinuteKey: "2024-01-15-16-00",
		PlannedPackets: []domain.PlannedPacket{
			{RemindID: "remind-delete-001", OriginalTime: now, PlannedTime: now, Lane: domain.LaneStrict},
			{RemindID: "remind-delete-002", OriginalTime: now, PlannedTime: now, Lane: domain.LaneLoose},
		},
		PlannedAt:    now,
		TotalPlanned: 2,
	}
	if err := repo.SavePlan(ctx, plan); err != nil {
		t.Fatalf("failed to save plan: %v", err)
	}

	// Verify plan exists
	_, err := repo.GetPlan(ctx, plan.MinuteKey)
	if err != nil {
		t.Fatalf("plan should exist before delete: %v", err)
	}

	// Verify planned packets exist
	for _, p := range plan.PlannedPackets {
		_, err := repo.GetPlannedPacket(ctx, p.RemindID)
		if err != nil {
			t.Fatalf("planned packet should exist before delete: %v", err)
		}
	}

	tests := []struct {
		name      string
		minuteKey string
	}{
		{
			name:      "delete existing plan",
			minuteKey: "2024-01-15-16-00",
		},
		{
			name:      "delete non-existing plan is no-op",
			minuteKey: "2024-01-15-99-99",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.DeletePlan(ctx, tt.minuteKey)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}

	// Verify plan is deleted
	_, err = repo.GetPlan(ctx, plan.MinuteKey)
	if err != domain.ErrPlanNotFound {
		t.Errorf("expected ErrPlanNotFound after delete, got %v", err)
	}

	// Verify planned packets are deleted
	for _, p := range plan.PlannedPackets {
		_, err := repo.GetPlannedPacket(ctx, p.RemindID)
		if err != domain.ErrPlannedPacketNotFound {
			t.Errorf("expected ErrPlannedPacketNotFound after delete, got %v", err)
		}
	}
}

func TestGetPlannedPacketSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	client, cleanup := testutil.SetupRedisContainer(ctx, t)
	defer cleanup()

	repo := NewThrottleRepository(client)

	now := time.Now().UTC().Truncate(time.Millisecond)
	shiftedTime := now.Add(3 * time.Minute)

	// Save a plan with packets
	plan := &domain.Plan{
		MinuteKey: "2024-01-15-17-00",
		PlannedPackets: []domain.PlannedPacket{
			{
				RemindID:     "remind-planned-001",
				OriginalTime: now,
				PlannedTime:  now,
				Lane:         domain.LaneStrict,
			},
			{
				RemindID:     "remind-planned-002",
				OriginalTime: now,
				PlannedTime:  shiftedTime,
				Lane:         domain.LaneLoose,
			},
		},
		PlannedAt:    now,
		TotalPlanned: 2,
	}
	if err := repo.SavePlan(ctx, plan); err != nil {
		t.Fatalf("failed to save plan: %v", err)
	}

	tests := []struct {
		name          string
		remindID      string
		expectedLane  domain.Lane
		wasShifted    bool
	}{
		{
			name:         "get strict lane packet not shifted",
			remindID:     "remind-planned-001",
			expectedLane: domain.LaneStrict,
			wasShifted:   false,
		},
		{
			name:         "get loose lane packet shifted",
			remindID:     "remind-planned-002",
			expectedLane: domain.LaneLoose,
			wasShifted:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packet, err := repo.GetPlannedPacket(ctx, tt.remindID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if packet.RemindID != tt.remindID {
				t.Errorf("expected RemindID %s, got %s", tt.remindID, packet.RemindID)
			}
			if packet.Lane != tt.expectedLane {
				t.Errorf("expected Lane %s, got %s", tt.expectedLane, packet.Lane)
			}
			if packet.WasShifted() != tt.wasShifted {
				t.Errorf("expected WasShifted %v, got %v", tt.wasShifted, packet.WasShifted())
			}
		})
	}
}

func TestGetPlannedPacketError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	client, cleanup := testutil.SetupRedisContainer(ctx, t)
	defer cleanup()

	repo := NewThrottleRepository(client)

	tests := []struct {
		name        string
		remindID    string
		expectedErr error
	}{
		{
			name:        "non-existing planned packet",
			remindID:    "remind-not-planned",
			expectedErr: domain.ErrPlannedPacketNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := repo.GetPlannedPacket(ctx, tt.remindID)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err != tt.expectedErr {
				t.Errorf("expected error %v, got %v", tt.expectedErr, err)
			}
		})
	}
}

func TestGetPlannedPacketCountSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	client, cleanup := testutil.SetupRedisContainer(ctx, t)
	defer cleanup()

	repo := NewThrottleRepository(client)

	now := time.Now().UTC().Truncate(time.Millisecond)

	// Save a plan with multiple packets
	plan := &domain.Plan{
		MinuteKey: "2024-01-15-18-00",
		PlannedPackets: []domain.PlannedPacket{
			{RemindID: "remind-count-001", OriginalTime: now, PlannedTime: now, Lane: domain.LaneStrict},
			{RemindID: "remind-count-002", OriginalTime: now, PlannedTime: now, Lane: domain.LaneLoose},
			{RemindID: "remind-count-003", OriginalTime: now, PlannedTime: now, Lane: domain.LaneLoose},
		},
		PlannedAt:    now,
		TotalPlanned: 3,
	}
	if err := repo.SavePlan(ctx, plan); err != nil {
		t.Fatalf("failed to save plan: %v", err)
	}

	tests := []struct {
		name          string
		minuteKey     string
		expectedCount int
	}{
		{
			name:          "get count for existing plan",
			minuteKey:     "2024-01-15-18-00",
			expectedCount: 3,
		},
		{
			name:          "get count for non-existing plan returns 0",
			minuteKey:     "2024-01-15-99-99",
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, err := repo.GetPlannedPacketCount(ctx, tt.minuteKey)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if count != tt.expectedCount {
				t.Errorf("expected count %d, got %d", tt.expectedCount, count)
			}
		})
	}
}
