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

	// Merge with previous plans
	mergeResult, err := s.mergeWithPreviousPlans(ctx, activeReminds, start, end)
	if err != nil {
		slog.WarnContext(ctx, "failed to merge with previous plans",
			slog.String("error", err.Error()),
		)

		// Fall back to treating all as new
		mergeResult = &MergeResult{
			Notifications: make([]MergedNotification, len(activeReminds)),
			NewCount:      len(activeReminds),
		}
		for i, remind := range activeReminds {
			mergeResult.Notifications[i] = MergedNotification{
				Remind: remind,
				IsNew:  true,
			}
		}
	}

	slog.DebugContext(ctx, "merged notifications with previous plans",
		slog.Int("new_count", mergeResult.NewCount),
		slog.Int("previously_planned_count", mergeResult.PreviouslyPlannedCount),
	)

	allCountByMinute := s.organizeByMinuteFromMerged(mergeResult.Notifications)
	previouslyPlannedByMinute := s.countPreviouslyPlannedByMinute(mergeResult.Notifications)
	currentByMinute := s.getCurrentCountsByMinuteExcludingReplanned(ctx, start, end, previouslyPlannedByMinute)

	// Build radius batches for optimization strategy
	radiusBatches := s.buildRadiusBatches(start, end, mergeResult.Notifications)

	// Get previous planned count for start minute (only for notifications still in scope)
	startValue := s.getStartMinuteTarget(ctx, start, previouslyPlannedByMinute)

	// Calculate smoothing targets based on merged notifications
	smoothingInput := smoothing.SmoothingInput{
		TotalCount:      len(mergeResult.Notifications),
		CountByMinute:   allCountByMinute,
		CurrentByMinute: currentByMinute,
		CapPerMinute:    s.capPerMinute,
		RadiusBatches:   radiusBatches,
		StartValue:      startValue,
	}

	slog.DebugContext(ctx, "smoothing input",
		slog.Int("total_count", smoothingInput.TotalCount),
		slog.Any("count_by_minute", allCountByMinute),
		slog.Int("cap_per_minute", s.capPerMinute),
	)

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

	// Create slide context with targets
	slideCtx := &sliding.SlideContext{
		Targets:      targets,
		CapPerMinute: s.capPerMinute,
	}

	results := make([]ResultItem, 0, len(mergeResult.Notifications))
	results = append(results, skippedResults...)
	plannedCount := 0
	shiftedCount := 0

	// Track plans by minute key
	plansByMinute := make(map[string]*domain.Plan)

	looseNotifications, strictNotifications := s.separateByLane(mergeResult.Notifications)

	slog.DebugContext(ctx, "separated notifications by lane",
		slog.Int("loose_count", len(looseNotifications)),
		slog.Int("strict_count", len(strictNotifications)),
	)

	if s.slideDiscovery != nil && s.slideDiscovery.SupportsBatch() {
		// Batch processing
		looseItems, strictItems := s.collectPriorityItems(looseNotifications, strictNotifications)

		if err := s.slideDiscovery.PrepareSlots(ctx, looseItems, strictItems, slideCtx); err != nil {
			slog.WarnContext(ctx, "failed to prepare batch slots, falling back to individual processing",
				slog.String("error", err.Error()),
			)
		} else {
			slog.DebugContext(ctx, "batch slots prepared",
				slog.Int("loose_items", len(looseItems)),
				slog.Int("strict_items", len(strictItems)),
			)
		}
	}

	// Process Loose notifications
	for _, merged := range looseNotifications {
		var result ResultItem
		if !merged.IsNew {
			result = s.processPreviouslyPlanned(ctx, merged, slideCtx, plansByMinute)
		} else {
			if s.throttleMetrics != nil {
				s.throttleMetrics.RecordLaneDistribution(ctx, domain.LaneLoose.String())
			}
			result = s.processRemind(ctx, merged.Remind, domain.LaneLoose, slideCtx, plansByMinute)
		}
		if result.WasShifted {
			shiftedCount++
		}
		plannedCount++
		results = append(results, result)
	}

	// Process Strict notifications
	for _, merged := range strictNotifications {
		var result ResultItem
		if !merged.IsNew {
			result = s.processPreviouslyPlanned(ctx, merged, slideCtx, plansByMinute)
		} else {
			if s.throttleMetrics != nil {
				s.throttleMetrics.RecordLaneDistribution(ctx, domain.LaneStrict.String())
			}
			result = s.processRemind(ctx, merged.Remind, domain.LaneStrict, slideCtx, plansByMinute)
		}
		if result.WasShifted {
			shiftedCount++
		}
		plannedCount++
		results = append(results, result)
	}

	slog.DebugContext(ctx, "processed notifications",
		slog.Int("previously_planned_count", mergeResult.PreviouslyPlannedCount),
		slog.Int("new_count", mergeResult.NewCount),
	)

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

