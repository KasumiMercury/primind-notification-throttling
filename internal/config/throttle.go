package config

import (
	"os"
	"strconv"
)

const (
	requestCapPerMinuteEnv   = "REQUEST_CAP_PER_MINUTE"
	confirmWindowMinutesEnv  = "CONFIRM_WINDOW_MINUTES"
	planningWindowMinutesEnv = "PLANNING_WINDOW_MINUTES"

	defaultRequestCapPerMinute   = 60
	defaultConfirmWindowMinutes  = 5
	defaultPlanningWindowMinutes = 30
)

type ThrottleConfig struct {
	RequestCapPerMinute   int
	ConfirmWindowMinutes  int
	PlanningWindowMinutes int
}

func LoadThrottleConfig() *ThrottleConfig {
	requestCap := defaultRequestCapPerMinute
	if v := os.Getenv(requestCapPerMinuteEnv); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			requestCap = parsed
		}
	}

	confirmWindow := defaultConfirmWindowMinutes
	if v := os.Getenv(confirmWindowMinutesEnv); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			confirmWindow = parsed
		}
	}

	planningWindow := defaultPlanningWindowMinutes
	if v := os.Getenv(planningWindowMinutesEnv); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			planningWindow = parsed
		}
	}

	return &ThrottleConfig{
		RequestCapPerMinute:   requestCap,
		ConfirmWindowMinutes:  confirmWindow,
		PlanningWindowMinutes: planningWindow,
	}
}
