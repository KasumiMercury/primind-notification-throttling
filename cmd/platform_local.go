//go:build !gcloud

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

func initTaskQueue(_ context.Context, cfg *config.Config) (taskqueue.TaskQueue, func() error, error) {
	if cfg.TaskQueue.PrimindTasksURL == "" {
		slog.Warn("PRIMIND_TASKS_URL not set, task queue registration disabled")

		return nil, nil, nil
	}

	tq := taskqueue.NewPrimindTasksClient(
		cfg.TaskQueue.PrimindTasksURL,
		cfg.TaskQueue.QueueName,
		cfg.TaskQueue.MaxRetries,
	)

	slog.Info("task queue initialized",
		slog.String("type", "primind_tasks"),
		slog.String("url", cfg.TaskQueue.PrimindTasksURL),
		slog.String("queue", cfg.TaskQueue.QueueName),
	)

	return tq, nil, nil
}

func initObservability(ctx context.Context) (*observability.Resources, error) {
	serviceName := os.Getenv("SERVICE_NAME")
	if serviceName == "" {
		serviceName = "throttling"
	}

	env := logging.EnvDev
	if e := os.Getenv("ENV"); e != "" {
		env = logging.Environment(e)
	}

	obs, err := observability.Init(ctx, observability.Config{
		ServiceInfo: logging.ServiceInfo{
			Name:     serviceName,
			Version:  Version,
			Revision: "",
		},
		Environment:   env,
		GCPProjectID:  "",
		SamplingRate:  1.0,
		DefaultModule: logging.Module("notification-throttling"),
	})
	if err != nil {
		return nil, err
	}

	return obs, nil
}
