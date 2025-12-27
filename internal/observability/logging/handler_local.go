//go:build !gcloud

package logging

import (
	"context"
	"log/slog"
)

// gcpTraceAttrs returns empty for non-GCP environments.
func gcpTraceAttrs(_ context.Context, _ string) []slog.Attr {
	return nil
}