// processPreviouslyPlanned handles a notification that was already planned.
func (s *Service) processPreviouslyPlanned(
	ctx context.Context,
	merged MergedNotification,
	slideCtx *sliding.SlideContext,
	plansByMinute map[string]*domain.Plan,
) ResultItem {
	prevPlan := merged.PreviousPlan
	remind := merged.Remind

	result := ResultItem{
		RemindID:     remind.ID,
		TaskID:       remind.TaskID,
		TaskType:     remind.TaskType,
		Lane:         prevPlan.Lane,
		OriginalTime: remind.Time,
		PlannedTime:  prevPlan.PlannedTime,
		WasShifted:   prevPlan.WasShifted(),
	}

	// Check if previous PlannedTime minute has available capacity
	prevMinuteKey := domain.MinuteKey(prevPlan.PlannedTime)
	hasCapacity := s.hasAvailableCapacity(slideCtx, prevMinuteKey)

	if hasCapacity {
		// Use previous planned time
		slog.DebugContext(ctx, "preserving previously planned time",
			slog.String("remind_id", remind.ID),
			slog.Time("original_time", remind.Time),
			slog.Time("planned_time", prevPlan.PlannedTime),
			slog.Bool("was_shifted", result.WasShifted),
		)

		// Update context to account for this slot
		if s.slideDiscovery != nil {
			s.slideDiscovery.UpdateContext(slideCtx, prevMinuteKey)
		}
	} else {
		// Find a new slot
		var slideResult sliding.SlideResult

		if s.slideDiscovery != nil {
			if prevPlan.Lane.IsStrict() {
				slideResult = s.slideDiscovery.FindSlotForStrict(ctx, remind, slideCtx)
			} else {
				slideResult = s.slideDiscovery.FindSlotForLoose(ctx, remind, slideCtx)
			}
			s.slideDiscovery.UpdateContext(slideCtx, domain.MinuteKey(slideResult.PlannedTime))
		} else if s.slotCalculator != nil {
			// Fall back to existing SlotCalculator
			plannedTime, wasShifted := s.slotCalculator.FindSlot(
				ctx,
				remind.Time,
				int(remind.SlideWindowWidth),
			)
			slideResult = sliding.SlideResult{
				PlannedTime: plannedTime,
				WasShifted:  wasShifted,
			}
		} else {
			// No calculator available, use previous planned time anyway
			slideResult = sliding.SlideResult{
				PlannedTime: prevPlan.PlannedTime,
				WasShifted:  prevPlan.WasShifted(),
			}
		}

		result.PlannedTime = slideResult.PlannedTime
		result.WasShifted = slideResult.WasShifted

		slog.DebugContext(ctx, "re-planned previously planned notification due to capacity",
			slog.String("remind_id", remind.ID),
			slog.Time("original_time", remind.Time),
			slog.Time("previous_planned_time", prevPlan.PlannedTime),
			slog.Time("new_planned_time", slideResult.PlannedTime),
		)
	}

	// Record metrics
	if s.throttleMetrics != nil {
		s.throttleMetrics.RecordLaneDistribution(ctx, prevPlan.Lane.String())
		if result.WasShifted {
			s.throttleMetrics.RecordPacketShifted(ctx, "plan", prevPlan.Lane.String())
		}
		s.throttleMetrics.RecordPacketProcessed(ctx, "plan", prevPlan.Lane.String(), "success")
	}

	// Add to plan
	minuteKey := domain.MinuteKey(result.PlannedTime)
	if _, ok := plansByMinute[minuteKey]; !ok {
		plansByMinute[minuteKey] = domain.NewPlan(minuteKey)
	}
	plansByMinute[minuteKey].AddPacket(domain.NewPlannedPacket(
		remind.ID,
		remind.Time,
		result.PlannedTime,
		prevPlan.Lane,
	))

	return result
}

