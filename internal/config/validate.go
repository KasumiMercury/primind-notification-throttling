package config

import "errors"

func ValidateForRun(cfg *Config) error {
	if cfg.RemindTimeManagementURL == "" {
		return errors.New("REMIND_TIME_MANAGEMENT_URL environment variable is required")
	}
	return nil
}
