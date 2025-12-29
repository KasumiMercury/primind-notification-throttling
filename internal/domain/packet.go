package domain

import (
	"time"
)

type Packet struct {
	RemindID      string
	OriginalTime  time.Time
	ScheduledTime time.Time
	Lane          Lane
	CommittedAt   time.Time
}

func NewPacket(remindID string, originalTime, scheduledTime time.Time, lane Lane) *Packet {
	return &Packet{
		RemindID:      remindID,
		OriginalTime:  originalTime,
		ScheduledTime: scheduledTime,
		Lane:          lane,
		CommittedAt:   time.Now().UTC(),
	}
}

func (p *Packet) WasShifted() bool {
	return !p.OriginalTime.Equal(p.ScheduledTime)
}

func (p *Packet) ScheduledMinuteKey() string {
	return MinuteKey(p.ScheduledTime)
}

func MinuteKey(t time.Time) string {
	return t.UTC().Truncate(time.Minute).Format("2006-01-02-15-04")
}

func ParseMinuteKey(key string) (time.Time, error) {
	return time.Parse("2006-01-02-15-04", key)
}
