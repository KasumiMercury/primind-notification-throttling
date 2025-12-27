package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type Config struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	SamplingRate   float64 // 1.0 = always sample, 0.0 = never sample
}

type Provider struct {
	tp *sdktrace.TracerProvider
}

func (p *Provider) Shutdown(ctx context.Context) error {
	if p.tp == nil {
		return nil
	}

	return p.tp.Shutdown(ctx)
}

func (p *Provider) TracerProvider() trace.TracerProvider {
	return p.tp
}

func SetupPropagator() {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
}

func (p *Provider) SetGlobalProvider() {
	otel.SetTracerProvider(p.tp)
}
