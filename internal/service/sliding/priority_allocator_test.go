package sliding

import (
	"context"
	"testing"
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
	"github.com/KasumiMercury/primind-notification-throttling/internal/infra/timemgmt"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service/smoothing"
)

func TestPriorityAllocator_EmptyBatch(t *testing.T) {
	allocator := NewPriorityAllocator()
	ctx := context.Background()

	slideCtx := &SlideContext{
		Targets:      []smoothing.TargetAllocation{},
		CapPerMinute: 10,
	}

	result := allocator.AllocateBatch(ctx, []*PriorityItem{}, slideCtx)

	if len(result.Assignments) != 0 {
		t.Errorf("expected 0 assignments, got %d", len(result.Assignments))
	}
}

func TestPriorityAllocator_SingleNotification(t *testing.T) {
	allocator := NewPriorityAllocator()
	ctx := context.Background()

	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	items := []*PriorityItem{
		{
			Remind:  timemgmt.RemindResponse{ID: "remind-1", Time: now},
			Lane:    domain.LaneLoose,
			Release: now,
			Latest:  now.Add(5 * time.Minute),
		},
	}

	slideCtx := &SlideContext{
		Targets: []smoothing.TargetAllocation{
			{MinuteKey: "2024-01-01-12-00", MinuteTime: now, Target: 10, CurrentCount: 0, Available: 10},
		},
		CapPerMinute: 10,
	}

	result := allocator.AllocateBatch(ctx, items, slideCtx)

	if len(result.Assignments) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(result.Assignments))
	}

	assignment := result.Assignments["remind-1"]
	if !assignment.PlannedTime.Equal(now) {
		t.Errorf("expected planned time %v, got %v", now, assignment.PlannedTime)
	}
	if assignment.WasShifted {
		t.Errorf("expected no shift for notification at original slot")
	}
}

func TestPriorityAllocator_EDFOrdering(t *testing.T) {
	allocator := NewPriorityAllocator()
	ctx := context.Background()

	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Both notifications want the same slot (12:00), but only 1 capacity
	// EDF should pick the one with earlier deadline
	items := []*PriorityItem{
		{
			Remind:  timemgmt.RemindResponse{ID: "late-deadline", Time: now},
			Lane:    domain.LaneLoose,
			Release: now,
			Latest:  now.Add(10 * time.Minute), // deadline 12:10
		},
		{
			Remind:  timemgmt.RemindResponse{ID: "early-deadline", Time: now},
			Lane:    domain.LaneLoose,
			Release: now,
			Latest:  now.Add(3 * time.Minute), // deadline 12:03
		},
	}

	slideCtx := &SlideContext{
		Targets: []smoothing.TargetAllocation{
			{MinuteKey: "2024-01-01-12-00", MinuteTime: now, Target: 1, CurrentCount: 0, Available: 1},
			{MinuteKey: "2024-01-01-12-01", MinuteTime: now.Add(time.Minute), Target: 1, CurrentCount: 0, Available: 1},
		},
		CapPerMinute: 1,
	}

	result := allocator.AllocateBatch(ctx, items, slideCtx)

	// early-deadline should get 12:00 (no shift)
	earlyAssignment := result.Assignments["early-deadline"]
	if !earlyAssignment.PlannedTime.Equal(now) {
		t.Errorf("early-deadline: expected 12:00, got %v", earlyAssignment.PlannedTime)
	}
	if earlyAssignment.WasShifted {
		t.Errorf("early-deadline: expected no shift")
	}

	// late-deadline should be shifted to 12:01
	lateAssignment := result.Assignments["late-deadline"]
	expected := now.Add(time.Minute)
	if !lateAssignment.PlannedTime.Equal(expected) {
		t.Errorf("late-deadline: expected 12:01, got %v", lateAssignment.PlannedTime)
	}
	if !lateAssignment.WasShifted {
		t.Errorf("late-deadline: expected shift")
	}
}

