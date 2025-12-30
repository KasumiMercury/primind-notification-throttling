//go:build gcloud

package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/KasumiMercury/primind-notification-throttling/internal/config"
	"github.com/KasumiMercury/primind-notification-throttling/internal/infra/taskqueue"
	"github.com/KasumiMercury/primind-notification-throttling/internal/observability"
	"github.com/KasumiMercury/primind-notification-throttling/internal/observability/logging"
)

func initTaskQueue(ctx context.Context, cfg *config.Config) (taskqueue.TaskQueue, func() error, error) {
	cloudTasksClient, err := taskqueue.NewCloudTasksClient(ctx, taskqueue.CloudTasksConfig{
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

func initObservability(ctx context.Context) (*observability.Resources, error) {
	serviceName := os.Getenv("K_SERVICE")
	if serviceName == "" {
		serviceName = "throttling"
	}

	env := logging.EnvProd
	if e := os.Getenv("ENV"); e != "" {
		env = logging.Environment(e)
	}

	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		projectID = os.Getenv("GCLOUD_PROJECT_ID")
	}

	obs, err := observability.Init(ctx, observability.Config{
		ServiceInfo: logging.ServiceInfo{
			Name:     serviceName,
			Version:  Version,
			Revision: os.Getenv("K_REVISION"),
		},
		Environment:   env,
		GCPProjectID:  projectID,
		SamplingRate:  1.0,
		DefaultModule: logging.Module("notification-throttling"),
	})
	if err != nil {
		return nil, err
	}

	return obs, nil
}
