package lane

import (
	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
	"github.com/KasumiMercury/primind-notification-throttling/internal/infra/timemgmt"
)

const (
	// StrictSlideWindowThreshold is the maximum slide window width (in seconds)
	// for a task to be classified as strict lane.
	// This corresponds to a total window of 5 minutes (one-sided <= 2.5 minutes = 150 seconds).
	// Tasks with slide window <= 150 seconds are considered time-critical.
	StrictSlideWindowThreshold = 150
)

type Classifier struct{}

func NewClassifier() *Classifier {
	return &Classifier{}
}

func (c *Classifier) Classify(remind timemgmt.RemindResponse) domain.Lane {
	// SlideWindowWidth=0 or narrow windows are strict
	if remind.SlideWindowWidth <= StrictSlideWindowThreshold {
		return domain.LaneStrict
	}

	return domain.LaneLoose
}
