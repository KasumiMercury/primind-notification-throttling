package client

import "context"

//go:generate mockgen -source=task_queue.go -destination=task_queue_mock.go -package=client

type TaskQueue interface {
	RegisterNotification(ctx context.Context, task *NotificationTask) (*TaskResponse, error)
}
