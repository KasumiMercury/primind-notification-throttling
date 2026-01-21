package sliding

import (
	"container/heap"
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service/smoothing"
)

type AllocationResult struct {
	Assignments map[string]SlideResult // remindID -> result
}

type slotState struct {
	MinuteKey    string
	MinuteTime   time.Time
	Available    int
	CurrentCount int
}

type PriorityAllocator struct{}

func NewPriorityAllocator() *PriorityAllocator {
	return &PriorityAllocator{}
}

// For each slot:
// Build a heap of eligible notifications (release <= slot <= deadline)
// Pop highest priority notifications until slot is full
// Mark assigned notifications as done
func (a *PriorityAllocator) AllocateBatch(
	ctx context.Context,
	items []*PriorityItem,
	slideCtx *SlideContext,
) *AllocationResult {
	result := &AllocationResult{
		Assignments: make(map[string]SlideResult),
	}

	if len(items) == 0 {
		return result
	}

	slots := a.copySlotStates(slideCtx.Targets)
	if len(slots) == 0 {
		// No slots available, use original times
		for _, item := range items {
			result.Assignments[item.Remind.ID] = SlideResult{
				PlannedTime: item.Remind.Time,
				WasShifted:  false,
			}
		}
		return result
	}

	sort.Slice(slots, func(i, j int) bool {
		return slots[i].MinuteTime.Before(slots[j].MinuteTime)
	})

	// Check if all slots have Available <= 0 (all at or over smoothing target)
	// In this case, we fall back to hard cap allocation to allow some shifting
	allSlotsAtTarget := true
	for _, slot := range slots {
		if slot.Available > 0 {
			allSlotsAtTarget = false
			break
		}
	}

	assigned := make(map[string]bool)

	for slotIdx := range slots {
		slot := &slots[slotIdx]

		// Hard cap check is always enforced
		if slot.CurrentCount >= slideCtx.CapPerMinute {
			continue
		}

		// Prefer slots with Available > 0
		// But if all slots are at target (Available <= 0), allow any slot under hard cap
		if slot.Available <= 0 && !allSlotsAtTarget {
			continue
		}

		pq := NewPriorityQueue(slot.MinuteTime)
		for _, item := range items {
			if assigned[item.Remind.ID] {
				continue
			}
			if item.IsEligibleForSlot(slot.MinuteTime) {
				heap.Push(pq, item)
			}
		}

		// Assign highest priority items until slot is full
		for pq.Len() > 0 && slot.Available > 0 && slot.CurrentCount < slideCtx.CapPerMinute {
			item := heap.Pop(pq).(*PriorityItem)

			plannedTime := item.CalculatePlannedTime(slot.MinuteTime)
			wasShifted := domain.MinuteKey(plannedTime) != domain.MinuteKey(item.Remind.Time)

			result.Assignments[item.Remind.ID] = SlideResult{
				PlannedTime: plannedTime,
				WasShifted:  wasShifted,
			}

			assigned[item.Remind.ID] = true
			slot.Available--
			slot.CurrentCount++

			slog.DebugContext(ctx, "priority allocator: assigned notification to slot",
				slog.String("remind_id", item.Remind.ID),
				slog.String("slot_key", slot.MinuteKey),
				slog.String("lane", item.Lane.String()),
				slog.Bool("shifted", wasShifted),
			)
		}
	}

	// Handle unassigned notifications
	// use original time as fallback
	for _, item := range items {
		if !assigned[item.Remind.ID] {
			result.Assignments[item.Remind.ID] = SlideResult{
				PlannedTime: item.Remind.Time,
				WasShifted:  false,
			}
			slog.DebugContext(ctx, "priority allocator: notification unassigned, using original time",
				slog.String("remind_id", item.Remind.ID),
				slog.String("lane", item.Lane.String()),
			)
		}
	}

	return result
}

func (a *PriorityAllocator) copySlotStates(targets []smoothing.TargetAllocation) []slotState {
	slots := make([]slotState, len(targets))
	for i, t := range targets {
		slots[i] = slotState{
			MinuteKey:    t.MinuteKey,
			MinuteTime:   t.MinuteTime,
			Available:    t.Available,
			CurrentCount: t.CurrentCount,
		}
	}
	return slots
}
