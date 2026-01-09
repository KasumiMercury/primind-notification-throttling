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
	return &PriorityItem{
		Remind:  remind,
		Lane:    lane,
		Release: remind.Time,
		Latest:  remind.Time.Add(time.Duration(remind.SlideWindowWidth) * time.Second),
		Index:   -1,
	}
}

func (p *PriorityItem) IsEligibleForSlot(slotTime time.Time) bool {
	slotMinute := slotTime.Truncate(time.Minute)
	return !slotMinute.After(p.Latest)
}

func (p *PriorityItem) CalculatePlannedTime(slotMinute time.Time) time.Time {
	offset := p.Remind.Time.Sub(p.Remind.Time.Truncate(time.Minute))
	return slotMinute.Truncate(time.Minute).Add(offset)
}
