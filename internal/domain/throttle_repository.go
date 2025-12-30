package domain

import "context"

//go:generate mockgen -source=throttle_repository.go -destination=throttle_repository_mock.go -package=domain

type ThrottleRepository interface {
	GetPacketCountForMinute(ctx context.Context, minuteKey string) (int, error)
	IncrementPacketCount(ctx context.Context, minuteKey string, delta int) error
	SaveCommittedPacket(ctx context.Context, packet *Packet) error
	IsPacketCommitted(ctx context.Context, remindID string) (bool, error)
	GetCommittedPacket(ctx context.Context, remindID string) (*Packet, error)
	SavePlan(ctx context.Context, plan *Plan) error
	GetPlan(ctx context.Context, minuteKey string) (*Plan, error)
	GetPlansInRange(ctx context.Context, startMinuteKey, endMinuteKey string) ([]*Plan, error)
	DeletePlan(ctx context.Context, minuteKey string) error
	GetPlannedPacket(ctx context.Context, remindID string) (*PlannedPacket, error)
	GetPlannedPacketCount(ctx context.Context, minuteKey string) (int, error)
}
