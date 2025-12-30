package domain

import (
	"time"
)

type PlannedPacket struct {
	RemindID     string    `json:"remind_id"`
	OriginalTime time.Time `json:"original_time"`
	PlannedTime  time.Time `json:"planned_time"`
	Lane         Lane      `json:"lane"`
}

func (p *PlannedPacket) WasShifted() bool {
	return !p.PlannedTime.Equal(p.OriginalTime)
}

type Plan struct {
	MinuteKey      string          `json:"minute_key"`
	PlannedPackets []PlannedPacket `json:"planned_packets"`
	PlannedAt      time.Time       `json:"planned_at"`
	TotalPlanned   int             `json:"total_planned"`
}

func NewPlan(minuteKey string) *Plan {
	return &Plan{
		MinuteKey:      minuteKey,
		PlannedPackets: make([]PlannedPacket, 0),
		PlannedAt:      time.Now().UTC(),
		TotalPlanned:   0,
	}
}

func (p *Plan) AddPacket(packet PlannedPacket) {
	p.PlannedPackets = append(p.PlannedPackets, packet)
	p.TotalPlanned = len(p.PlannedPackets)
}

func NewPlannedPacket(remindID string, originalTime, plannedTime time.Time, lane Lane) PlannedPacket {
	return PlannedPacket{
		RemindID:     remindID,
		OriginalTime: originalTime,
		PlannedTime:  plannedTime,
		Lane:         lane,
	}
}
