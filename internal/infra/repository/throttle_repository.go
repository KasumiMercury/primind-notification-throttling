package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/KasumiMercury/primind-notification-throttling/internal/domain"
)

const (
	packetCountKeyPrefix     = "throttle:packets:"
	committedPacketKeyPrefix = "throttle:committed:"
	planKeyPrefix            = "throttle:plan:"
	plannedPacketKeyPrefix   = "throttle:planned:"

	packetCountTTL     = 1 * time.Hour    // 1 hour
	committedPacketTTL = 2 * time.Hour    // 2 hours
	planTTL            = 30 * time.Minute // 30 minutes (covers planning window)
)

type packetRecord struct {
	RemindID      string    `json:"remind_id"`
	OriginalTime  time.Time `json:"original_time"`
	ScheduledTime time.Time `json:"scheduled_time"`
	Lane          string    `json:"lane"`
	CommittedAt   time.Time `json:"committed_at"`
}

type planRecord struct {
	MinuteKey      string                `json:"minute_key"`
	PlannedPackets []plannedPacketRecord `json:"planned_packets"`
	PlannedAt      time.Time             `json:"planned_at"`
	TotalPlanned   int                   `json:"total_planned"`
}

type plannedPacketRecord struct {
	RemindID     string    `json:"remind_id"`
	OriginalTime time.Time `json:"original_time"`
	PlannedTime  time.Time `json:"planned_time"`
	Lane         string    `json:"lane"`
}

type throttleRepository struct {
	client *redis.Client
}

func NewThrottleRepository(client *redis.Client) domain.ThrottleRepository {
	return &throttleRepository{
		client: client,
	}
}

func (r *throttleRepository) GetPacketCountForMinute(ctx context.Context, minuteKey string) (int, error) {
	key := packetCountKeyPrefix + minuteKey

	val, err := r.client.Get(ctx, key).Int()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}
		return 0, err
	}

	return val, nil
}

func (r *throttleRepository) IncrementPacketCount(ctx context.Context, minuteKey string, delta int) error {
	key := packetCountKeyPrefix + minuteKey

	pipe := r.client.TxPipeline()
	pipe.IncrBy(ctx, key, int64(delta))
	pipe.Expire(ctx, key, packetCountTTL)

	_, err := pipe.Exec(ctx)
	return err
}

func (r *throttleRepository) SaveCommittedPacket(ctx context.Context, packet *domain.Packet) error {
	if packet == nil {
		return ErrInvalidPacketData
	}

	key := committedPacketKeyPrefix + packet.RemindID

	record := packetRecord{
		RemindID:      packet.RemindID,
		OriginalTime:  packet.OriginalTime,
		ScheduledTime: packet.ScheduledTime,
		Lane:          packet.Lane.String(),
		CommittedAt:   packet.CommittedAt,
	}

	data, err := json.Marshal(record)
	if err != nil {
		return ErrInvalidPacketData
	}

	return r.client.Set(ctx, key, data, committedPacketTTL).Err()
}

func (r *throttleRepository) IsPacketCommitted(ctx context.Context, remindID string) (bool, error) {
	key := committedPacketKeyPrefix + remindID

	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}

	return exists > 0, nil
}

func (r *throttleRepository) GetCommittedPacket(ctx context.Context, remindID string) (*domain.Packet, error) {
	key := committedPacketKeyPrefix + remindID

	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, domain.ErrPacketNotFound
		}
		return nil, err
	}

	var record packetRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, ErrInvalidPacketData
	}

	return &domain.Packet{
		RemindID:      record.RemindID,
		OriginalTime:  record.OriginalTime,
		ScheduledTime: record.ScheduledTime,
		Lane:          domain.Lane(record.Lane),
		CommittedAt:   record.CommittedAt,
	}, nil
}

