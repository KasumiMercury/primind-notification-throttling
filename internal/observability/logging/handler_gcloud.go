//go:build gcloud

package logging

import (
	"context"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// gcpTraceAttrs returns GCP Cloud Logging trace correlation attributes.
func gcpTraceAttrs(ctx context.Context, projectID string) []slog.Attr {
	if projectID == "" {
		return nil
	}

	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return []slog.Attr{
			slog.String("logging.googleapis.com/trace", ""),
			slog.String("logging.googleapis.com/spanId", ""),
			slog.Bool("logging.googleapis.com/trace_sampled", false),
		}
	}

	sc := span.SpanContext()

	return []slog.Attr{
		slog.String("logging.googleapis.com/trace",
			fmt.Sprintf("projects/%s/traces/%s", projectID, sc.TraceID().String())),
		slog.String("logging.googleapis.com/spanId", sc.SpanID().String()),
		slog.Bool("logging.googleapis.com/trace_sampled", sc.IsSampled()),
	}
}
