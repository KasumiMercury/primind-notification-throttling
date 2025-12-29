package service

import (
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
)

type PlanResultItem struct {
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

type PlanResponse struct {
	PlannedCount int              `json:"planned_count"`
	SkippedCount int              `json:"skipped_count"`
	ShiftedCount int              `json:"shifted_count"`
	Results      []PlanResultItem `json:"results"`
}
