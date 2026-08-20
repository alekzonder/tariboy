package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Metrics holds the instruments described in spec §14. A nil *Metrics is a
// valid no-op receiver (unit tests and the OTel-off path pass nil).
type Metrics struct {
	meter        metric.Meter
	iterations   metric.Int64Counter
	iterationDur metric.Float64Histogram
	tokens       metric.Int64Counter
	cost         metric.Float64Counter
	proxyLatency metric.Float64Histogram
}

func NewMetrics(m metric.Meter) (*Metrics, error) {
	iterations, err := m.Int64Counter("tariboy.iterations",
		metric.WithDescription("iterations by outcome"))
	if err != nil {
		return nil, err
	}
	iterationDur, err := m.Float64Histogram("tariboy.iteration.duration_ms",
		metric.WithDescription("iteration wall-clock duration"), metric.WithUnit("ms"))
	if err != nil {
		return nil, err
	}
	tokens, err := m.Int64Counter("tariboy.tokens",
		metric.WithDescription("AI tokens by direction"))
	if err != nil {
		return nil, err
	}
	cost, err := m.Float64Counter("tariboy.cost_usd",
		metric.WithDescription("AI cost in USD"))
	if err != nil {
		return nil, err
	}
	proxyLatency, err := m.Float64Histogram("tariboy.proxy.latency_ms",
		metric.WithDescription("AI proxy request latency"), metric.WithUnit("ms"))
	if err != nil {
		return nil, err
	}
	return &Metrics{meter: m, iterations: iterations, iterationDur: iterationDur,
		tokens: tokens, cost: cost, proxyLatency: proxyLatency}, nil
}

// RecordIteration counts one finished iteration by outcome and records its
// duration.
func (m *Metrics) RecordIteration(ctx context.Context, outcome string, durationMs float64) {
	if m == nil {
		return
	}
	attrs := metric.WithAttributes(attribute.String("outcome", outcome))
	m.iterations.Add(ctx, 1, attrs)
	m.iterationDur.Record(ctx, durationMs, attrs)
}

// RecordProxyRequest records one AI proxy request's tokens, cost and latency.
func (m *Metrics) RecordProxyRequest(ctx context.Context, status string, latencyMs float64, inTok, outTok int, costUSD float64) {
	if m == nil {
		return
	}
	st := metric.WithAttributes(attribute.String("status", status))
	m.tokens.Add(ctx, int64(inTok), metric.WithAttributes(attribute.String("direction", "input")))
	m.tokens.Add(ctx, int64(outTok), metric.WithAttributes(attribute.String("direction", "output")))
	m.cost.Add(ctx, costUSD, st)
	m.proxyLatency.Record(ctx, latencyMs, st)
}

// GaugeSource supplies the point-in-time readings for the observable gauges.
type GaugeSource struct {
	QueueDepth     func(context.Context) int64
	PluginsHealthy func(context.Context) int64
	ActiveAgents   func(context.Context) int64
}

// RegisterGauges installs the observable gauges (spec §14: queue depth, plugin
// health, active agents). No-op instruments when OTel is off.
func (m *Metrics) RegisterGauges(g GaugeSource) error {
	if m == nil {
		return nil
	}
	reg := func(name, desc string, read func(context.Context) int64) error {
		if read == nil {
			return nil
		}
		_, err := m.meter.Int64ObservableGauge(name, metric.WithDescription(desc),
			metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
				o.Observe(read(ctx))
				return nil
			}))
		return err
	}
	if err := reg("tariboy.channel.queue_depth", "unacked bus deliveries", g.QueueDepth); err != nil {
		return err
	}
	if err := reg("tariboy.plugins.healthy", "running plugins", g.PluginsHealthy); err != nil {
		return err
	}
	return reg("tariboy.agents.active", "running agent loops", g.ActiveAgents)
}
