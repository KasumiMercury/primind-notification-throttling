package sliding

import (
	"container/heap"
	"testing"
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
	"github.com/KasumiMercury/primind-notification-throttling/internal/infra/timemgmt"
)

func TestPriorityQueue_EDF(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	pq := NewPriorityQueue(now)

	// Item with later deadline (12:10)
	item1 := &PriorityItem{
		Remind:  timemgmt.RemindResponse{ID: "late-deadline"},
		Lane:    domain.LaneLoose,
		Release: now,
		Latest:  now.Add(10 * time.Minute),
	}

	// Item with earlier deadline (12:05)
	item2 := &PriorityItem{
		Remind:  timemgmt.RemindResponse{ID: "early-deadline"},
		Lane:    domain.LaneLoose,
		Release: now,
		Latest:  now.Add(5 * time.Minute),
	}

	heap.Push(pq, item1)
	heap.Push(pq, item2)

	// Earlier deadline should come out first
	first := heap.Pop(pq).(*PriorityItem)
	if first.Remind.ID != "early-deadline" {
		t.Errorf("expected early-deadline first, got %s", first.Remind.ID)
	}

	second := heap.Pop(pq).(*PriorityItem)
	if second.Remind.ID != "late-deadline" {
		t.Errorf("expected late-deadline second, got %s", second.Remind.ID)
	}
}

func TestPriorityQueue_MoveCost_LateHeavy(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	slotTime := now.Add(3 * time.Minute) // 12:03

	pq := NewPriorityQueue(slotTime)

	item := &PriorityItem{
		Release: now, // 12:00
	}

	// Late by 3 minutes -> cost = 3 * 10 = 30
	cost := pq.MoveCost(item)
	expected := 30
	if cost != expected {
		t.Errorf("expected move cost %d, got %d", expected, cost)
	}
}

func TestPriorityQueue_MoveCost_EarlyLight(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	slotTime := now // 12:00

	pq := NewPriorityQueue(slotTime)

	item := &PriorityItem{
		Release: now.Add(3 * time.Minute), // 12:03
	}

	// Early by 3 minutes -> cost = 3 * 1 = 3
	cost := pq.MoveCost(item)
	expected := 3
	if cost != expected {
		t.Errorf("expected move cost %d, got %d", expected, cost)
	}
}

func TestPriorityQueue_MoveCost_NoShift(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	pq := NewPriorityQueue(now)

	item := &PriorityItem{
		Release: now,
	}

	// No shift -> cost = 0
	cost := pq.MoveCost(item)
	if cost != 0 {
		t.Errorf("expected move cost 0, got %d", cost)
	}
}

func TestPriorityQueue_MoveCostTiebreaker(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	slotTime := now.Add(2 * time.Minute) // 12:02

	pq := NewPriorityQueue(slotTime)

	// Same deadline, but different move costs
	deadline := now.Add(10 * time.Minute)

	// item1: release at 12:00, slot at 12:02 -> late by 2 min -> cost = 20
	item1 := &PriorityItem{
		Remind:  timemgmt.RemindResponse{ID: "higher-cost"},
		Release: now,
		Latest:  deadline,
	}

	// item2: release at 12:02, slot at 12:02 -> no shift -> cost = 0
	item2 := &PriorityItem{
		Remind:  timemgmt.RemindResponse{ID: "lower-cost"},
		Release: now.Add(2 * time.Minute),
		Latest:  deadline,
	}

	heap.Push(pq, item1)
	heap.Push(pq, item2)

	// Lower cost should come out first
	first := heap.Pop(pq).(*PriorityItem)
	if first.Remind.ID != "lower-cost" {
		t.Errorf("expected lower-cost first, got %s", first.Remind.ID)
	}
}