func TestPriorityAllocator_SlotCapacity(t *testing.T) {
	allocator := NewPriorityAllocator()
	ctx := context.Background()

	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// 3 notifications, but slot has capacity for only 2
	items := []*PriorityItem{
		{
			Remind:  timemgmt.RemindResponse{ID: "n1", Time: now},
			Lane:    domain.LaneLoose,
			Release: now,
			Latest:  now.Add(5 * time.Minute),
		},
		{
			Remind:  timemgmt.RemindResponse{ID: "n2", Time: now},
			Lane:    domain.LaneLoose,
			Release: now,
			Latest:  now.Add(5 * time.Minute),
		},
		{
			Remind:  timemgmt.RemindResponse{ID: "n3", Time: now},
			Lane:    domain.LaneLoose,
			Release: now,
			Latest:  now.Add(5 * time.Minute),
		},
	}

	slideCtx := &SlideContext{
		Targets: []smoothing.TargetAllocation{
			{MinuteKey: "2024-01-01-12-00", MinuteTime: now, Target: 2, CurrentCount: 0, Available: 2},
			{MinuteKey: "2024-01-01-12-01", MinuteTime: now.Add(time.Minute), Target: 2, CurrentCount: 0, Available: 2},
		},
		CapPerMinute: 2,
	}

	result := allocator.AllocateBatch(ctx, items, slideCtx)

	// Count how many are at 12:00 vs 12:01
	countAt1200 := 0
	countAt1201 := 0
	for _, assignment := range result.Assignments {
		if assignment.PlannedTime.Equal(now) {
			countAt1200++
		} else if assignment.PlannedTime.Equal(now.Add(time.Minute)) {
			countAt1201++
		}
	}

	if countAt1200 != 2 {
		t.Errorf("expected 2 at 12:00, got %d", countAt1200)
	}
	if countAt1201 != 1 {
		t.Errorf("expected 1 at 12:01, got %d", countAt1201)
	}
}

func TestPriorityAllocator_SlideWindowBoundary(t *testing.T) {
	allocator := NewPriorityAllocator()
	ctx := context.Background()

	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Notification with narrow window that can only fit in 12:00-12:01
	items := []*PriorityItem{
		{
			Remind:  timemgmt.RemindResponse{ID: "narrow-window", Time: now},
			Lane:    domain.LaneStrict,
			Release: now,
			Latest:  now.Add(1 * time.Minute), // Can only use 12:00 or 12:01
		},
	}

	slideCtx := &SlideContext{
		Targets: []smoothing.TargetAllocation{
			// 12:00 is full
			{MinuteKey: "2024-01-01-12-00", MinuteTime: now, Target: 1, CurrentCount: 1, Available: 0},
			// 12:01 has capacity
			{MinuteKey: "2024-01-01-12-01", MinuteTime: now.Add(time.Minute), Target: 1, CurrentCount: 0, Available: 1},
			// 12:02 has capacity but outside window
			{MinuteKey: "2024-01-01-12-02", MinuteTime: now.Add(2 * time.Minute), Target: 1, CurrentCount: 0, Available: 1},
		},
		CapPerMinute: 1,
	}

	result := allocator.AllocateBatch(ctx, items, slideCtx)

	assignment := result.Assignments["narrow-window"]
	expected := now.Add(time.Minute) // Should be shifted to 12:01

	if !assignment.PlannedTime.Equal(expected) {
		t.Errorf("expected 12:01, got %v", assignment.PlannedTime)
	}
	if !assignment.WasShifted {
		t.Errorf("expected shift")
	}
}

