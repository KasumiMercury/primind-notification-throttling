package service

type ThrottleResultItem struct {
	RemindID  string   `json:"remind_id"`
	TaskID    string   `json:"task_id"`
	TaskType  string   `json:"task_type"`
	FCMTokens []string `json:"fcm_tokens"`
	Success   bool     `json:"success"`
	Error     string   `json:"error,omitempty"`
}

type ThrottleResponse struct {
	ProcessedCount int                  `json:"processed_count"`
	SuccessCount   int                  `json:"success_count"`
	FailedCount    int                  `json:"failed_count"`
	Results        []ThrottleResultItem `json:"results"`
}
