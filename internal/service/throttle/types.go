package throttle

import (
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
)

type ResultItem struct {
	RemindID      string      `json:"remind_id"`
	TaskID        string      `json:"task_id"`
	TaskType      string      `json:"task_type"`
	FCMTokens     []string    `json:"fcm_tokens"`
	Lane          domain.Lane `json:"lane"`
	OriginalTime  time.Time   `json:"original_time"`
	ScheduledTime time.Time   `json:"scheduled_time"`
	WasShifted    bool        `json:"was_shifted"`
	Skipped       bool        `json:"skipped"`
	SkipReason    string      `json:"skip_reason,omitempty"`
	Success       bool        `json:"success"`
	Error         string      `json:"error,omitempty"`
}

type Response struct {
	ProcessedCount int          `json:"processed_count"`
	SuccessCount   int          `json:"success_count"`
	FailedCount    int          `json:"failed_count"`
	SkippedCount   int          `json:"skipped_count"`
	ShiftedCount   int          `json:"shifted_count"`
	Results        []ResultItem `json:"results"`
}
