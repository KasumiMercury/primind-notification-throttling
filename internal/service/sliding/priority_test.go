package sliding

import (
	"context"
	"testing"
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
	"github.com/KasumiMercury/primind-notification-throttling/internal/infra/timemgmt"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service/smoothing"
)

func TestPriorityDiscovery_SupportsBatch(t *testing.T) {
	discovery := NewPriorityDiscovery()

	if !discovery.SupportsBatch() {
		t.Errorf("PriorityDiscovery should support batch processing")
	}
}

func TestPriorityDiscovery_PrepareAndLookup(t *testing.T) {
	discovery := NewPriorityDiscovery()
	ctx := context.Background()

	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	looseItems := []*PriorityItem{
		NewPriorityItem(timemgmt.RemindResponse{
			ID:               "loose-1",
			Time:             now,
			SlideWindowWidth: 300, // 5 minutes
		}, domain.LaneLoose),
	}

	strictItems := []*PriorityItem{
		NewPriorityItem(timemgmt.RemindResponse{
			ID:               "strict-1",
			Time:             now,
			SlideWindowWidth: 120, // 2 minutes
		}, domain.LaneStrict),
	}

	slideCtx := &SlideContext{
		Targets: []smoothing.TargetAllocation{
			{MinuteKey: "2024-01-01-12-00", MinuteTime: now, Target: 1, CurrentCount: 0, Available: 1},
			{MinuteKey: "2024-01-01-12-01", MinuteTime: now.Add(time.Minute), Target: 1, CurrentCount: 0, Available: 1},
		},
		CapPerMinute: 1,
	}

	// Prepare slots
	err := discovery.PrepareSlots(ctx, looseItems, strictItems, slideCtx)
	if err != nil {
		t.Fatalf("PrepareSlots failed: %v", err)
	}

	// Lookup results
	looseResult := discovery.FindSlotForLoose(ctx, looseItems[0].Remind, slideCtx)
	strictResult := discovery.FindSlotForStrict(ctx, strictItems[0].Remind, slideCtx)

	// Strict has earlier deadline, should get 12:00
	if !strictResult.PlannedTime.Equal(now) {
		t.Errorf("strict-1: expected 12:00, got %v", strictResult.PlannedTime)
	}

	// Loose should be shifted to 12:01
	expected := now.Add(time.Minute)
	if !looseResult.PlannedTime.Equal(expected) {
		t.Errorf("loose-1: expected 12:01, got %v", looseResult.PlannedTime)
	}
}

func TestPriorityDiscovery_WithoutPrepare(t *testing.T) {
	discovery := NewPriorityDiscovery()
	ctx := context.Background()

	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	remind := timemgmt.RemindResponse{
		ID:   "test-1",
		Time: now,
	}

	slideCtx := &SlideContext{
		Targets:      []smoothing.TargetAllocation{},
		CapPerMinute: 10,
	}

	// Call FindSlotForLoose without PrepareSlots
	result := discovery.FindSlotForLoose(ctx, remind, slideCtx)

	// Should fallback to original time
	if !result.PlannedTime.Equal(now) {
		t.Errorf("expected fallback to original time, got %v", result.PlannedTime)
	}
	if result.WasShifted {
		t.Errorf("expected no shift for fallback")
	}
}

func TestPriorityDiscovery_Reset(t *testing.T) {
	discovery := NewPriorityDiscovery()
	ctx := context.Background()

	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	items := []*PriorityItem{
		NewPriorityItem(timemgmt.RemindResponse{
			ID:               "test-1",
			Time:             now,
			SlideWindowWidth: 300,
		}, domain.LaneLoose),
	}

	slideCtx := &SlideContext{
		Targets: []smoothing.TargetAllocation{
			{MinuteKey: "2024-01-01-12-00", MinuteTime: now, Target: 1, CurrentCount: 0, Available: 1},
		},
		CapPerMinute: 1,
	}

	// First batch
	_ = discovery.PrepareSlots(ctx, items, nil, slideCtx)
	result := discovery.FindSlotForLoose(ctx, items[0].Remind, slideCtx)
	if !result.PlannedTime.Equal(now) {
		t.Errorf("first batch: expected 12:00, got %v", result.PlannedTime)
	}

	// Reset
	discovery.Reset()

	// After reset, should fallback to original time
	result = discovery.FindSlotForLoose(ctx, items[0].Remind, slideCtx)
	if !result.PlannedTime.Equal(now) {
		t.Errorf("after reset: expected fallback to 12:00, got %v", result.PlannedTime)
	}
}

