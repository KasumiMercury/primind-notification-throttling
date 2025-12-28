//go:build !gcloud

package tracing

import (
	"context"
	"errors"
	"os"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.38.0"
)

func NewProvider(ctx context.Context, cfg Config) (*Provider, error) {
	if os.Getenv("OTEL_EXPORTER_DISABLED") == "true" {
		return newNoopProvider(cfg), nil
	}

	// When exporter is enabled, endpoint is required
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		return nil, errors.New("OTEL_EXPORTER_OTLP_ENDPOINT is required when OTEL_EXPORTER_DISABLED is not set to 'true'")
	}

	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(endpoint),
	}

	// Use insecure connection for local development
	if os.Getenv("OTEL_EXPORTER_OTLP_INSECURE") != "false" {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	exporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(cfg.ServiceName),
		semconv.ServiceVersion(cfg.ServiceVersion),
		semconv.DeploymentEnvironmentName(cfg.Environment),
	)

	var sampler sdktrace.Sampler
	if cfg.SamplingRate >= 1.0 {
		sampler = sdktrace.AlwaysSample()
	} else if cfg.SamplingRate <= 0.0 {
		sampler = sdktrace.NeverSample()
	} else {
		sampler = sdktrace.TraceIDRatioBased(cfg.SamplingRate)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	return &Provider{tp: tp}, nil
}

func newNoopProvider(cfg Config) *Provider {
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(cfg.ServiceName),
		semconv.ServiceVersion(cfg.ServiceVersion),
		semconv.DeploymentEnvironmentName(cfg.Environment),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.NeverSample()),
	)

	return &Provider{tp: tp}
}
