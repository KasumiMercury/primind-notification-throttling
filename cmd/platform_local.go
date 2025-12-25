//go:build !gcloud

package main

import (
	"context"
	"log/slog"

	"github.com/KasumiMercury/primind-notification-throttling/internal/client"
	"github.com/KasumiMercury/primind-notification-throttling/internal/config"
)

func initTaskQueue(_ context.Context, cfg *config.Config) (client.TaskQueue, func() error, error) {
	if cfg.TaskQueue.PrimindTasksURL == "" {
		slog.Warn("PRIMIND_TASKS_URL not set, task queue registration disabled")
		return nil, nil, nil
	}

	taskQueue := client.NewPrimindTasksClient(
		cfg.TaskQueue.PrimindTasksURL,
		cfg.TaskQueue.QueueName,
		cfg.TaskQueue.MaxRetries,
	)

	slog.Info("task queue initialized",
		slog.String("type", "primind_tasks"),
		slog.String("url", cfg.TaskQueue.PrimindTasksURL),
		slog.String("queue", cfg.TaskQueue.QueueName),
	)

	return taskQueue, nil, nil
}