// hasAvailableCapacity checks if the given minute has available capacity in the slide context.
func (s *Service) hasAvailableCapacity(slideCtx *sliding.SlideContext, minuteKey string) bool {
	if slideCtx == nil || slideCtx.Targets == nil {
		return true
	}

	for _, target := range slideCtx.Targets {
		if target.MinuteKey == minuteKey {
			return target.Available > 0
		}
	}

	// Minute not in targets (outside planning window), use cap check
	if slideCtx.CapPerMinute > 0 {
		// For minutes outside the target window, we allow if cap is not reached
		return true
	}

	return true
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

// mergeWithPreviousPlans merges API notifications with previously planned packets.
func (s *Service) mergeWithPreviousPlans(
	ctx context.Context,
	reminds []timemgmt.RemindResponse,
	start, end time.Time,
) (*MergeResult, error) {
	result := &MergeResult{
		Notifications: make([]MergedNotification, 0, len(reminds)),
	}

	// If no repository, treat all as new
	if s.throttleRepo == nil {
		for _, remind := range reminds {
			result.Notifications = append(result.Notifications, MergedNotification{
				Remind: remind,
				IsNew:  true,
			})
			result.NewCount++
		}
		return result, nil
	}

	// Fetch previous plans in range
	startKey := domain.MinuteKey(start)
	endKey := domain.MinuteKey(end)
	previousPlans, err := s.throttleRepo.GetPlansInRange(ctx, startKey, endKey)
	if err != nil {
		slog.WarnContext(ctx, "failed to fetch previous plans, treating all as new",
			slog.String("error", err.Error()),
		)

		// Fall back to treating all as new
		for _, remind := range reminds {
			result.Notifications = append(result.Notifications, MergedNotification{
				Remind: remind,
				IsNew:  true,
			})
			result.NewCount++
		}
		return result, nil
	}

	previousPacketByID := make(map[string]*domain.PlannedPacket)
	for _, plan := range previousPlans {
		for i := range plan.PlannedPackets {
			packet := &plan.PlannedPackets[i]
			previousPacketByID[packet.RemindID] = packet
		}
	}

	for _, remind := range reminds {
		merged := MergedNotification{
			Remind: remind,
		}

		if prevPacket, exists := previousPacketByID[remind.ID]; exists {
			merged.PreviousPlan = prevPacket
			merged.IsNew = false
			result.PreviouslyPlannedCount++
		} else {
			merged.IsNew = true
			result.NewCount++
		}

		result.Notifications = append(result.Notifications, merged)
	}

	slog.DebugContext(ctx, "merged API notifications with previous plans",
		slog.Int("new_count", result.NewCount),
		slog.Int("previously_planned_count", result.PreviouslyPlannedCount),
	)

	return result, nil
}

// organizeByMinuteFromMerged organizes merged notifications by minute key.
// For previously-planned notifications, uses the PlannedTime minute.
// For new notifications, uses the OriginalTime minute.
func (s *Service) organizeByMinuteFromMerged(merged []MergedNotification) map[string]int {
	countByMinute := make(map[string]int)
	for _, m := range merged {
		var key string
		if m.PreviousPlan != nil {
			// Use previous planned time for already-planned notifications
			key = domain.MinuteKey(m.PreviousPlan.PlannedTime)
		} else {
			// Use original time for new notifications
			key = domain.MinuteKey(m.Remind.Time)
		}
		countByMinute[key]++
	}
	return countByMinute
}

// countPreviouslyPlannedByMinute counts previously planned packets by their planned minute.
func (s *Service) countPreviouslyPlannedByMinute(notifications []MergedNotification) map[string]int {
	counts := make(map[string]int)
	for _, n := range notifications {
		if n.PreviousPlan != nil {
			key := domain.MinuteKey(n.PreviousPlan.PlannedTime)
			counts[key]++
		}
	}
	return counts
}

// getCurrentCountsByMinuteExcludingReplanned gets current counts for each minute,
func (s *Service) getCurrentCountsByMinuteExcludingReplanned(
	ctx context.Context,
	start, end time.Time,
	previouslyPlannedByMinute map[string]int,
) map[string]int {
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

		// Subtract previously planned packets that will be re-processed
		if exclude, ok := previouslyPlannedByMinute[key]; ok {
			count -= exclude
			if count < 0 {
				count = 0
			}
		}
		currentByMinute[key] = count
	}

	return currentByMinute
}

