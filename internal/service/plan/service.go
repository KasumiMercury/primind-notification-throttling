package plan

import (
	"context"
	"log/slog"
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
	"github.com/KasumiMercury/primind-notification-throttling/internal/infra/timemgmt"
	"github.com/KasumiMercury/primind-notification-throttling/internal/observability/metrics"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service/lane"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service/sliding"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service/slot"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service/smoothing"
)

type Service struct {
	remindTimeClient  timemgmt.RemindTimeRepository
	throttleRepo      domain.ThrottleRepository
	laneClassifier    *lane.Classifier
	slotCalculator    *slot.Calculator
	slotCounter       slot.Counter
	smoothingStrategy smoothing.Strategy
	slideDiscovery    sliding.Discovery
	throttleMetrics   *metrics.ThrottleMetrics
	capPerMinute      int
}

func NewService(
	remindTimeClient timemgmt.RemindTimeRepository,
	throttleRepo domain.ThrottleRepository,
	laneClassifier *lane.Classifier,
	slotCalculator *slot.Calculator,
	slotCounter slot.Counter,
	smoothingStrategy smoothing.Strategy,
	slideDiscovery sliding.Discovery,
	throttleMetrics *metrics.ThrottleMetrics,
	capPerMinute int,
) *Service {
	return &Service{
		remindTimeClient:  remindTimeClient,
		throttleRepo:      throttleRepo,
		laneClassifier:    laneClassifier,
		slotCalculator:    slotCalculator,
		slotCounter:       slotCounter,
		smoothingStrategy: smoothingStrategy,
		slideDiscovery:    slideDiscovery,
		throttleMetrics:   throttleMetrics,
		capPerMinute:      capPerMinute,
	}
}

func (s *Service) PlanReminds(ctx context.Context, start, end time.Time, runID string) (*Response, error) {
	// Fetch reminds in the planning window
	remindsResp, err := s.remindTimeClient.GetRemindsByTimeRange(ctx, start, end, runID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to fetch reminds for planning",
			slog.String("error", err.Error()),
			slog.Time("start", start),
			slog.Time("end", end),
		)
		return nil, err
	}

	slog.DebugContext(ctx, "fetched reminds for planning",
		slog.Int("total_count", remindsResp.Count),
		slog.Time("start", start),
		slog.Time("end", end),
	)

	// Filter unthrottled reminds
	unthrottledReminds := filterUnthrottled(remindsResp.Reminds)

	slog.DebugContext(ctx, "filtered unthrottled reminds for planning",
		slog.Int("unthrottled_count", len(unthrottledReminds)),
	)

	// Filter already committed and collect active reminds
	activeReminds, skippedResults, skippedCount := s.filterAlreadyProcessed(ctx, unthrottledReminds)

	allCountByMinute := s.organizeByMinute(unthrottledReminds)
	currentByMinute := s.getCurrentCountsByMinute(ctx, start, end)

	// Calculate smoothing targets based on all unthrottled reminds
	smoothingInput := smoothing.SmoothingInput{
		TotalCount:      len(unthrottledReminds),
		CountByMinute:   allCountByMinute,
		CurrentByMinute: currentByMinute,
		CapPerMinute:    s.capPerMinute,
	}

	var targets []smoothing.TargetAllocation
	if s.smoothingStrategy != nil {
		targets, err = s.smoothingStrategy.CalculateTargets(ctx, start, end, smoothingInput)
		if err != nil {
			slog.WarnContext(ctx, "failed to calculate smoothing targets, using passthrough",
				slog.String("error", err.Error()),
			)
			targets = s.buildPassthroughTargets(start, end, allCountByMinute, currentByMinute)
		}
	} else {
		targets = s.buildPassthroughTargets(start, end, allCountByMinute, currentByMinute)
	}

	// Classify into strict/loose lanes
	var strictReminds, looseReminds []timemgmt.RemindResponse
	for _, remind := range activeReminds {
		classifiedLane := s.laneClassifier.Classify(remind)
		if s.throttleMetrics != nil {
			s.throttleMetrics.RecordLaneDistribution(ctx, classifiedLane.String())
		}
		if classifiedLane.IsStrict() {
			strictReminds = append(strictReminds, remind)
		} else {
			looseReminds = append(looseReminds, remind)
		}
	}

	slog.DebugContext(ctx, "classified reminds by lane",
		slog.Int("strict_count", len(strictReminds)),
		slog.Int("loose_count", len(looseReminds)),
		slog.Int("skipped_count", skippedCount),
	)

	// Create slide context with targets
	slideCtx := &sliding.SlideContext{
		Targets:      targets,
		CapPerMinute: s.capPerMinute,
	}

	results := make([]ResultItem, 0, len(activeReminds))
	results = append(results, skippedResults...)
	plannedCount := 0
	shiftedCount := 0

	// Track plans by minute key
	plansByMinute := make(map[string]*domain.Plan)

	// Process loose lane
	for _, remind := range looseReminds {
		result := s.processRemind(ctx, remind, domain.LaneLoose, slideCtx, plansByMinute)
		if result.WasShifted {
			shiftedCount++
		}
		plannedCount++
		results = append(results, result)
	}

	// Process strict lane
	for _, remind := range strictReminds {
		result := s.processRemind(ctx, remind, domain.LaneStrict, slideCtx, plansByMinute)
		if result.WasShifted {
			shiftedCount++
		}
		plannedCount++
		results = append(results, result)
	}

	// Save plans to Redis
	if s.throttleRepo != nil {
		for _, plan := range plansByMinute {
			if err := s.throttleRepo.SavePlan(ctx, plan); err != nil {
				slog.WarnContext(ctx, "failed to save plan",
					slog.String("minute_key", plan.MinuteKey),
					slog.String("error", err.Error()),
				)
			}
		}
	}

	slog.InfoContext(ctx, "planning phase completed",
		slog.Int("planned_count", plannedCount),
		slog.Int("skipped_count", skippedCount),
		slog.Int("shifted_count", shiftedCount),
	)

	smoothingTargets := make([]SmoothingTarget, len(targets))
	for i, t := range targets {
		smoothingTargets[i] = SmoothingTarget{
			MinuteKey:  t.MinuteKey,
			MinuteTime: t.MinuteTime,
			Target:     t.Target,
		}
	}

	return &Response{
		PlannedCount:     plannedCount,
		SkippedCount:     skippedCount,
		ShiftedCount:     shiftedCount,
		Results:          results,
		SmoothingTargets: smoothingTargets,
	}, nil
}

