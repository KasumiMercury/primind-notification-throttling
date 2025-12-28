//go:build !gcloud

package metrics

import (
	"context"
	"errors"
	"os"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.38.0"
)

func NewProvider(ctx context.Context, cfg Config) (*Provider, error) {
	if os.Getenv("OTEL_EXPORTER_DISABLED") == "true" {
		return newNoopProvider(cfg), nil
	}

	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT")
	if endpoint == "" {
		endpoint = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	}
	if endpoint == "" {
		return nil, errors.New("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT or OTEL_EXPORTER_OTLP_ENDPOINT is required when OTEL_EXPORTER_DISABLED is not set to 'true'")
	}

	opts := []otlpmetrichttp.Option{
		otlpmetrichttp.WithEndpoint(endpoint),
		otlpmetrichttp.WithURLPath("/api/v1/otlp/v1/metrics"),
	}

	if os.Getenv("OTEL_EXPORTER_OTLP_INSECURE") != "false" {
		opts = append(opts, otlpmetrichttp.WithInsecure())
	}

	exporter, err := otlpmetrichttp.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(cfg.ServiceName),
		semconv.ServiceVersion(cfg.ServiceVersion),
		semconv.DeploymentEnvironmentName(cfg.Environment),
	)

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter,
			sdkmetric.WithInterval(15*time.Second))),
		sdkmetric.WithResource(res),
	)

	return &Provider{mp: mp}, nil
}

func newNoopProvider(cfg Config) *Provider {
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(cfg.ServiceName),
		semconv.ServiceVersion(cfg.ServiceVersion),
		semconv.DeploymentEnvironmentName(cfg.Environment),
	)

	// MeterProvider without any reader does not export metrics
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
	)

	return &Provider{mp: mp}
}
