package observability

import (
	"context"
	"io"
	"log/slog"
	"os"

	"github.com/KasumiMercury/primind-notification-throttling/internal/observability/logging"
	"github.com/KasumiMercury/primind-notification-throttling/internal/observability/metrics"
	"github.com/KasumiMercury/primind-notification-throttling/internal/observability/tracing"
)

type Config struct {
	ServiceInfo   logging.ServiceInfo
	Environment   logging.Environment
	GCPProjectID  string  // empty for non-GCP environments
	SamplingRate  float64 // 1.0 = always sample
	DefaultModule logging.Module
}

type Resources struct {
	logger         *slog.Logger
	tracerProvider *tracing.Provider
	meterProvider  *metrics.Provider
}

func Init(ctx context.Context, cfg Config) (*Resources, error) {
	// default sampling rate
	if cfg.SamplingRate == 0 {
		cfg.SamplingRate = 1.0
	}

	tracing.SetupPropagator()

	tp, err := tracing.NewProvider(ctx, tracing.Config{
		ServiceName:    cfg.ServiceInfo.Name,
		ServiceVersion: cfg.ServiceInfo.Version,
		Environment:    string(cfg.Environment),
		SamplingRate:   cfg.SamplingRate,
	})
	if err != nil {
		return nil, err
	}

	tp.SetGlobalProvider()

	mp, err := metrics.NewProvider(ctx, metrics.Config{
		ServiceName:    cfg.ServiceInfo.Name,
		ServiceVersion: cfg.ServiceInfo.Version,
		Environment:    string(cfg.Environment),
	})
	if err != nil {
		return nil, err
	}

	mp.SetGlobalProvider()

	handler := logging.NewHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}, logging.HandlerConfig{
		ServiceInfo:   cfg.ServiceInfo,
		Environment:   cfg.Environment,
		GCPProject:    cfg.GCPProjectID,
		DefaultModule: cfg.DefaultModule,
	})

	logger := slog.New(handler)

	return &Resources{
		logger:         logger,
		tracerProvider: tp,
		meterProvider:  mp,
	}, nil
}

func (r *Resources) Logger() *slog.Logger {
	return r.logger
}

func (r *Resources) Shutdown(ctx context.Context) error {
	var errs []error

	if r.meterProvider != nil {
		if err := r.meterProvider.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}

	if r.tracerProvider != nil {
		if err := r.tracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errs[0]
	}

	return nil
}

func NewLogger(w io.Writer, cfg Config) *slog.Logger {
	handler := logging.NewHandler(w, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}, logging.HandlerConfig{
		ServiceInfo:   cfg.ServiceInfo,
		Environment:   cfg.Environment,
		GCPProject:    cfg.GCPProjectID,
		DefaultModule: cfg.DefaultModule,
	})

	return slog.New(handler)
}
