package metrics

import (
	"context"
	"strconv"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	httpMeterName = "http.server"
)

type HTTPMetrics struct {
	requestCounter  metric.Int64Counter
	requestDuration metric.Float64Histogram
}

func NewHTTPMetrics() (*HTTPMetrics, error) {
	meter := otel.Meter(httpMeterName)

	requestCounter, err := meter.Int64Counter(
		"http_requests_total",
		metric.WithDescription("Total number of HTTP requests"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, err
	}

	requestDuration, err := meter.Float64Histogram(
		"http_request_duration_seconds",
		metric.WithDescription("HTTP request duration in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(
			0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
		),
	)
	if err != nil {
		return nil, err
	}

	return &HTTPMetrics{
		requestCounter:  requestCounter,
		requestDuration: requestDuration,
	}, nil
}

func (m *HTTPMetrics) Record(ctx context.Context, method, path string, statusCode int, duration time.Duration) {
	attrs := []attribute.KeyValue{
		attribute.String("method", method),
		attribute.String("path", path),
		attribute.String("status_code", strconv.Itoa(statusCode)),
	}

	m.requestCounter.Add(ctx, 1, metric.WithAttributes(attrs...))
	m.requestDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}