func (s *Service) separateByLane(notifications []MergedNotification) (loose, strict []MergedNotification) {
	loose = make([]MergedNotification, 0, len(notifications))
	strict = make([]MergedNotification, 0, len(notifications))

	for _, n := range notifications {
		var classifiedLane domain.Lane
		if n.PreviousPlan != nil {
			classifiedLane = n.PreviousPlan.Lane
		} else {
			classifiedLane = s.laneClassifier.Classify(n.Remind)
		}

		if classifiedLane.IsLoose() {
			loose = append(loose, n)
		} else {
			strict = append(strict, n)
		}
	}
	return
}

func (s *Service) collectPriorityItems(
	looseNotifications []MergedNotification,
	strictNotifications []MergedNotification,
) ([]*sliding.PriorityItem, []*sliding.PriorityItem) {
	looseItems := make([]*sliding.PriorityItem, 0, len(looseNotifications))
	strictItems := make([]*sliding.PriorityItem, 0, len(strictNotifications))

	for _, n := range looseNotifications {
		if n.IsNew {
			looseItems = append(looseItems, sliding.NewPriorityItem(n.Remind, domain.LaneLoose))
		}
	}

	for _, n := range strictNotifications {
		if n.IsNew {
			strictItems = append(strictItems, sliding.NewPriorityItem(n.Remind, domain.LaneStrict))
		}
	}

	return looseItems, strictItems
}

// buildRadiusBatches builds RadiusBatch slices for the optimization strategy.
// Groups notifications by (minuteKey, radius) and calculates the relative minute index.
func (s *Service) buildRadiusBatches(
	start, end time.Time,
	notifications []MergedNotification,
) []smoothing.RadiusBatch {
	window := smoothing.NewTimeWindow(start, end)
	if window == nil || len(notifications) == 0 {
		return nil
	}

	// Create a map from minuteKey to relative minute index
	minuteKeyToIndex := make(map[string]int)
	for i, key := range window.MinuteKeys {
		minuteKeyToIndex[key] = i
	}

	// Group by (minuteKey, radius)
	type groupKey struct {
		minuteKey string
		radius    int
	}
	groups := make(map[groupKey]int)

	// Calculate last valid minute in window (end is exclusive)
	lastMinute := end.Add(-time.Minute)

	for _, n := range notifications {
		// Determine the time to use for minute key
		var t time.Time
		if n.PreviousPlan != nil {
			t = n.PreviousPlan.PlannedTime
		} else {
			t = n.Remind.Time
		}

		// Clamp time to window bounds to ensure all notifications are included
		// This handles notifications that were planned outside the current window
		if t.Before(start) {
			t = start
		} else if !t.Before(end) {
			t = lastMinute
		}

		minuteKey := domain.MinuteKey(t)

		// Calculate radius from SlideWindowWidth (seconds) to minutes.
		// SlideWindowWidth is the half-width (one side); radius is that width in minutes.
		radius := max(int(n.Remind.SlideWindowWidth)/60, 0)

		key := groupKey{minuteKey: minuteKey, radius: radius}
		groups[key]++
	}

	// Convert groups to RadiusBatch slice
	// All minuteKeys are now guaranteed to be within the window due to clamping
	batches := make([]smoothing.RadiusBatch, 0, len(groups))
	for key, count := range groups {
		minuteIdx := minuteKeyToIndex[key.minuteKey]

		batches = append(batches, smoothing.RadiusBatch{
			MinuteKey: key.minuteKey,
			Minute:    minuteIdx,
			Count:     count,
			Radius:    key.radius,
		})
	}

	return batches
}

// getStartMinuteTarget retrieves the previously planned count for the start minute.
// Returns nil if no previous plan exists for that minute.
func (s *Service) getStartMinuteTarget(ctx context.Context, start time.Time, previouslyPlannedByMinute map[string]int) *int {
	if len(previouslyPlannedByMinute) == 0 {
		return nil
	}

	startKey := domain.MinuteKey(start)
	if count, ok := previouslyPlannedByMinute[startKey]; ok && count > 0 {
		slog.DebugContext(ctx, "using previous planned count as start value",
			slog.String("minute_key", startKey),
			slog.Int("start_value", count),
		)
		return &count
	}

	return nil
}
