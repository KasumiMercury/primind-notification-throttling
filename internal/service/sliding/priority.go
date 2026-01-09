package sliding

import (
	"context"
	"log/slog"

	"github.com/KasumiMercury/primind-notification-throttling/internal/infra/timemgmt"
)

var _ Discovery = (*PriorityDiscovery)(nil)

type PriorityDiscovery struct {
	allocator   *PriorityAllocator
	assignments map[string]SlideResult
	prepared    bool
}

func NewPriorityDiscovery() *PriorityDiscovery {
	return &PriorityDiscovery{
		allocator:   NewPriorityAllocator(),
		assignments: make(map[string]SlideResult),
		prepared:    false,
	}
}

func (p *PriorityDiscovery) SupportsBatch() bool {
	return true
}

func (p *PriorityDiscovery) PrepareSlots(
	ctx context.Context,
	looseItems []*PriorityItem,
	strictItems []*PriorityItem,
	slideCtx *SlideContext,
) error {
	allItems := make([]*PriorityItem, 0, len(looseItems)+len(strictItems))
	allItems = append(allItems, looseItems...)
	allItems = append(allItems, strictItems...)

	slog.DebugContext(ctx, "priority discovery: preparing slots",
		slog.Int("loose_count", len(looseItems)),
		slog.Int("strict_count", len(strictItems)),
		slog.Int("total", len(allItems)),
	)

	result := p.allocator.AllocateBatch(ctx, allItems, slideCtx)

	p.assignments = result.Assignments
	p.prepared = true

	slog.DebugContext(ctx, "priority discovery: slots prepared",
		slog.Int("assignments", len(p.assignments)),
	)

	return nil
}

func (p *PriorityDiscovery) FindSlotForLoose(
	ctx context.Context,
	remind timemgmt.RemindResponse,
	slideCtx *SlideContext,
) SlideResult {
	return p.lookupAssignment(ctx, remind)
}

func (p *PriorityDiscovery) FindSlotForStrict(
	ctx context.Context,
	remind timemgmt.RemindResponse,
	slideCtx *SlideContext,
) SlideResult {
	return p.lookupAssignment(ctx, remind)
}

func (p *PriorityDiscovery) lookupAssignment(
	ctx context.Context,
	remind timemgmt.RemindResponse,
) SlideResult {
	if !p.prepared {
		slog.WarnContext(ctx, "priority discovery: PrepareSlots not called, using original time",
			slog.String("remind_id", remind.ID),
		)
		return SlideResult{PlannedTime: remind.Time, WasShifted: false}
	}

	if result, ok := p.assignments[remind.ID]; ok {
		return result
	}

	// fallback
	slog.WarnContext(ctx, "priority discovery: notification not in prepared batch, using original time",
		slog.String("remind_id", remind.ID),
	)
	return SlideResult{PlannedTime: remind.Time, WasShifted: false}
}

func (p *PriorityDiscovery) UpdateContext(slideCtx *SlideContext, minuteKey string) {
	UpdateContext(slideCtx, minuteKey)
}

func (p *PriorityDiscovery) Reset() {
	p.assignments = make(map[string]SlideResult)
	p.prepared = false
}
