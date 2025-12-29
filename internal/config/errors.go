package config

import "errors"

var (
	ErrRedisAddrMissing = errors.New("REDIS_ADDR is required")
	ErrInvalidRedisDB   = errors.New("REDIS_DB must be a valid integer")
)
