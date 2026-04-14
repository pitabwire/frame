package telemetry_test

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// setGlobalTracerProvider swaps the global OTel TracerProvider and returns the
// previous one so tests can restore it during cleanup.
func setGlobalTracerProvider(tp trace.TracerProvider) trace.TracerProvider {
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	return prev
}
