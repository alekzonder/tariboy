package telemetry

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestMetricsRecordAndGauges(t *testing.T) {
	// A manual reader lets the test collect metrics deterministically (no export).
	reader := metric.NewManualReader()
	mp := metric.NewMeterProvider(metric.WithReader(reader))
	m, err := NewMetrics(mp.Meter("tariboy"))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterGauges(GaugeSource{
		QueueDepth:     func(context.Context) int64 { return 7 },
		PluginsHealthy: func(context.Context) int64 { return 2 },
		ActiveAgents:   func(context.Context) int64 { return 3 },
	}); err != nil {
		t.Fatal(err)
	}
	m.RecordIteration(context.Background(), "done", 120)
	m.RecordIteration(context.Background(), "timeout", 500)
	m.RecordProxyRequest(context.Background(), "ok", 42, 100, 50, 0.00175)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, sm := range rm.ScopeMetrics {
		for _, mm := range sm.Metrics {
			got[mm.Name] = true
		}
	}
	for _, want := range []string{
		"tariboy.iterations", "tariboy.iteration.duration_ms",
		"tariboy.tokens", "tariboy.cost_usd", "tariboy.proxy.latency_ms",
		"tariboy.channel.queue_depth", "tariboy.plugins.healthy", "tariboy.agents.active",
	} {
		if !got[want] {
			t.Errorf("metric %q not collected", want)
		}
	}
	// A nil Metrics records nothing and never panics.
	var nilM *Metrics
	nilM.RecordIteration(context.Background(), "done", 1)
	nilM.RecordProxyRequest(context.Background(), "ok", 1, 1, 1, 1)
	_ = attribute.String // keep import if unused across edits
}
