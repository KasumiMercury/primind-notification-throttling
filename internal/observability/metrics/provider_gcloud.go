//go:build gcloud

package metrics

import (
	"context"
	"os"

	mexporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.38.0"
)

func NewProvider(ctx context.Context, cfg Config) (*Provider, error) {
	if os.Getenv("OTEL_EXPORTER_DISABLED") == "true" {
		return newNoopProvider(cfg), nil
	}

	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		projectID = os.Getenv("GCLOUD_PROJECT_ID")
	}

	exporter, err := mexporter.New(mexporter.WithProjectID(projectID))
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
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
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
