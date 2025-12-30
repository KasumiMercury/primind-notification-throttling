package slot

import (
	"context"
	"errors"
	"fmt"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
)

type Counter interface {
	GetTotalCountForMinute(ctx context.Context, minuteKey string) (int, error)
}

type counterImpl struct {
	throttleRepo domain.ThrottleRepository
}

func NewCounter(throttleRepo domain.ThrottleRepository) Counter {
	return &counterImpl{
		throttleRepo: throttleRepo,
	}
}

func (s *counterImpl) GetTotalCountForMinute(ctx context.Context, minuteKey string) (int, error) {
	var (
		totalCount   int
		committedErr error
		plannedErr   error
	)

	// Get committed packet count
	committedCount, err := s.throttleRepo.GetPacketCountForMinute(ctx, minuteKey)
	if err != nil {
		committedErr = fmt.Errorf("committed count: %w", err)
	} else {
		totalCount += committedCount
	}

	// Get planned packet count
	plannedCount, err := s.throttleRepo.GetPlannedPacketCount(ctx, minuteKey)
	if err != nil {
		plannedErr = fmt.Errorf("planned count: %w", err)
	} else {
		totalCount += plannedCount
	}

	// Combine errors if any occurred
	if combinedErr := errors.Join(committedErr, plannedErr); combinedErr != nil {
		return totalCount, fmt.Errorf("failed to get packet counts for minute %s: %w", minuteKey, combinedErr)
	}

	return totalCount, nil
}
