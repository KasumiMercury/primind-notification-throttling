//go:build gcloud

package main

import (
	"context"
	"log/slog"

	"github.com/KasumiMercury/primind-notification-throttling/internal/client"
	"github.com/KasumiMercury/primind-notification-throttling/internal/config"
)

func initTaskQueue(ctx context.Context, cfg *config.Config) (client.TaskQueue, func() error, error) {
	cloudTasksClient, err := client.NewCloudTasksClient(ctx, client.CloudTasksConfig{
		ProjectID:  cfg.TaskQueue.GCloudProjectID,
		LocationID: cfg.TaskQueue.GCloudLocationID,
		QueueID:    cfg.TaskQueue.GCloudQueueID,
		TargetURL:  cfg.TaskQueue.GCloudTargetURL,
		MaxRetries: cfg.TaskQueue.MaxRetries,
	})
	if err != nil {
		return nil, nil, err
	}

	slog.Info("task queue initialized",
		slog.String("type", "cloud_tasks"),
		slog.String("project", cfg.TaskQueue.GCloudProjectID),
		slog.String("location", cfg.TaskQueue.GCloudLocationID),
		slog.String("queue", cfg.TaskQueue.GCloudQueueID),
	)

	cleanup := func() error {
		if err := cloudTasksClient.Close(); err != nil {
			slog.Warn("failed to close cloud tasks client", slog.String("error", err.Error()))
			return err
		}
		return nil
	}

	return cloudTasksClient, cleanup, nil
}