func (s *Service) filterAlreadyProcessed(
	ctx context.Context,
	reminds []timemgmt.RemindResponse,
) (active []timemgmt.RemindResponse, skipped []ResultItem, skippedCount int) {
	skipped = make([]ResultItem, 0)
	active = make([]timemgmt.RemindResponse, 0, len(reminds))

	for _, remind := range reminds {
		classifiedLane := s.laneClassifier.Classify(remind)

		if s.throttleRepo != nil {
			committed, err := s.throttleRepo.IsPacketCommitted(ctx, remind.ID)
			if err != nil {
				slog.WarnContext(ctx, "failed to check packet committed status during planning",
					slog.String("remind_id", remind.ID),
					slog.String("error", err.Error()),
				)
			} else if committed {
				slog.DebugContext(ctx, "skipping already committed remind in planning",
					slog.String("remind_id", remind.ID),
				)
				skipped = append(skipped, ResultItem{
					RemindID:     remind.ID,
					TaskID:       remind.TaskID,
					TaskType:     remind.TaskType,
					Lane:         classifiedLane,
					OriginalTime: remind.Time,
					PlannedTime:  remind.Time,
					Skipped:      true,
					SkipReason:   "already committed",
				})
				skippedCount++
				if s.throttleMetrics != nil {
					s.throttleMetrics.RecordPacketProcessed(ctx, "plan", classifiedLane.String(), "skipped")
				}
				continue
			}
		}

		active = append(active, remind)
	}

	return active, skipped, skippedCount
}

// organizeByMinute organizes reminds by minute key.
func (s *Service) organizeByMinute(reminds []timemgmt.RemindResponse) map[string]int {
	countByMinute := make(map[string]int)
	for _, remind := range reminds {
		key := domain.MinuteKey(remind.Time)
		countByMinute[key]++
	}
	return countByMinute
}

// getCurrentCountsByMinute gets current counts (committed + planned) for each minute.
func (s *Service) getCurrentCountsByMinute(ctx context.Context, start, end time.Time) map[string]int {
	currentByMinute := make(map[string]int)
	if s.slotCounter == nil {
		return currentByMinute
	}

	for minute := start.UTC().Truncate(time.Minute); minute.Before(end); minute = minute.Add(time.Minute) {
		key := domain.MinuteKey(minute)
		count, err := s.slotCounter.GetTotalCountForMinute(ctx, key)
		if err != nil {
			slog.WarnContext(ctx, "failed to get current count for minute",
				slog.String("minute_key", key),
				slog.String("error", err.Error()),
			)
		}
		currentByMinute[key] = count
	}

	return currentByMinute
}

