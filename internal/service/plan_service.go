package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/client"
	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
)

type PlanService struct {
	remindTimeClient  client.RemindTimeRepository
	throttleRepo      domain.ThrottleRepository
	laneClassifier    *LaneClassifier
	slotCalculator    *SlotCalculator
	smoothingStrategy SmoothingStrategy
}

func NewPlanService(
	remindTimeClient client.RemindTimeRepository,
	throttleRepo domain.ThrottleRepository,
	laneClassifier *LaneClassifier,
	slotCalculator *SlotCalculator,
	smoothingStrategy SmoothingStrategy,
) *PlanService {
	return &PlanService{
		remindTimeClient:  remindTimeClient,
		throttleRepo:      throttleRepo,
		laneClassifier:    laneClassifier,
		slotCalculator:    slotCalculator,
		smoothingStrategy: smoothingStrategy,
	}
}

func (s *PlanService) PlanReminds(ctx context.Context, start, end time.Time) (*PlanResponse, error) {
	// Fetch reminds in the planning window
	remindsResp, err := s.remindTimeClient.GetRemindsByTimeRange(ctx, start, end)
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
	var strictReminds, looseReminds []client.RemindResponse
	skippedResults := make([]PlanResultItem, 0)
	skippedCount := 0

	for _, remind := range unthrottledReminds {
		lane := s.laneClassifier.Classify(remind)

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
				skippedResults = append(skippedResults, PlanResultItem{
					RemindID:     remind.ID,
					TaskID:       remind.TaskID,
					TaskType:     remind.TaskType,
					Lane:         lane,
					OriginalTime: remind.Time,
					PlannedTime:  remind.Time,
					Skipped:      true,
					SkipReason:   "already committed",
				})
				skippedCount++
				continue
			}

			_, err = s.throttleRepo.GetPlannedPacket(ctx, remind.ID)
			if err == nil {
				slog.DebugContext(ctx, "skipping already planned remind",
					slog.String("remind_id", remind.ID),
				)
				skippedResults = append(skippedResults, PlanResultItem{
					RemindID:     remind.ID,
					TaskID:       remind.TaskID,
					TaskType:     remind.TaskType,
					Lane:         lane,
					OriginalTime: remind.Time,
					PlannedTime:  remind.Time,
					Skipped:      true,
					SkipReason:   "already planned",
				})
				skippedCount++
				continue
			}
		}

		// Classify into Strict or Loose
		if lane.IsStrict() {
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

	results := make([]PlanResultItem, 0, len(strictReminds)+len(looseReminds))
	results = append(results, skippedResults...)
	plannedCount := 0
	shiftedCount := 0

	// Track plans by minute key
	plansByMinute := make(map[string]*domain.Plan)

	// Process Strict lane
	for _, remind := range strictReminds {
		lane := domain.LaneStrict
		result := PlanResultItem{
			RemindID:     remind.ID,
			TaskID:       remind.TaskID,
			TaskType:     remind.TaskType,
			Lane:         lane,
			OriginalTime: remind.Time,
			PlannedTime:  remind.Time,
		}

		plannedTime, wasShifted := s.slotCalculator.FindSlot(
			ctx,
			remind.Time,
			int(remind.SlideWindowWidth),
			lane,
		)
		result.PlannedTime = plannedTime
		result.WasShifted = wasShifted

		if wasShifted {
			shiftedCount++
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
			lane,
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
		results = append(results, result)
	}

	// Process Loose lane
	if len(looseReminds) > 0 && s.smoothingStrategy != nil {
		// Calculate allocations using the smoothing strategy
		allocations, err := s.smoothingStrategy.CalculateAllocations(ctx, start, end, len(looseReminds))
		if err != nil {
			slog.WarnContext(ctx, "failed to calculate smoothing allocations, falling back to default",
				slog.String("error", err.Error()),
			)
			// Fall back to existing behavior
			allocations = nil
		}

		for _, remind := range looseReminds {
			lane := domain.LaneLoose
			result := PlanResultItem{
				RemindID:     remind.ID,
				TaskID:       remind.TaskID,
				TaskType:     remind.TaskType,
				Lane:         lane,
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
				UpdateAllocation(allocations, domain.MinuteKey(plannedTime))
			} else {
				// Fall back to existing SlotCalculator
				plannedTime, wasShifted = s.slotCalculator.FindSlot(
					ctx,
					remind.Time,
					int(remind.SlideWindowWidth),
					lane,
				)
			}

			result.PlannedTime = plannedTime
			result.WasShifted = wasShifted

			if wasShifted {
				shiftedCount++
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
				lane,
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
			results = append(results, result)
		}
	}

	slog.InfoContext(ctx, "planning phase completed",
		slog.Int("planned_count", plannedCount),
		slog.Int("skipped_count", skippedCount),
		slog.Int("shifted_count", shiftedCount),
	)

	return &PlanResponse{
		PlannedCount: plannedCount,
		SkippedCount: skippedCount,
		ShiftedCount: shiftedCount,
		Results:      results,
	}, nil
}