func TestPriorityAllocator_MoveCostTiebreaker(t *testing.T) {
	allocator := NewPriorityAllocator()
	ctx := context.Background()

	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	slot1201 := now.Add(time.Minute) // 12:01

	// Two notifications with same deadline, different original times
	// At slot 12:01:
	// - n1 (release 12:00): late by 1 min -> cost = 10
	// - n2 (release 12:01): no shift -> cost = 0
	items := []*PriorityItem{
		{
			Remind:  timemgmt.RemindResponse{ID: "n1-higher-cost", Time: now},
			Lane:    domain.LaneLoose,
			Release: now,
			Latest:  now.Add(5 * time.Minute),
		},
		{
			Remind:  timemgmt.RemindResponse{ID: "n2-lower-cost", Time: slot1201},
			Lane:    domain.LaneLoose,
			Release: slot1201,
			Latest:  slot1201.Add(4 * time.Minute), // Same deadline as n1
		},
	}

	slideCtx := &SlideContext{
		Targets: []smoothing.TargetAllocation{
			// 12:00 is full
			{MinuteKey: "2024-01-01-12-00", MinuteTime: now, Target: 1, CurrentCount: 1, Available: 0},
			// Only 12:01 has capacity for 1
			{MinuteKey: "2024-01-01-12-01", MinuteTime: slot1201, Target: 1, CurrentCount: 0, Available: 1},
		},
		CapPerMinute: 1,
	}

	result := allocator.AllocateBatch(ctx, items, slideCtx)

	// n2 should get 12:01 (lower move cost)
	n2Assignment := result.Assignments["n2-lower-cost"]
	if !n2Assignment.PlannedTime.Equal(slot1201) {
		t.Errorf("n2: expected 12:01, got %v", n2Assignment.PlannedTime)
	}

	// n1 should fallback to original time (no slot available)
	n1Assignment := result.Assignments["n1-higher-cost"]
	if !n1Assignment.PlannedTime.Equal(now) {
		t.Errorf("n1: expected fallback to 12:00, got %v", n1Assignment.PlannedTime)
	}
}

func TestPriorityAllocator_OverflowFallback(t *testing.T) {
	allocator := NewPriorityAllocator()
	ctx := context.Background()

	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Notification that cannot fit in any available slot
	items := []*PriorityItem{
		{
			Remind:  timemgmt.RemindResponse{ID: "overflow", Time: now},
			Lane:    domain.LaneLoose,
			Release: now,
			Latest:  now.Add(2 * time.Minute), // Can use 12:00-12:02
		},
	}

	slideCtx := &SlideContext{
		Targets: []smoothing.TargetAllocation{
			// All slots within window are full
			{MinuteKey: "2024-01-01-12-00", MinuteTime: now, Target: 1, CurrentCount: 1, Available: 0},
			{MinuteKey: "2024-01-01-12-01", MinuteTime: now.Add(time.Minute), Target: 1, CurrentCount: 1, Available: 0},
			{MinuteKey: "2024-01-01-12-02", MinuteTime: now.Add(2 * time.Minute), Target: 1, CurrentCount: 1, Available: 0},
		},
		CapPerMinute: 1,
	}

	result := allocator.AllocateBatch(ctx, items, slideCtx)

	// Should fallback to original time
	assignment := result.Assignments["overflow"]
	if !assignment.PlannedTime.Equal(now) {
		t.Errorf("expected fallback to original time, got %v", assignment.PlannedTime)
	}
	if assignment.WasShifted {
		t.Errorf("expected no shift for fallback")
	}
}

func TestPriorityAllocator_BothLanes(t *testing.T) {
	allocator := NewPriorityAllocator()
	ctx := context.Background()

	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Mix of loose and strict notifications
	items := []*PriorityItem{
		{
			Remind:  timemgmt.RemindResponse{ID: "loose-1", Time: now},
			Lane:    domain.LaneLoose,
			Release: now,
			Latest:  now.Add(5 * time.Minute),
		},
		{
			Remind:  timemgmt.RemindResponse{ID: "strict-1", Time: now},
			Lane:    domain.LaneStrict,
			Release: now,
			Latest:  now.Add(2 * time.Minute), // Stricter deadline
		},
	}

	slideCtx := &SlideContext{
		Targets: []smoothing.TargetAllocation{
			{MinuteKey: "2024-01-01-12-00", MinuteTime: now, Target: 1, CurrentCount: 0, Available: 1},
			{MinuteKey: "2024-01-01-12-01", MinuteTime: now.Add(time.Minute), Target: 1, CurrentCount: 0, Available: 1},
		},
		CapPerMinute: 1,
	}

	result := allocator.AllocateBatch(ctx, items, slideCtx)

	// Strict should get 12:00 (earlier deadline due to EDF)
	strictAssignment := result.Assignments["strict-1"]
	if !strictAssignment.PlannedTime.Equal(now) {
		t.Errorf("strict-1: expected 12:00, got %v", strictAssignment.PlannedTime)
	}

	// Loose should be shifted to 12:01
	looseAssignment := result.Assignments["loose-1"]
	expected := now.Add(time.Minute)
	if !looseAssignment.PlannedTime.Equal(expected) {
		t.Errorf("loose-1: expected 12:01, got %v", looseAssignment.PlannedTime)
	}
}

