//go:build gcloud

package config

import (
	"errors"
	"fmt"
)

func (c *TaskQueueConfig) Validate() error {
	var errs []error

	if c.GCloudProjectID == "" {
		errs = append(errs, errors.New("GCLOUD_PROJECT_ID is required"))
	}
	if c.GCloudLocationID == "" {
		errs = append(errs, errors.New("GCLOUD_LOCATION_ID is required"))
	}
	if c.GCloudQueueID == "" {
		errs = append(errs, errors.New("GCLOUD_QUEUE_ID is required"))
	}
	if c.GCloudTargetURL == "" {
		errs = append(errs, errors.New("GCLOUD_TARGET_URL is required"))
	}

	if len(errs) > 0 {
		return fmt.Errorf("task queue configuration errors: %w", errors.Join(errs...))
	}

	return nil
}
