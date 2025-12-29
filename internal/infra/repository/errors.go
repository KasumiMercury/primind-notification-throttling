package repository

import "errors"

var (
	ErrRedisConnection   = errors.New("redis connection error")
	ErrInvalidPacketData = errors.New("invalid packet data")
	ErrInvalidPlanData   = errors.New("invalid plan data")
)