func TestPriorityAllocator_PreservesSubMinuteOffset(t *testing.T) {
	allocator := NewPriorityAllocator()
	ctx := context.Background()

	// Original time with sub-minute offset: 12:00:45
	originalTime := time.Date(2024, 1, 1, 12, 0, 45, 0, time.UTC)
	slot := originalTime.Truncate(time.Minute)

	items := []*PriorityItem{
		{
			Remind:  timemgmt.RemindResponse{ID: "with-offset", Time: originalTime},
			Lane:    domain.LaneLoose,
			Release: originalTime,
			Latest:  originalTime.Add(5 * time.Minute),
		},
	}

	slideCtx := &SlideContext{
		Targets: []smoothing.TargetAllocation{
			// 12:00 is full
			{MinuteKey: "2024-01-01-12-00", MinuteTime: slot, Target: 1, CurrentCount: 1, Available: 0},
			// 12:01 has capacity
			{MinuteKey: "2024-01-01-12-01", MinuteTime: slot.Add(time.Minute), Target: 1, CurrentCount: 0, Available: 1},
		},
		CapPerMinute: 1,
	}

	result := allocator.AllocateBatch(ctx, items, slideCtx)

	assignment := result.Assignments["with-offset"]
	// Should be shifted to 12:01 but preserve 45-second offset
	expected := time.Date(2024, 1, 1, 12, 1, 45, 0, time.UTC)
	if !assignment.PlannedTime.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, assignment.PlannedTime)
	}
}

func TestPriorityAllocator_NoTargets(t *testing.T) {
	allocator := NewPriorityAllocator()
	ctx := context.Background()

	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	items := []*PriorityItem{
		{
			Remind:  timemgmt.RemindResponse{ID: "n1", Time: now},
			Lane:    domain.LaneLoose,
			Release: now,
			Latest:  now.Add(5 * time.Minute),
		},
	}

	// No targets available
	slideCtx := &SlideContext{
		Targets:      []smoothing.TargetAllocation{},
		CapPerMinute: 10,
	}

	result := allocator.AllocateBatch(ctx, items, slideCtx)

	// Should fallback to original time
	assignment := result.Assignments["n1"]
	if !assignment.PlannedTime.Equal(now) {
		t.Errorf("expected fallback to original time, got %v", assignment.PlannedTime)
	}
}

func TestPriorityAllocator_HardCapRespected(t *testing.T) {
	allocator := NewPriorityAllocator()
	ctx := context.Background()

	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// 3 notifications for a slot that has Available=10 but CurrentCount already at cap
	items := []*PriorityItem{
		{
			Remind:  timemgmt.RemindResponse{ID: "n1", Time: now},
			Lane:    domain.LaneLoose,
			Release: now,
			Latest:  now.Add(5 * time.Minute),
		},
	}

	slideCtx := &SlideContext{
		Targets: []smoothing.TargetAllocation{
			// Available is 10, but CurrentCount is already at CapPerMinute
			{MinuteKey: "2024-01-01-12-00", MinuteTime: now, Target: 10, CurrentCount: 5, Available: 10},
		},
		CapPerMinute: 5, // Hard cap at 5
	}

	result := allocator.AllocateBatch(ctx, items, slideCtx)

	// Should fallback to original time since hard cap is reached
	assignment := result.Assignments["n1"]
	if !assignment.PlannedTime.Equal(now) {
		t.Errorf("expected fallback to original time, got %v", assignment.PlannedTime)
	}
}
