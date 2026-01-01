package plan

import (
	"context"
	"log/slog"
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
	"github.com/KasumiMercury/primind-notification-throttling/internal/infra/timemgmt"
	"github.com/KasumiMercury/primind-notification-throttling/internal/observability/metrics"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service/lane"
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

	// Separate Strict and Loose reminds
	var strictReminds, looseReminds []timemgmt.RemindResponse
	skippedResults := make([]ResultItem, 0)
	skippedCount := 0

	for _, remind := range unthrottledReminds {
		classifiedLane := s.laneClassifier.Classify(remind)

		// Record lane distribution for planning phase
		if s.throttleMetrics != nil {
			s.throttleMetrics.RecordLaneDistribution(ctx, classifiedLane.String())
		}

		// Check if already committed or planned
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
				skippedResults = append(skippedResults, ResultItem{
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

			_, err = s.throttleRepo.GetPlannedPacket(ctx, remind.ID)
			if err == nil {
				slog.DebugContext(ctx, "skipping already planned remind",
					slog.String("remind_id", remind.ID),
				)
				skippedResults = append(skippedResults, ResultItem{
					RemindID:     remind.ID,
					TaskID:       remind.TaskID,
					TaskType:     remind.TaskType,
					Lane:         classifiedLane,
					OriginalTime: remind.Time,
					PlannedTime:  remind.Time,
					Skipped:      true,
					SkipReason:   "already planned",
				})
				skippedCount++
				if s.throttleMetrics != nil {
					s.throttleMetrics.RecordPacketProcessed(ctx, "plan", classifiedLane.String(), "skipped")
				}
				continue
			}
		}

		// Classify into Strict or Loose
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

	results := make([]ResultItem, 0, len(strictReminds)+len(looseReminds))
	results = append(results, skippedResults...)
	plannedCount := 0
	shiftedCount := 0

	// Track plans by minute key
	plansByMinute := make(map[string]*domain.Plan)

	// Process Strict lane
	for _, remind := range strictReminds {
		classifiedLane := domain.LaneStrict
		result := ResultItem{
			RemindID:     remind.ID,
			TaskID:       remind.TaskID,
			TaskType:     remind.TaskType,
			Lane:         classifiedLane,
			OriginalTime: remind.Time,
			PlannedTime:  remind.Time,
		}

		plannedTime, wasShifted := s.slotCalculator.FindSlot(
			ctx,
			remind.Time,
			int(remind.SlideWindowWidth),
			classifiedLane,
		)
		result.PlannedTime = plannedTime
		result.WasShifted = wasShifted

		if wasShifted {
			shiftedCount++
			if s.throttleMetrics != nil {
				s.throttleMetrics.RecordPacketShifted(ctx, "plan", classifiedLane.String())
			}
			slog.DebugContext(ctx, "strict remind shifted",
				slog.String("remind_id", remind.ID),
				slog.Time("original_time", remind.Time),
				slog.Time("planned_time", plannedTime),
				slog.Duration("shift", plannedTime.Sub(remind.Time)),
			)
		}

		minuteKey := domain.MinuteKey(plannedTime)
		if _, ok := plansByMinute[minuteKey]; !ok {
			plansByMinute[minuteKey] = domain.NewPlan(minuteKey)
		}
		plansByMinute[minuteKey].AddPacket(domain.NewPlannedPacket(
			remind.ID,
			remind.Time,
			plannedTime,
			classifiedLane,
		))

		if s.throttleRepo != nil {
			if err := s.throttleRepo.SavePlan(ctx, plansByMinute[minuteKey]); err != nil {
				slog.WarnContext(ctx, "failed to save plan during planning",
					slog.String("minute_key", minuteKey),
					slog.String("error", err.Error()),
				)
			}
		}

		plannedCount++
		if s.throttleMetrics != nil {
			s.throttleMetrics.RecordPacketProcessed(ctx, "plan", classifiedLane.String(), "success")
		}
		results = append(results, result)
	}

	// Process Loose lane
	if len(looseReminds) > 0 && s.smoothingStrategy != nil {
		// Build strictByMinute map from processed strict reminds
		strictByMinute := make(map[string]int)
		for _, result := range results {
			if result.Lane.IsStrict() && !result.Skipped {
				minuteKey := domain.MinuteKey(result.PlannedTime)
				strictByMinute[minuteKey]++
			}
		}

		// Build currentByMinute map for each minute in the window
		currentByMinute := make(map[string]int)
		if s.slotCounter != nil {
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
		}

		// Build AllocationInput
		allocationInput := smoothing.AllocationInput{
			StrictCount:     len(strictReminds),
			LooseCount:      len(looseReminds),
			StrictByMinute:  strictByMinute,
			CurrentByMinute: currentByMinute,
			CapPerMinute:    s.capPerMinute,
		}

		// Calculate allocations using the smoothing strategy with context
		allocations, err := s.smoothingStrategy.CalculateAllocationsWithContext(ctx, start, end, allocationInput)
		if err != nil {
			slog.WarnContext(ctx, "failed to calculate smoothing allocations, falling back to default",
				slog.String("error", err.Error()),
			)
			// Fall back to existing behavior
			allocations = nil
		}

		for _, remind := range looseReminds {
			classifiedLane := domain.LaneLoose
			result := ResultItem{
				RemindID:     remind.ID,
				TaskID:       remind.TaskID,
				TaskType:     remind.TaskType,
				Lane:         classifiedLane,
				OriginalTime: remind.Time,
				PlannedTime:  remind.Time,
			}

			var plannedTime time.Time
			var wasShifted bool

			if allocations != nil {
				plannedTime, wasShifted = s.smoothingStrategy.FindBestSlot(
					remind.Time,
					int(remind.SlideWindowWidth),
					allocations,
				)
				smoothing.UpdateAllocation(allocations, domain.MinuteKey(plannedTime))
			} else {
				// Fall back to existing SlotCalculator
				plannedTime, wasShifted = s.slotCalculator.FindSlot(
					ctx,
					remind.Time,
					int(remind.SlideWindowWidth),
					classifiedLane,
				)
			}

			result.PlannedTime = plannedTime
			result.WasShifted = wasShifted

			if wasShifted {
				shiftedCount++
				if s.throttleMetrics != nil {
					s.throttleMetrics.RecordPacketShifted(ctx, "plan", classifiedLane.String())
				}
				slog.DebugContext(ctx, "loose remind shifted (smoothing)",
					slog.String("remind_id", remind.ID),
					slog.Time("original_time", remind.Time),
					slog.Time("planned_time", plannedTime),
					slog.Duration("shift", plannedTime.Sub(remind.Time)),
				)
			}

			// Save to plan
			minuteKey := domain.MinuteKey(plannedTime)
			if _, ok := plansByMinute[minuteKey]; !ok {
				plansByMinute[minuteKey] = domain.NewPlan(minuteKey)
			}
			plansByMinute[minuteKey].AddPacket(domain.NewPlannedPacket(
				remind.ID,
				remind.Time,
				plannedTime,
				classifiedLane,
			))

			if s.throttleRepo != nil {
				if err := s.throttleRepo.SavePlan(ctx, plansByMinute[minuteKey]); err != nil {
					slog.WarnContext(ctx, "failed to save plan during planning",
						slog.String("minute_key", minuteKey),
						slog.String("error", err.Error()),
					)
				}
			}

			plannedCount++
			if s.throttleMetrics != nil {
				s.throttleMetrics.RecordPacketProcessed(ctx, "plan", classifiedLane.String(), "success")
			}
			results = append(results, result)
		}
	}

	slog.InfoContext(ctx, "planning phase completed",
		slog.Int("planned_count", plannedCount),
		slog.Int("skipped_count", skippedCount),
		slog.Int("shifted_count", shiftedCount),
	)

	return &Response{
		PlannedCount: plannedCount,
		SkippedCount: skippedCount,
		ShiftedCount: shiftedCount,
		Results:      results,
	}, nil
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
