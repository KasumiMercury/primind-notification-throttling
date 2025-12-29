package client

import "time"

type GetRemindsByTimeRangeRequest struct {
	Start time.Time `form:"start" binding:"required" time_format:"2006-01-02T15:04:05Z07:00"`
	End   time.Time `form:"end" binding:"required,gtfield=Start" time_format:"2006-01-02T15:04:05Z07:00"`
}

type UpdateThrottledRequest struct {
	Throttled bool `json:"throttled"`
}

type DeviceResponse struct {
	DeviceID string `json:"device_id"`
	FCMToken string `json:"fcm_token"`
}

type RemindResponse struct {
	ID               string           `json:"id"`
	Time             time.Time        `json:"time"`
	UserID           string           `json:"user_id"`
	Devices          []DeviceResponse `json:"devices"`
	TaskID           string           `json:"task_id"`
	TaskType         string           `json:"task_type"`
	Throttled        bool             `json:"throttled"`
	SlideWindowWidth int32            `json:"slide_window_width"` // slide window width in seconds (range: 60-1800)
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

type RemindsResponse struct {
	Reminds []RemindResponse `json:"reminds"`
	Count   int              `json:"count"`
}