func TestPriorityDiscovery_UpdateContext(t *testing.T) {
	discovery := NewPriorityDiscovery()

	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	slideCtx := &SlideContext{
		Targets: []smoothing.TargetAllocation{
			{MinuteKey: "2024-01-01-12-00", MinuteTime: now, Target: 5, CurrentCount: 2, Available: 3},
		},
		CapPerMinute: 10,
	}

	// UpdateContext should increment CurrentCount and decrement Available
	discovery.UpdateContext(slideCtx, "2024-01-01-12-00")

	target := slideCtx.Targets[0]
	if target.CurrentCount != 3 {
		t.Errorf("expected CurrentCount 3, got %d", target.CurrentCount)
	}
	if target.Available != 2 {
		t.Errorf("expected Available 2, got %d", target.Available)
	}
}

func TestPriorityDiscovery_EDFWithBothLanes(t *testing.T) {
	discovery := NewPriorityDiscovery()
	ctx := context.Background()

	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Loose with later deadline
	looseItems := []*PriorityItem{
		NewPriorityItem(timemgmt.RemindResponse{
			ID:               "loose-later-deadline",
			Time:             now,
			SlideWindowWidth: 600, // 10 minutes, deadline 12:10
		}, domain.LaneLoose),
	}

	// Strict with earlier deadline
	strictItems := []*PriorityItem{
		NewPriorityItem(timemgmt.RemindResponse{
			ID:               "strict-earlier-deadline",
			Time:             now,
			SlideWindowWidth: 120, // 2 minutes, deadline 12:02
		}, domain.LaneStrict),
	}

	slideCtx := &SlideContext{
		Targets: []smoothing.TargetAllocation{
			{MinuteKey: "2024-01-01-12-00", MinuteTime: now, Target: 1, CurrentCount: 0, Available: 1},
			{MinuteKey: "2024-01-01-12-01", MinuteTime: now.Add(time.Minute), Target: 1, CurrentCount: 0, Available: 1},
		},
		CapPerMinute: 1,
	}

	_ = discovery.PrepareSlots(ctx, looseItems, strictItems, slideCtx)

	// Strict with earlier deadline should get priority for 12:00
	strictResult := discovery.FindSlotForStrict(ctx, strictItems[0].Remind, slideCtx)
	if !strictResult.PlannedTime.Equal(now) {
		t.Errorf("strict (earlier deadline): expected 12:00, got %v", strictResult.PlannedTime)
	}
	if strictResult.WasShifted {
		t.Errorf("strict: expected no shift")
	}

	// Loose should be shifted
	looseResult := discovery.FindSlotForLoose(ctx, looseItems[0].Remind, slideCtx)
	expected := now.Add(time.Minute)
	if !looseResult.PlannedTime.Equal(expected) {
		t.Errorf("loose (later deadline): expected 12:01, got %v", looseResult.PlannedTime)
	}
	if !looseResult.WasShifted {
		t.Errorf("loose: expected shift")
	}
}

func TestPriorityDiscovery_NotificationNotInBatch(t *testing.T) {
	discovery := NewPriorityDiscovery()
	ctx := context.Background()

	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Prepare with one notification
	items := []*PriorityItem{
		NewPriorityItem(timemgmt.RemindResponse{
			ID:               "prepared",
			Time:             now,
			SlideWindowWidth: 300,
		}, domain.LaneLoose),
	}

	slideCtx := &SlideContext{
		Targets: []smoothing.TargetAllocation{
			{MinuteKey: "2024-01-01-12-00", MinuteTime: now, Target: 1, CurrentCount: 0, Available: 1},
		},
		CapPerMinute: 1,
	}

	_ = discovery.PrepareSlots(ctx, items, nil, slideCtx)

	// Lookup a different notification that wasn't in the batch
	unknownRemind := timemgmt.RemindResponse{
		ID:   "unknown",
		Time: now.Add(5 * time.Minute),
	}

	result := discovery.FindSlotForLoose(ctx, unknownRemind, slideCtx)

	// Should fallback to original time
	if !result.PlannedTime.Equal(unknownRemind.Time) {
		t.Errorf("expected fallback to original time, got %v", result.PlannedTime)
	}
}
