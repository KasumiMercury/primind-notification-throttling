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
	TaskQueue               TaskQueueConfig
}

type TaskQueueConfig struct {
	PrimindTasksURL string
	QueueName       string

	GCloudProjectID  string
	GCloudLocationID string
	GCloudQueueID    string
	GCloudTargetURL  string

	MaxRetries int
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

	queueName := os.Getenv("TASK_QUEUE_NAME")
	if queueName == "" {
		queueName = "default"
	}

	maxRetries := 3
	if v := os.Getenv("TASK_QUEUE_MAX_RETRIES"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			maxRetries = parsed
		}
	}

	return &Config{
		RemindTimeManagementURL: os.Getenv("REMIND_TIME_MANAGEMENT_URL"),
		Port:                    port,
		TimeRangeMinutes:        timeRangeMinutes,
		LogLevel:                parseLogLevel(os.Getenv("LOG_LEVEL")),
		TaskQueue: TaskQueueConfig{
			PrimindTasksURL: os.Getenv("PRIMIND_TASKS_URL"),
			QueueName:       queueName,

			GCloudProjectID:  os.Getenv("GCLOUD_PROJECT_ID"),
			GCloudLocationID: os.Getenv("GCLOUD_LOCATION_ID"),
			GCloudQueueID:    os.Getenv("GCLOUD_QUEUE_ID"),
			GCloudTargetURL:  os.Getenv("GCLOUD_TARGET_URL"),

			MaxRetries: maxRetries,
		},
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
