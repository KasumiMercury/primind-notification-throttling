package service

import (
	"github.com/KasumiMercury/primind-notification-throttling/internal/client"
	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
)

type LaneClassifier struct{}

func NewLaneClassifier() *LaneClassifier {
	return &LaneClassifier{}
}

func (c *LaneClassifier) Classify(remind client.RemindResponse) domain.Lane {
	// SlideWindowWidth=0 or narrow windows are strict
	if remind.SlideWindowWidth <= StrictSlideWindowThreshold {
		return domain.LaneStrict
	}

	return domain.LaneLoose
}
