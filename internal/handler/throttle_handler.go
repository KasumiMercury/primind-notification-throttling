package handler

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/KasumiMercury/primind-notification-throttling/internal/config"
	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
	commonv1 "github.com/KasumiMercury/primind-notification-throttling/internal/gen/common/v1"
	throttlev1 "github.com/KasumiMercury/primind-notification-throttling/internal/gen/throttle/v1"
	"github.com/KasumiMercury/primind-notification-throttling/internal/observability/metrics"
	"github.com/KasumiMercury/primind-notification-throttling/internal/observability/tracing"
	pjson "github.com/KasumiMercury/primind-notification-throttling/internal/proto"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service/plan"
	"github.com/KasumiMercury/primind-notification-throttling/internal/service/throttle"
)

type ThrottleHandler struct {
	throttleService *throttle.Service
	planService     *plan.Service
	config          *config.Config
	throttleMetrics *metrics.ThrottleMetrics
	resultRecorder  domain.ThrottleResultRecorder
}

func NewThrottleHandler(
	throttleService *throttle.Service,
	planService *plan.Service,
	cfg *config.Config,
	throttleMetrics *metrics.ThrottleMetrics,
	resultRecorder domain.ThrottleResultRecorder,
) *ThrottleHandler {
	return &ThrottleHandler{
		throttleService: throttleService,
		planService:     planService,
		config:          cfg,
		throttleMetrics: throttleMetrics,
		resultRecorder:  resultRecorder,
	}
}

func (h *ThrottleHandler) HandleThrottleBatch(c *gin.Context) {
	ctx := c.Request.Context()

	var now time.Time
	if fromStr := c.Query("from"); fromStr != "" {
		parsed, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			respondProtoError(c, http.StatusBadRequest, "invalid from time format, expected RFC3339")
			return
		}
		now = parsed.Truncate(time.Minute)
		slog.InfoContext(ctx, "using virtual time",
			slog.Time("virtual_now", now),
		)
	} else {
		now = time.Now().Truncate(time.Minute)
	}

	var planEndOverride *time.Time
	if planEndStr := c.Query("plan_end"); planEndStr != "" {
		parsed, err := time.Parse(time.RFC3339, planEndStr)
		if err != nil {
			respondProtoError(c, http.StatusBadRequest, "invalid plan_end time format, expected RFC3339")
			return
		}
		t := parsed.Truncate(time.Minute)
		planEndOverride = &t
	}

	var commitEndOverride *time.Time
	if commitEndStr := c.Query("commit_end"); commitEndStr != "" {
		parsed, err := time.Parse(time.RFC3339, commitEndStr)
		if err != nil {
			respondProtoError(c, http.StatusBadRequest, "invalid commit_end time format, expected RFC3339")
			return
		}
		t := parsed.Truncate(time.Minute)
		commitEndOverride = &t
	}

	h.processThrottle(c, ctx, now, planEndOverride, commitEndOverride)
}

func (h *ThrottleHandler) HandleThrottle(c *gin.Context) {
	ctx := c.Request.Context()

	now := time.Now().Truncate(time.Minute)
	h.processThrottle(c, ctx, now, nil, nil)
}

