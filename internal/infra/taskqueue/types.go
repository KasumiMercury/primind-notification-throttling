package taskqueue

import "time"

type NotificationTask struct {
	RemindID   string    `json:"-"`
	UserID     string    `json:"-"`
	ScheduleAt time.Time `json:"-"`

	FCMTokens []string `json:"tokens"`
	TaskID    string   `json:"task_id"`
	TaskType  string   `json:"task_type"`
	Color     string   `json:"color"` // hex color code e.g. "#EF4444"
}

type TaskResponse struct {
	Name         string    `json:"name"`
	ScheduleTime time.Time `json:"schedule_time"`
	CreateTime   time.Time `json:"create_time"`
}

type PrimindTaskRequest struct {
	Task PrimindTask `json:"task"`
}

type PrimindTask struct {
	HTTPRequest  PrimindHTTPRequest `json:"httpRequest"`
	ScheduleTime string             `json:"scheduleTime,omitempty"`
}

type PrimindHTTPRequest struct {
	Body    string            `json:"body"`
	Headers map[string]string `json:"headers,omitempty"`
}

type PrimindTaskResponse struct {
	Name         string `json:"name"`
	ScheduleTime string `json:"scheduleTime"`
	CreateTime   string `json:"createTime"`
}