// buildPassthroughTargets builds targets without smoothing (passthrough).
func (s *Service) buildPassthroughTargets(
	start, end time.Time,
	countByMinute, currentByMinute map[string]int,
) []smoothing.TargetAllocation {
	window := smoothing.NewTimeWindow(start, end)
	if window == nil {
		return nil
	}

	allocations := make([]smoothing.TargetAllocation, window.NumMinutes)
	for i := 0; i < window.NumMinutes; i++ {
		key := window.MinuteKeys[i]

		target := 0
		if count, ok := countByMinute[key]; ok {
			target = count
		}

		currentCount := 0
		if count, ok := currentByMinute[key]; ok {
			currentCount = count
		}

		available := target - currentCount
		if s.capPerMinute > 0 && currentCount+available > s.capPerMinute {
			available = s.capPerMinute - currentCount
		}
		if available < 0 {
			available = 0
		}

		allocations[i] = smoothing.TargetAllocation{
			MinuteKey:    key,
			MinuteTime:   window.MinuteTimes[i],
			Target:       target,
			CurrentCount: currentCount,
			Available:    available,
		}
	}

	return allocations
}

// processRemind processes a single remind and adds it to the plan.
func (s *Service) processRemind(
	ctx context.Context,
	remind timemgmt.RemindResponse,
	classifiedLane domain.Lane,
	slideCtx *sliding.SlideContext,
	plansByMinute map[string]*domain.Plan,
) ResultItem {
	result := ResultItem{
		RemindID:     remind.ID,
		TaskID:       remind.TaskID,
		TaskType:     remind.TaskType,
		Lane:         classifiedLane,
		OriginalTime: remind.Time,
		PlannedTime:  remind.Time,
	}

	var slideResult sliding.SlideResult

	if s.slideDiscovery != nil {
		if classifiedLane.IsStrict() {
			slideResult = s.slideDiscovery.FindSlotForStrict(ctx, remind, slideCtx)
		} else {
			slideResult = s.slideDiscovery.FindSlotForLoose(ctx, remind, slideCtx)
		}
		s.slideDiscovery.UpdateContext(slideCtx, domain.MinuteKey(slideResult.PlannedTime))
	} else {
		// Fall back to existing SlotCalculator
		plannedTime, wasShifted := s.slotCalculator.FindSlot(
			ctx,
			remind.Time,
			int(remind.SlideWindowWidth),
			classifiedLane,
		)
		slideResult = sliding.SlideResult{
			PlannedTime: plannedTime,
			WasShifted:  wasShifted,
		}
	}

	result.PlannedTime = slideResult.PlannedTime
	result.WasShifted = slideResult.WasShifted

	if slideResult.WasShifted {
		if s.throttleMetrics != nil {
			s.throttleMetrics.RecordPacketShifted(ctx, "plan", classifiedLane.String())
		}
		slog.DebugContext(ctx, "remind shifted",
			slog.String("remind_id", remind.ID),
			slog.String("lane", classifiedLane.String()),
			slog.Time("original_time", remind.Time),
			slog.Time("planned_time", slideResult.PlannedTime),
			slog.Duration("shift", slideResult.PlannedTime.Sub(remind.Time)),
		)
	}

	// Add to plan
	minuteKey := domain.MinuteKey(slideResult.PlannedTime)
	if _, ok := plansByMinute[minuteKey]; !ok {
		plansByMinute[minuteKey] = domain.NewPlan(minuteKey)
	}
	plansByMinute[minuteKey].AddPacket(domain.NewPlannedPacket(
		remind.ID,
		remind.Time,
		slideResult.PlannedTime,
		classifiedLane,
	))

	if s.throttleMetrics != nil {
		s.throttleMetrics.RecordPacketProcessed(ctx, "plan", classifiedLane.String(), "success")
	}

	return result
}

func filterUnthrottled(reminds []timemgmt.RemindResponse) []timemgmt.RemindResponse {
	filtered := make([]timemgmt.RemindResponse, 0, len(reminds))
	for _, remind := range reminds {
		if !remind.Throttled {
			filtered = append(filtered, remind)
		}
	}
	return filtered
}
