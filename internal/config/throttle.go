package config

import (
	"os"
	"strconv"
)

const (
	requestCapPerMinuteEnv   = "REQUEST_CAP_PER_MINUTE"
	confirmWindowMinutesEnv  = "CONFIRM_WINDOW_MINUTES"
	planningWindowMinutesEnv = "PLANNING_WINDOW_MINUTES"
	toleranceEnv             = "TOLERANCE"

	defaultRequestCapPerMinute   = 60
	defaultConfirmWindowMinutes  = 5
	defaultPlanningWindowMinutes = 30
	defaultTolerance             = 0.2 // 20% tolerance
)

type ThrottleConfig struct {
	RequestCapPerMinute   int
	ConfirmWindowMinutes  int
	PlanningWindowMinutes int
	Tolerance             float64 // Tolerance for ramp-up deviation (0.0 - 1.0)
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

	tolerance := defaultTolerance
	if v := os.Getenv(toleranceEnv); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil && parsed >= 0.0 && parsed <= 1.0 {
			tolerance = parsed
		}
	}

	return &ThrottleConfig{
		RequestCapPerMinute:   requestCap,
		ConfirmWindowMinutes:  confirmWindow,
		PlanningWindowMinutes: planningWindow,
		Tolerance:             tolerance,
	}
}
