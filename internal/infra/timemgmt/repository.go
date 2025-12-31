package timemgmt

import (
	"context"
	"time"
)

//go:generate mockgen -source=repository.go -destination=mock.go -package=timemgmt

type RemindTimeRepository interface {
	GetRemindsByTimeRange(ctx context.Context, start, end time.Time, runID string) (*RemindsResponse, error)
	UpdateThrottled(ctx context.Context, id string, throttled bool) error
}
