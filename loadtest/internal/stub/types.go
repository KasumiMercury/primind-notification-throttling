package stub

import "time"

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
	SlideWindowWidth int32            `json:"slide_window_width"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

type RemindsResponse struct {
	Reminds []RemindResponse `json:"reminds"`
	Count   int              `json:"count"`
}

type SeedRequest struct {
	Buckets []SeedBucket `json:"buckets"`
}

type SeedBucket struct {
	StartTime        string           `json:"start_time"`
	EndTime          string           `json:"end_time"`
	Count            int              `json:"count"`
	SlideWindowWidth int32            `json:"slide_window_width"`
	TaskType         string           `json:"task_type"`
	Devices          []DeviceResponse `json:"devices,omitempty"`
}

type UpdateThrottledRequest struct {
	Throttled bool `json:"throttled"`
}
