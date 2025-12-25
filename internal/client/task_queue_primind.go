//go:build !gcloud

package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	commonv1 "github.com/KasumiMercury/primind-notification-throttling/internal/gen/common/v1"
	notifyv1 "github.com/KasumiMercury/primind-notification-throttling/internal/gen/notify/v1"
	taskqueuev1 "github.com/KasumiMercury/primind-notification-throttling/internal/gen/taskqueue/v1"
	pjson "github.com/KasumiMercury/primind-notification-throttling/internal/proto"
)

type PrimindTasksClient struct {
	baseURL    string
	queueName  string
	httpClient *http.Client
	maxRetries int
}

func NewPrimindTasksClient(baseURL, queueName string, maxRetries int) *PrimindTasksClient {
	if maxRetries <= 0 {
		maxRetries = 3
	}
	return &PrimindTasksClient{
		baseURL:   baseURL,
		queueName: queueName,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		maxRetries: maxRetries,
	}
}

func (c *PrimindTasksClient) RegisterNotification(ctx context.Context, task *NotificationTask) (*TaskResponse, error) {
	notifyReq := &notifyv1.NotificationRequest{
		Tokens:   task.FCMTokens,
		TaskId:   task.TaskID,
		TaskType: stringToTaskType(task.TaskType),
	}

	payload, err := pjson.Marshal(notifyReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal notification request: %w", err)
	}

	encodedBody := base64.StdEncoding.EncodeToString(payload)

	taskReq := &taskqueuev1.CreateTaskRequest{
		Task: &taskqueuev1.Task{
			Name: task.RemindID, // Use RemindID as task name for deletion
			HttpRequest: &taskqueuev1.HTTPRequest{
				Body: encodedBody,
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
			},
		},
	}

	if !task.ScheduleAt.IsZero() {
		taskReq.Task.ScheduleTime = task.ScheduleAt.Format(time.RFC3339)
	}

	reqBody, err := pjson.Marshal(taskReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal task request: %w", err)
	}

	url := fmt.Sprintf("%s/tasks", c.baseURL)
	if c.queueName != "" && c.queueName != "default" {
		url = fmt.Sprintf("%s/tasks/%s", c.baseURL, c.queueName)
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

		resp, err := c.doRequest(ctx, url, reqBody, task.RemindID, task.UserID)
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

func (c *PrimindTasksClient) doRequest(ctx context.Context, url string, reqBody []byte, remindID, userID string) (*TaskResponse, error) {
	slog.Debug("registering notification to Primind Tasks",
		slog.String("url", url),
		slog.String("remind_id", remindID),
		slog.String("user_id", userID),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		slog.Warn("failed to send request to Primind Tasks",
			slog.String("remind_id", remindID),
			slog.String("user_id", userID),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		slog.Warn("unexpected status code from Primind Tasks",
			slog.String("remind_id", remindID),
			slog.String("user_id", userID),
			slog.Int("status_code", resp.StatusCode),
		)
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var protoResp taskqueuev1.CreateTaskResponse
	if err := pjson.Unmarshal(body, &protoResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	scheduleTime, _ := time.Parse(time.RFC3339, protoResp.ScheduleTime)
	createTime, _ := time.Parse(time.RFC3339, protoResp.CreateTime)

	slog.Info("notification task registered to Primind Tasks",
		slog.String("task_name", protoResp.Name),
		slog.String("remind_id", remindID),
		slog.String("user_id", userID),
	)

	return &TaskResponse{
		Name:         protoResp.Name,
		ScheduleTime: scheduleTime,
		CreateTime:   createTime,
	}, nil
}

func stringToTaskType(s string) commonv1.TaskType {
	upper := "TASK_TYPE_" + strings.ToUpper(s)
	if v, ok := commonv1.TaskType_value[upper]; ok {
		return commonv1.TaskType(v)
	}
	return commonv1.TaskType_TASK_TYPE_UNSPECIFIED
}

func (c *PrimindTasksClient) DeleteTask(ctx context.Context, taskID string) error {
	url := fmt.Sprintf("%s/tasks/%s", c.baseURL, taskID)
	if c.queueName != "" && c.queueName != "default" {
		url = fmt.Sprintf("%s/tasks/%s/%s", c.baseURL, c.queueName, taskID)
	}

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

		err := c.doDeleteRequest(ctx, url, taskID)
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

func (c *PrimindTasksClient) doDeleteRequest(ctx context.Context, url, taskID string) error {
	slog.Debug("deleting task from Primind Tasks",
		slog.String("url", url),
		slog.String("task_id", taskID),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create delete request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		slog.Warn("failed to send delete request to Primind Tasks",
			slog.String("task_id", taskID),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("failed to send delete request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		slog.Info("task not found (may have been processed)",
			slog.String("task_id", taskID),
		)
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		slog.Warn("unexpected status code from Primind Tasks delete",
			slog.String("task_id", taskID),
			slog.Int("status_code", resp.StatusCode),
			slog.String("body", string(body)),
		)
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	slog.Info("task deleted from Primind Tasks",
		slog.String("task_id", taskID),
	)
	return nil
}
