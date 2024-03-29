package frame

import (
	"context"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.10.0"
)

func (s *Service) initTracer(ctx context.Context) error {
	if s.traceExporter != nil {
		res, err := resource.Merge(
			resource.Default(),
			resource.NewWithAttributes(
				semconv.SchemaURL,
				semconv.ServiceNameKey.String(s.name),
				semconv.ServiceVersionKey.String("v0.1.0"),
				attribute.String("environment", "demo"),
			),
		)

		if err != nil {
			return err
		}

		if s.traceSampler == nil {
			s.traceSampler = sdktrace.AlwaysSample()
		}

		tp := sdktrace.NewTracerProvider(
			sdktrace.WithSampler(s.traceSampler),
			sdktrace.WithSyncer(s.traceExporter),
			sdktrace.WithResource(res))

		otel.SetTracerProvider(tp)
		otel.SetTextMapPropagator(
			propagation.NewCompositeTextMapPropagator(
				propagation.TraceContext{},
				propagation.Baggage{},
			))
	}

	return nil
}

// TraceExporter Option that specify the trace exporter to use
func TraceExporter(exporter sdktrace.SpanExporter) Option {
	return func(s *Service) {
		s.traceExporter = exporter
	}
}

// TraceSampler Option that specify the trace sampler to use
func TraceSampler(sampler sdktrace.Sampler) Option {
	return func(s *Service) {
		s.traceSampler = sampler
	}
}
