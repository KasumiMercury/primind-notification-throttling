package throttlerecorder

import (
	"context"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
)

type noopRecorder struct{}

func NewNoopRecorder() domain.ThrottleResultRecorder {
	return &noopRecorder{}
}

func (n *noopRecorder) RecordBatchResults(_ context.Context, _ []domain.ThrottleResultRecord) error {
	return nil
}

func (n *noopRecorder) Flush(_ context.Context) error {
	return nil
}

func (n *noopRecorder) Close() error {
	return nil
}
