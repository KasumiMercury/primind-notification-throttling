//go:build !gcloud

package config

func (c *TaskQueueConfig) Validate() error {
	return nil
}