func (h *ThrottleHandler) processThrottle(c *gin.Context, ctx context.Context, now time.Time, planEndOverride *time.Time, commitEndOverride *time.Time) {
	runID := c.GetHeader("X-Run-ID")

	// Capture smoothing targets from planning phase for recording
	var smoothingTargetsForCommit map[string]int

	if h.planService != nil {
		planStart := now
		planEnd := now.Add(time.Duration(h.config.Throttle.PlanningWindowMinutes) * time.Minute)
		if planEndOverride != nil {
			planEnd = *planEndOverride
		}

		slog.InfoContext(ctx, "starting planning phase",
			slog.Time("plan_start", planStart),
			slog.Time("plan_end", planEnd),
		)

		planCtx, planSpan := tracing.StartPlanningPhaseSpan(ctx, planStart, planEnd)
		planStartTime := time.Now()

		planResult, err := h.planService.PlanReminds(planCtx, planStart, planEnd, runID)

		planDuration := time.Since(planStartTime)
		if h.throttleMetrics != nil {
			h.throttleMetrics.RecordPlanningPhaseDuration(planCtx, planDuration)
		}

		if err != nil {
			tracing.RecordPlanningPhaseResult(planSpan, 0, 0, 0, err)
			slog.WarnContext(ctx, "planning phase failed, continuing to commit phase",
				slog.String("error", err.Error()),
			)
			// Continue to commit phase even if planning fails
		} else {
			tracing.RecordPlanningPhaseResult(planSpan, planResult.PlannedCount, planResult.SkippedCount, planResult.ShiftedCount, nil)
			slog.InfoContext(ctx, "planning phase completed",
				slog.Int("planned_count", planResult.PlannedCount),
				slog.Int("skipped_count", planResult.SkippedCount),
				slog.Int("shifted_count", planResult.ShiftedCount),
			)

			// Capture smoothing targets for commit window only (for recording)
			if len(planResult.SmoothingTargets) > 0 {
				commitEnd := now.Add(time.Duration(h.config.Throttle.ConfirmWindowMinutes) * time.Minute)
				if commitEndOverride != nil {
					commitEnd = *commitEndOverride
				}
				smoothingTargetsForCommit = make(map[string]int)
				for _, target := range planResult.SmoothingTargets {
					// Filter to commit window only
					if !target.MinuteTime.Before(now) && target.MinuteTime.Before(commitEnd) {
						key := target.MinuteTime.UTC().Truncate(time.Minute).Format(time.RFC3339)
						smoothingTargetsForCommit[key] = target.Target
					}
				}
			}
		}
		planSpan.End()
	}

	commitStart := now
	commitEnd := now.Add(time.Duration(h.config.Throttle.ConfirmWindowMinutes) * time.Minute)
	if commitEndOverride != nil {
		commitEnd = *commitEndOverride
	}

	slog.InfoContext(ctx, "starting commit phase",
		slog.Time("commit_start", commitStart),
		slog.Time("commit_end", commitEnd),
	)

	commitCtx, commitSpan := tracing.StartCommitPhaseSpan(ctx, commitStart, commitEnd)
	commitStartTime := time.Now()

	result, err := h.throttleService.ProcessReminds(commitCtx, commitStart, commitEnd, runID)

	commitDuration := time.Since(commitStartTime)
	if h.throttleMetrics != nil {
		h.throttleMetrics.RecordCommitPhaseDuration(commitCtx, commitDuration)
	}

	if err != nil {
		tracing.RecordCommitPhaseResult(commitSpan, 0, 0, 0, 0, 0, err)
		commitSpan.End()
		slog.ErrorContext(ctx, "commit phase failed",
			slog.String("error", err.Error()),
		)
		respondProtoError(c, http.StatusInternalServerError, err.Error())
		return
	}

	tracing.RecordCommitPhaseResult(commitSpan, result.ProcessedCount, result.SuccessCount, result.FailedCount, result.SkippedCount, result.ShiftedCount, nil)
	commitSpan.End()

	slog.InfoContext(ctx, "commit phase completed",
		slog.Int("processed", result.ProcessedCount),
		slog.Int("success", result.SuccessCount),
		slog.Int("failed", result.FailedCount),
		slog.Int("skipped", result.SkippedCount),
		slog.Int("shifted", result.ShiftedCount),
	)

	if h.resultRecorder != nil {
		fillAll := h.resultRecorder.FillAllMinutes()
		if len(result.Results) > 0 || fillAll {
			records := h.buildCommitResultRecords(runID, result, commitStart, commitEnd, fillAll, smoothingTargetsForCommit)
			if len(records) > 0 {
				if err := h.resultRecorder.RecordBatchResults(commitCtx, records); err != nil {
					slog.WarnContext(ctx, "failed to record commit phase results",
						slog.String("error", err.Error()),
					)
				}
			}
		}
	}

	respondProtoThrottleResponse(c, http.StatusOK, result)
}

func respondProtoError(c *gin.Context, status int, message string) {
	resp := &throttlev1.ErrorResponse{
		Error:   "processing_error",
		Message: message,
	}

	respBytes, err := pjson.Marshal(resp)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Data(status, "application/json", respBytes)
}

