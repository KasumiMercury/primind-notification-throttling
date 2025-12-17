package client

import (
	"context"
	"time"
)

//go:generate mockgen -source=remind_time.go -destination=remind_time_mock.go -package=client

type RemindTimeRepository interface {
	GetRemindsByTimeRange(ctx context.Context, start, end time.Time) (*RemindsResponse, error)
	UpdateThrottled(ctx context.Context, id string, throttled bool) error
}
