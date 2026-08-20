package telemetry

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel"
)

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestSetupDisabledIsNoop(t *testing.T) {
	p, err := Setup(context.Background(), Config{Endpoint: "", ServiceName: "tariboy"}, discardLog())
	if err != nil {
		t.Fatal(err)
	}
	if p.Enabled() {
		t.Fatal("empty endpoint must be disabled")
	}
	// A span from the global tracer must be a no-op (never recording) when off.
	_, span := otel.Tracer("t").Start(context.Background(), "x")
	if span.SpanContext().IsValid() {
		t.Fatal("disabled span context should be invalid (no-op provider)")
	}
	span.End()
	// Shutdown of a disabled Providers is a clean no-op.
	if err := p.Shutdown(context.Background()); err != nil {
		t.Fatalf("disabled shutdown: %v", err)
	}
}

func TestSetupEnabledInstallsRecordingProvider(t *testing.T) {
	// A bogus endpoint is fine: Setup must not dial at construction time, only on
	// export. The provider must be recording so spans carry a valid context.
	p, err := Setup(context.Background(), Config{Endpoint: "127.0.0.1:4318", ServiceName: "tariboy"}, discardLog())
	if err != nil {
		t.Fatal(err)
	}
	if !p.Enabled() {
		t.Fatal("configured endpoint must be enabled")
	}
	_, span := otel.Tracer("t").Start(context.Background(), "x")
	if !span.SpanContext().IsValid() {
		t.Fatal("enabled span context should be valid (recording provider)")
	}
	span.End()
	if err := p.Shutdown(context.Background()); err != nil {
		t.Fatalf("enabled shutdown: %v", err)
	}
}
