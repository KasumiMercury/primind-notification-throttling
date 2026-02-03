//go:build !gcloud

package timemgmt

import (
	"net/http"
	"time"
)

// newHTTPClient creates a plain HTTP client for local development.
func newHTTPClient(_ string) *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
	}
}
