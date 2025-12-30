package config

import (
	"os"
	"strconv"
)

const (
	redisAddrEnv     = "REDIS_ADDR"
	redisPasswordEnv = "REDIS_PASSWORD"
	redisDBEnv       = "REDIS_DB"

	defaultRedisAddr = "localhost:6379"
	defaultRedisDB   = 0
)

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

func LoadRedisConfig() (*RedisConfig, error) {
	addr := os.Getenv(redisAddrEnv)
	if addr == "" {
		addr = defaultRedisAddr
	}

	password := os.Getenv(redisPasswordEnv)

	db := defaultRedisDB
	if raw := os.Getenv(redisDBEnv); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return nil, ErrInvalidRedisDB
		}
		db = parsed
	}

	return &RedisConfig{
		Addr:     addr,
		Password: password,
		DB:       db,
	}, nil
}

func (c *RedisConfig) Validate() error {
	if c == nil || c.Addr == "" {
		return ErrRedisAddrMissing
	}
	return nil
}
