package config

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	RemindTimeManagementURL string
	Port                    string
	TimeRangeMinutes        int
	LogLevel                slog.Level
}

func Load() *Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	timeRangeMinutes := 10
	if v := os.Getenv("TIME_RANGE_MINUTES"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			timeRangeMinutes = parsed
		}
	}

	return &Config{
		RemindTimeManagementURL: os.Getenv("REMIND_TIME_MANAGEMENT_URL"),
		Port:                    port,
		TimeRangeMinutes:        timeRangeMinutes,
		LogLevel:                parseLogLevel(os.Getenv("LOG_LEVEL")),
	}
}

func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
