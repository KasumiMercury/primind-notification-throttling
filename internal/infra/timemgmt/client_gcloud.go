//go:build gcloud

package timemgmt

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"google.golang.org/api/idtoken"
)

// newHTTPClient creates an HTTP client with GCP ID token authentication.
func newHTTPClient(baseURL string) *http.Client {
	httpClient, err := idtoken.NewClient(context.Background(), baseURL)
	if err != nil {
		slog.Error("failed to create idtoken client, falling back to unauthenticated client",
			slog.String("error", err.Error()),
		)
		return &http.Client{
			Timeout: 30 * time.Second,
		}
	}
	return httpClient
}
