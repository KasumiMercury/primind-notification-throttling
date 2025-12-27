package metrics

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

type Config struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
}

type Provider struct {
	mp *sdkmetric.MeterProvider
}

func (p *Provider) Shutdown(ctx context.Context) error {
	if p.mp == nil {
		return nil
	}

	return p.mp.Shutdown(ctx)
}

func (p *Provider) MeterProvider() metric.MeterProvider {
	return p.mp
}

func (p *Provider) SetGlobalProvider() {
	otel.SetMeterProvider(p.mp)
}
