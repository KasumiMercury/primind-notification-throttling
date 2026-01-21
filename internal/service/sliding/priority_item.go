package sliding

import (
	"time"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
	"github.com/KasumiMercury/primind-notification-throttling/internal/infra/timemgmt"
)

type PriorityItem struct {
	Remind  timemgmt.RemindResponse
	Lane    domain.Lane
	Release time.Time
	Latest  time.Time
	Index   int
}

func NewPriorityItem(remind timemgmt.RemindResponse, lane domain.Lane) *PriorityItem {
	slideWindow := time.Duration(remind.SlideWindowWidth) * time.Second
	return &PriorityItem{
		Remind:  remind,
		Lane:    lane,
		Release: remind.Time.Add(-slideWindow),
		Latest:  remind.Time.Add(slideWindow),
		Index:   -1,
	}
}

func (p *PriorityItem) IsEligibleForSlot(slotTime time.Time) bool {
	plannedTime := p.CalculatePlannedTime(slotTime)
	if plannedTime.Before(p.Release) {
		return false
	}
	return !plannedTime.After(p.Latest)
}

func (p *PriorityItem) CalculatePlannedTime(slotMinute time.Time) time.Time {
	offset := p.Remind.Time.Sub(p.Remind.Time.Truncate(time.Minute))
	return slotMinute.Truncate(time.Minute).Add(offset)
}
