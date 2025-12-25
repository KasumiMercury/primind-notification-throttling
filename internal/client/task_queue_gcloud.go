//go:build gcloud

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"time"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	taskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type CloudTasksClient struct {
	client     *cloudtasks.Client
	projectID  string
	locationID string
	queueID    string
	targetURL  string
	maxRetries int
}

type CloudTasksConfig struct {
	ProjectID  string
	LocationID string
	QueueID    string
	TargetURL  string
	MaxRetries int
}

func NewCloudTasksClient(ctx context.Context, cfg CloudTasksConfig) (*CloudTasksClient, error) {
	client, err := cloudtasks.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create cloud tasks client: %w", err)
	}

	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	return &CloudTasksClient{
		client:     client,
		projectID:  cfg.ProjectID,
		locationID: cfg.LocationID,
		queueID:    cfg.QueueID,
		targetURL:  cfg.TargetURL,
		maxRetries: maxRetries,
	}, nil
}

func (c *CloudTasksClient) RegisterNotification(ctx context.Context, task *NotificationTask) (*TaskResponse, error) {
	queuePath := fmt.Sprintf("projects/%s/locations/%s/queues/%s",
		c.projectID, c.locationID, c.queueID)

	payload, err := json.Marshal(task)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal notification task: %w", err)
	}

	taskName := fmt.Sprintf("projects/%s/locations/%s/queues/%s/tasks/%s",
		c.projectID, c.locationID, c.queueID, task.RemindID)

	cloudTask := &taskspb.Task{
		Name: taskName,
		MessageType: &taskspb.Task_HttpRequest{
			HttpRequest: &taskspb.HttpRequest{
				HttpMethod: taskspb.HttpMethod_POST,
				Url:        c.targetURL,
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
				Body: payload,
			},
		},
	}

	if !task.ScheduleAt.IsZero() {
		cloudTask.ScheduleTime = timestamppb.New(task.ScheduleAt)
	}

	req := &taskspb.CreateTaskRequest{
		Parent: queuePath,
		Task:   cloudTask,
	}

	var lastErr error
	for attempt := 0; attempt < c.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(math.Pow(2, float64(attempt-1))) * 100 * time.Millisecond
			slog.Debug("retrying task registration",
				slog.String("remind_id", task.RemindID),
				slog.String("user_id", task.UserID),
				slog.Int("attempt", attempt+1),
				slog.Duration("backoff", backoff),
			)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		resp, err := c.createTask(ctx, req, task.RemindID, task.UserID)
		if err == nil {
			return resp, nil
		}
		lastErr = err
	}

	slog.Error("all retries exhausted for task registration",
		slog.String("remind_id", task.RemindID),
		slog.String("user_id", task.UserID),
		slog.Int("max_retries", c.maxRetries),
		slog.String("error", lastErr.Error()),
	)
	return nil, fmt.Errorf("failed to register task after %d retries: %w", c.maxRetries, lastErr)
}

func (c *CloudTasksClient) createTask(ctx context.Context, req *taskspb.CreateTaskRequest, remindID, userID string) (*TaskResponse, error) {
	slog.Debug("registering notification to Cloud Tasks",
		slog.String("queue_path", req.Parent),
		slog.String("remind_id", remindID),
		slog.String("user_id", userID),
	)

	createdTask, err := c.client.CreateTask(ctx, req)
	if err != nil {
		slog.Warn("failed to create cloud task",
			slog.String("remind_id", remindID),
			slog.String("user_id", userID),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("failed to create cloud task: %w", err)
	}

	slog.Info("notification task registered to Cloud Tasks",
		slog.String("task_name", createdTask.Name),
		slog.String("remind_id", remindID),
		slog.String("user_id", userID),
	)

	var scheduleTime, createTime time.Time
	if createdTask.ScheduleTime != nil {
		scheduleTime = createdTask.ScheduleTime.AsTime()
	}
	if createdTask.CreateTime != nil {
		createTime = createdTask.CreateTime.AsTime()
	}

	return &TaskResponse{
		Name:         createdTask.Name,
		ScheduleTime: scheduleTime,
		CreateTime:   createTime,
	}, nil
}

func (c *CloudTasksClient) Close() error {
	return c.client.Close()
}

func (c *CloudTasksClient) DeleteTask(ctx context.Context, taskID string) error {
	taskPath := fmt.Sprintf("projects/%s/locations/%s/queues/%s/tasks/%s",
		c.projectID, c.locationID, c.queueID, taskID)

	var lastErr error
	for attempt := 0; attempt < c.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(math.Pow(2, float64(attempt-1))) * 100 * time.Millisecond
			slog.Debug("retrying task deletion",
				slog.String("task_id", taskID),
				slog.Int("attempt", attempt+1),
				slog.Duration("backoff", backoff),
			)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		err := c.deleteTask(ctx, taskPath, taskID)
		if err == nil {
			return nil
		}
		lastErr = err
	}

	slog.Error("all retries exhausted for task deletion",
		slog.String("task_id", taskID),
		slog.Int("max_retries", c.maxRetries),
		slog.String("error", lastErr.Error()),
	)
	return fmt.Errorf("failed to delete task after %d retries: %w", c.maxRetries, lastErr)
}

func (c *CloudTasksClient) deleteTask(ctx context.Context, taskPath, taskID string) error {
	slog.Debug("deleting task from Cloud Tasks",
		slog.String("task_path", taskPath),
		slog.String("task_id", taskID),
	)

	req := &taskspb.DeleteTaskRequest{
		Name: taskPath,
	}

	err := c.client.DeleteTask(ctx, req)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			slog.Info("task not found in Cloud Tasks (may have been processed)",
				slog.String("task_id", taskID),
			)
			return nil
		}

		slog.Warn("failed to delete cloud task",
			slog.String("task_id", taskID),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("failed to delete cloud task: %w", err)
	}

	slog.Info("task deleted from Cloud Tasks",
		slog.String("task_id", taskID),
	)
	return nil
}
