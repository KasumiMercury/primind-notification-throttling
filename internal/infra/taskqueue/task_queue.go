package taskqueue

import "context"

//go:generate mockgen -source=task_queue.go -destination=mock.go -package=taskqueue

type TaskQueue interface {
	RegisterNotification(ctx context.Context, task *NotificationTask) (*TaskResponse, error)
	DeleteTask(ctx context.Context, taskID string) error
}
