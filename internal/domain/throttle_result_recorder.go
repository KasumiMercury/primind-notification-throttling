package domain

import (
	"context"
	"time"
)

type ThrottleResultRecord struct {
	RunID        string
	SlotTime     time.Time
	Lane         string
	Phase        string
	BeforeCount  int
	AfterCount   int
	ShiftedCount int
	PlannedCount int
	SkippedCount int
	FailedCount  int
}

type SmoothingTargetRecord struct {
	RunID       string
	SlotTime    time.Time
	TargetCount int
}

type ThrottleResultRecorder interface {
	RecordBatchResults(ctx context.Context, records []ThrottleResultRecord) error
	RecordSmoothingTargets(ctx context.Context, records []SmoothingTargetRecord) error
	FillAllMinutes() bool
	Flush(ctx context.Context) error
	Close() error
}
