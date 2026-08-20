// Package telemetry wires OpenTelemetry (spec §14): a trace per iteration and
// OTLP-exported metrics. Setup installs global trace/meter providers driven by
// a config endpoint; with no endpoint it installs no-op providers so the whole
// system runs with zero telemetry overhead (the default).
package telemetry

import (
	"context"
	"log/slog"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

// Config is the telemetry export configuration (spec §14). Endpoint is an
// OTLP/HTTP host:port; empty disables all export.
type Config struct {
	Endpoint    string
	ServiceName string
}

// Providers owns the installed OTel providers and their flush/shutdown.
type Providers struct {
	enabled  bool
	shutdown []func(context.Context) error
}

func (p *Providers) Enabled() bool { return p.enabled }

// Shutdown force-flushes and stops every installed provider. Safe on a
// disabled Providers (no-op).
func (p *Providers) Shutdown(ctx context.Context) error {
	var first error
	for _, fn := range p.shutdown {
		if err := fn(ctx); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// suppressOTLPEndpointEnv temporarily unsets OTEL_EXPORTER_OTLP_ENDPOINT (and
// its per-signal overrides) so the OTLP exporters' own env-based config reader
// can't choke on our scheme-less host:port value. Returns a func that restores
// whatever was there before. Setup is called once at daemon startup, before
// any concurrent readers of these env vars exist, so the brief mutation is
// safe.
func suppressOTLPEndpointEnv() func() {
	keys := []string{
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
		"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT",
	}
	saved := make(map[string]string, len(keys))
	had := make(map[string]bool, len(keys))
	for _, k := range keys {
		if v, ok := os.LookupEnv(k); ok {
			saved[k], had[k] = v, true
			os.Unsetenv(k)
		}
	}
	return func() {
		for _, k := range keys {
			if had[k] {
				os.Setenv(k, saved[k])
			}
		}
	}
}

// Setup installs the global trace + meter providers. With an empty endpoint it
// installs no-op providers (the default: zero overhead). With an endpoint it
// installs OTLP/HTTP exporters over an insecure local connection.
func Setup(ctx context.Context, cfg Config, log *slog.Logger) (*Providers, error) {
	if cfg.ServiceName == "" {
		cfg.ServiceName = "tariboy"
	}
	if cfg.Endpoint == "" {
		otel.SetTracerProvider(tracenoop.NewTracerProvider())
		otel.SetMeterProvider(metricnoop.NewMeterProvider())
		return &Providers{enabled: false}, nil
	}

	res := resource.NewSchemaless(attribute.String("service.name", cfg.ServiceName))

	// cfg.Endpoint is a scheme-less host:port (what otlptracehttp/otlpmetrichttp's
	// WithEndpoint wants), but both exporters' New() also self-configure from the
	// OTEL_EXPORTER_OTLP_ENDPOINT env var via an internal envconfig reader that
	// expects a full URL and url.Parse-fails on a bare host:port, logging a
	// "parse url" warning through otel's global error handler -- even though our
	// explicit WithEndpoint below always wins. Suppress that env var for the
	// duration of both New() calls so it can't be mis-parsed.
	restoreEnv := suppressOTLPEndpointEnv()
	traceExp, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(cfg.Endpoint), otlptracehttp.WithInsecure())
	if err != nil {
		restoreEnv()
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(traceExp, sdktrace.WithBatchTimeout(time.Second)),
	)
	otel.SetTracerProvider(tp)

	metricExp, err := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpoint(cfg.Endpoint), otlpmetrichttp.WithInsecure())
	restoreEnv()
	if err != nil {
		_ = tp.Shutdown(ctx)
		return nil, err
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp,
			sdkmetric.WithInterval(2*time.Second))),
	)
	otel.SetMeterProvider(mp)

	log.Info("telemetry OTLP export enabled", "endpoint", cfg.Endpoint)
	// Shutdown force-flushes to the collector. A transport failure flushing the
	// final batch to a down collector is expected and must not fail graceful
	// shutdown (the trace BatchSpanProcessor already logs-and-swallows such
	// export errors); we mirror that for the metric reader by logging instead of
	// propagating, so shutdown stays clean when the endpoint is unreachable.
	flush := func(name string, fn func(context.Context) error) func(context.Context) error {
		return func(ctx context.Context) error {
			if err := fn(ctx); err != nil {
				log.Warn("telemetry flush on shutdown failed", "provider", name, "error", err)
			}
			return nil
		}
	}
	return &Providers{enabled: true, shutdown: []func(context.Context) error{
		flush("trace", tp.Shutdown), flush("metric", mp.Shutdown),
	}}, nil
}