func respondProtoThrottleResponse(c *gin.Context, status int, result *throttle.Response) {
	protoResults := make([]*throttlev1.ThrottleResultItem, 0, len(result.Results))
	for _, r := range result.Results {
		protoResults = append(protoResults, &throttlev1.ThrottleResultItem{
			RemindId:  r.RemindID,
			TaskId:    r.TaskID,
			TaskType:  stringToTaskType(r.TaskType),
			FcmTokens: r.FCMTokens,
			Success:   r.Success,
			Error:     r.Error,
		})
	}

	resp := &throttlev1.ThrottleResponse{
		ProcessedCount: int32(result.ProcessedCount),
		SuccessCount:   int32(result.SuccessCount),
		FailedCount:    int32(result.FailedCount),
		Results:        protoResults,
	}
	respBytes, _ := pjson.Marshal(resp)
	c.Data(status, "application/json", respBytes)
}

// stringToTaskType converts a string to a proto TaskType enum
func stringToTaskType(s string) commonv1.TaskType {
	upper := "TASK_TYPE_" + strings.ToUpper(s)
	if v, ok := commonv1.TaskType_value[upper]; ok {
		return commonv1.TaskType(v)
	}
	return commonv1.TaskType_TASK_TYPE_UNSPECIFIED
}

type minuteDistribution struct {
	beforeCount  int
	afterCount   int
	shiftedCount int
	plannedCount int
	targetCount  int
}

func (h *ThrottleHandler) buildCommitResultRecords(runID string, result *throttle.Response, windowStart, windowEnd time.Time, fillAllMinutes bool, smoothingTargets map[string]int) []domain.ThrottleResultRecord {
	distributions := make(map[string]map[string]*minuteDistribution)

	// Pre-populate all minutes with empty distributions when fillAllMinutes is enabled
	if fillAllMinutes {
		for minute := windowStart.UTC().Truncate(time.Minute); minute.Before(windowEnd); minute = minute.Add(time.Minute) {
			key := minute.Format(time.RFC3339)
			targetCount := 0
			if smoothingTargets != nil {
				if t, ok := smoothingTargets[key]; ok {
					targetCount = t
				}
			}
			distributions[key] = map[string]*minuteDistribution{
				"strict": {targetCount: targetCount},
				"loose":  {targetCount: targetCount},
			}
		}
	}

	for _, item := range result.Results {
		if item.Skipped {
			continue
		}

		lane := item.Lane.String()
		originalMinuteKey := item.OriginalTime.UTC().Truncate(time.Minute).Format(time.RFC3339)
		scheduledMinuteKey := item.ScheduledTime.UTC().Truncate(time.Minute).Format(time.RFC3339)

		if distributions[originalMinuteKey] == nil {
			distributions[originalMinuteKey] = make(map[string]*minuteDistribution)
		}
		if distributions[originalMinuteKey][lane] == nil {
			targetCount := 0
			if smoothingTargets != nil {
				if t, ok := smoothingTargets[originalMinuteKey]; ok {
					targetCount = t
				}
			}
			distributions[originalMinuteKey][lane] = &minuteDistribution{targetCount: targetCount}
		}
		distributions[originalMinuteKey][lane].beforeCount++

		if distributions[scheduledMinuteKey] == nil {
			distributions[scheduledMinuteKey] = make(map[string]*minuteDistribution)
		}
		if distributions[scheduledMinuteKey][lane] == nil {
			targetCount := 0
			if smoothingTargets != nil {
				if t, ok := smoothingTargets[scheduledMinuteKey]; ok {
					targetCount = t
				}
			}
			distributions[scheduledMinuteKey][lane] = &minuteDistribution{targetCount: targetCount}
		}
		distributions[scheduledMinuteKey][lane].afterCount++

		if item.WasShifted {
			distributions[scheduledMinuteKey][lane].shiftedCount++
		}

		// Track items that used a pre-calculated plan
		if item.WasPlanned {
			distributions[scheduledMinuteKey][lane].plannedCount++
		}
	}

	var records []domain.ThrottleResultRecord
	for minuteKey, laneMap := range distributions {
		virtualMinute, _ := time.Parse(time.RFC3339, minuteKey)
		for lane, dist := range laneMap {
			records = append(records, domain.ThrottleResultRecord{
				RunID:         runID,
				VirtualMinute: virtualMinute,
				Lane:          lane,
				Phase:         "commit",
				BeforeCount:   dist.beforeCount,
				AfterCount:    dist.afterCount,
				ShiftedCount:  dist.shiftedCount,
				PlannedCount:  dist.plannedCount,
				TargetCount:   dist.targetCount,
			})
		}
	}

	return records
}