func TestPriorityQueue_SetSlotTime(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	pq := NewPriorityQueue(now)

	// Same deadline for all
	deadline := now.Add(10 * time.Minute)

	// At slot 12:00: item1 (release 12:00) has cost 0, item2 (release 12:02) has cost 2
	item1 := &PriorityItem{
		Remind:  timemgmt.RemindResponse{ID: "release-12:00"},
		Release: now,
		Latest:  deadline,
	}
	item2 := &PriorityItem{
		Remind:  timemgmt.RemindResponse{ID: "release-12:02"},
		Release: now.Add(2 * time.Minute),
		Latest:  deadline,
	}

	heap.Push(pq, item1)
	heap.Push(pq, item2)

	// At 12:00, item1 should be first (cost 0 vs cost 2)
	first := heap.Pop(pq).(*PriorityItem)
	if first.Remind.ID != "release-12:00" {
		t.Errorf("at 12:00, expected release-12:00 first, got %s", first.Remind.ID)
	}

	// Re-add item1
	heap.Push(pq, item1)

	// Change to slot 12:04
	// item1: late by 4 min -> cost = 40
	// item2: late by 2 min -> cost = 20
	pq.SetSlotTime(now.Add(4 * time.Minute))

	first = heap.Pop(pq).(*PriorityItem)
	if first.Remind.ID != "release-12:02" {
		t.Errorf("at 12:04, expected release-12:02 first, got %s", first.Remind.ID)
	}
}

func TestPriorityQueue_HeapOperations(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	pq := NewPriorityQueue(now)

	if pq.Len() != 0 {
		t.Errorf("expected empty queue, got len %d", pq.Len())
	}

	items := []*PriorityItem{
		{Remind: timemgmt.RemindResponse{ID: "a"}, Latest: now.Add(5 * time.Minute)},
		{Remind: timemgmt.RemindResponse{ID: "b"}, Latest: now.Add(3 * time.Minute)},
		{Remind: timemgmt.RemindResponse{ID: "c"}, Latest: now.Add(7 * time.Minute)},
	}

	for _, item := range items {
		heap.Push(pq, item)
	}

	if pq.Len() != 3 {
		t.Errorf("expected len 3, got %d", pq.Len())
	}

	// Should pop in EDF order: b (3min), a (5min), c (7min)
	expectedOrder := []string{"b", "a", "c"}
	for i, expected := range expectedOrder {
		item := heap.Pop(pq).(*PriorityItem)
		if item.Remind.ID != expected {
			t.Errorf("position %d: expected %s, got %s", i, expected, item.Remind.ID)
		}
	}

	if pq.Len() != 0 {
		t.Errorf("expected empty queue after pops, got len %d", pq.Len())
	}
}

func TestPriorityItem_IsEligibleForSlot(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	item := &PriorityItem{
		Release: now,
		Latest:  now.Add(5 * time.Minute),
	}

	tests := []struct {
		name     string
		slotTime time.Time
		expected bool
	}{
		{"before release", now.Add(-1 * time.Minute), true},
		{"at release", now, true},
		{"within window", now.Add(3 * time.Minute), true},
		{"at deadline", now.Add(5 * time.Minute), true},
		{"after deadline", now.Add(6 * time.Minute), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := item.IsEligibleForSlot(tt.slotTime)
			if result != tt.expected {
				t.Errorf("IsEligibleForSlot(%v) = %v, want %v", tt.slotTime, result, tt.expected)
			}
		})
	}
}

func TestPriorityItem_CalculatePlannedTime(t *testing.T) {
	// Original time with sub-minute offset: 12:00:45
	originalTime := time.Date(2024, 1, 1, 12, 0, 45, 0, time.UTC)

	item := &PriorityItem{
		Remind: timemgmt.RemindResponse{Time: originalTime},
	}

	// Assign to 12:03 slot
	slotMinute := time.Date(2024, 1, 1, 12, 3, 0, 0, time.UTC)

	plannedTime := item.CalculatePlannedTime(slotMinute)

	// Should preserve the 45-second offset
	expected := time.Date(2024, 1, 1, 12, 3, 45, 0, time.UTC)
	if !plannedTime.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, plannedTime)
	}
}
