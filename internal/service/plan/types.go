package plan

import (
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
	"github.com/KasumiMercury/primind-notification-throttling/internal/infra/timemgmt"
)

type ResultItem struct {
	RemindID     string      `json:"remind_id"`
	TaskID       string      `json:"task_id"`
	TaskType     string      `json:"task_type"`
	Lane         domain.Lane `json:"lane"`
	OriginalTime time.Time   `json:"original_time"`
	PlannedTime  time.Time   `json:"planned_time"`
	WasShifted   bool        `json:"was_shifted"`
	Skipped      bool        `json:"skipped"`
	SkipReason   string      `json:"skip_reason,omitempty"`
}

type SmoothingTarget struct {
	MinuteKey  string    `json:"minute_key"`
	MinuteTime time.Time `json:"minute_time"`
	Target     int       `json:"target"`
	InputCount int       `json:"input_count"`
}

type Response struct {
	PlannedCount     int               `json:"planned_count"`
	SkippedCount     int               `json:"skipped_count"`
	ShiftedCount     int               `json:"shifted_count"`
	Results          []ResultItem      `json:"results"`
	SmoothingTargets []SmoothingTarget `json:"smoothing_targets,omitempty"`
}

type MergedNotification struct {
	Remind       timemgmt.RemindResponse
	PreviousPlan *domain.PlannedPacket
	IsNew        bool
}

type MergeResult struct {
	Notifications          []MergedNotification
	NewCount               int
	PreviouslyPlannedCount int
}