func (r *throttleRepository) SavePlan(ctx context.Context, plan *domain.Plan) error {
	if plan == nil {
		return ErrInvalidPlanData
	}

	plannedRecords := make([]plannedPacketRecord, 0, len(plan.PlannedPackets))
	for _, p := range plan.PlannedPackets {
		plannedRecords = append(plannedRecords, plannedPacketRecord{
			RemindID:     p.RemindID,
			OriginalTime: p.OriginalTime,
			PlannedTime:  p.PlannedTime,
			Lane:         p.Lane.String(),
		})
	}

	record := planRecord{
		MinuteKey:      plan.MinuteKey,
		PlannedPackets: plannedRecords,
		PlannedAt:      plan.PlannedAt,
		TotalPlanned:   plan.TotalPlanned,
	}

	data, err := json.Marshal(record)
	if err != nil {
		return ErrInvalidPlanData
	}

	pipe := r.client.TxPipeline()

	planKey := planKeyPrefix + plan.MinuteKey
	pipe.Set(ctx, planKey, data, planTTL)

	for _, p := range plan.PlannedPackets {
		packetKey := plannedPacketKeyPrefix + p.RemindID
		packetData, err := json.Marshal(plannedPacketRecord{
			RemindID:     p.RemindID,
			OriginalTime: p.OriginalTime,
			PlannedTime:  p.PlannedTime,
			Lane:         p.Lane.String(),
		})
		if err != nil {
			return ErrInvalidPlanData
		}
		pipe.Set(ctx, packetKey, packetData, planTTL)
	}

	_, err = pipe.Exec(ctx)
	return err
}

func (r *throttleRepository) GetPlan(ctx context.Context, minuteKey string) (*domain.Plan, error) {
	key := planKeyPrefix + minuteKey

	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, domain.ErrPlanNotFound
		}
		return nil, err
	}

	var record planRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, ErrInvalidPlanData
	}

	plannedPackets := make([]domain.PlannedPacket, 0, len(record.PlannedPackets))
	for _, p := range record.PlannedPackets {
		plannedPackets = append(plannedPackets, domain.PlannedPacket{
			RemindID:     p.RemindID,
			OriginalTime: p.OriginalTime,
			PlannedTime:  p.PlannedTime,
			Lane:         domain.Lane(p.Lane),
		})
	}

	return &domain.Plan{
		MinuteKey:      record.MinuteKey,
		PlannedPackets: plannedPackets,
		PlannedAt:      record.PlannedAt,
		TotalPlanned:   record.TotalPlanned,
	}, nil
}

func (r *throttleRepository) GetPlansInRange(ctx context.Context, startMinuteKey, endMinuteKey string) ([]*domain.Plan, error) {
	pattern := planKeyPrefix + "*"
	plans := make([]*domain.Plan, 0)

	iter := r.client.Scan(ctx, 0, pattern, 0).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		minuteKey := key[len(planKeyPrefix):]

		if minuteKey >= startMinuteKey && minuteKey <= endMinuteKey {
			plan, err := r.GetPlan(ctx, minuteKey)
			if err != nil {
				if errors.Is(err, domain.ErrPlanNotFound) {
					continue
				}
				return nil, err
			}
			plans = append(plans, plan)
		}
	}

	if err := iter.Err(); err != nil {
		return nil, err
	}

	return plans, nil
}

func (r *throttleRepository) DeletePlan(ctx context.Context, minuteKey string) error {
	plan, err := r.GetPlan(ctx, minuteKey)
	if err != nil {
		if errors.Is(err, domain.ErrPlanNotFound) {
			return nil // Already deleted
		}
		return err
	}

	pipe := r.client.TxPipeline()

	planKey := planKeyPrefix + minuteKey
	pipe.Del(ctx, planKey)

	for _, p := range plan.PlannedPackets {
		packetKey := plannedPacketKeyPrefix + p.RemindID
		pipe.Del(ctx, packetKey)
	}

	_, err = pipe.Exec(ctx)
	return err
}

func (r *throttleRepository) GetPlannedPacket(ctx context.Context, remindID string) (*domain.PlannedPacket, error) {
	key := plannedPacketKeyPrefix + remindID

	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, domain.ErrPlannedPacketNotFound
		}
		return nil, err
	}

	var record plannedPacketRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, ErrInvalidPlanData
	}

	return &domain.PlannedPacket{
		RemindID:     record.RemindID,
		OriginalTime: record.OriginalTime,
		PlannedTime:  record.PlannedTime,
		Lane:         domain.Lane(record.Lane),
	}, nil
}

func (r *throttleRepository) GetPlannedPacketCount(ctx context.Context, minuteKey string) (int, error) {
	plan, err := r.GetPlan(ctx, minuteKey)
	if err != nil {
		if errors.Is(err, domain.ErrPlanNotFound) {
			return 0, nil
		}
		return 0, err
	}

	return plan.TotalPlanned, nil
}
