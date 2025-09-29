package telemetry

import (
	"fmt"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// Units are encoded according to the case-sensitive abbreviations from the
// Unified Code for Units of Measure: http://unitsofmeasure.org/ucum.html.
const (
	unitDimensionless = "1"
	unitMilliseconds  = "ms"
	unitBytes         = "B" // Changed from "By" to "B"
)

var (
	defaultMillisecondsBoundaries = []float64{ //nolint:gochecknoglobals // OpenTelemetry histogram boundaries must be global for reuse
		0.0,
		0.1,
		0.2,
		0.4,
		0.6,
		0.8,
		1.0,
		2.0,
		3.0,
		4.0,
		5.0,
		6.0,
		8.0,
		10.0,
		13.0,
		16.0,
		20.0,
		25.0,
		30.0,
		40.0,
		50.0,
		65.0,
		80.0,
		100.0,
		130.0,
		160.0,
		200.0,
		250.0,
		300.0,
		400.0,
		500.0,
		650.0,
		800.0,
		1000.0,
		2000.0,
		5000.0,
		10000.0,
	}
)

func Views(pkg string) []sdkmetric.View {
	return []sdkmetric.View{

		// View for latency histogram.
		func(inst sdkmetric.Instrument) (sdkmetric.Stream, bool) {
			if inst.Kind == sdkmetric.InstrumentKindHistogram {
				if inst.Name == pkg+"/latency" {
					return sdkmetric.Stream{
						Name:        inst.Name,
						Description: "Distribution of method latency, by provider and method.",
						Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
							Boundaries: defaultMillisecondsBoundaries,
						},
						AttributeFilter: func(kv attribute.KeyValue) bool {
							return kv.Key == packageKey || kv.Key == methodKey
						},
					}, true
				}
			}

			return sdkmetric.Stream{}, false
		},

		// View for completed_calls count.
		func(inst sdkmetric.Instrument) (sdkmetric.Stream, bool) {
			if inst.Kind == sdkmetric.InstrumentKindHistogram {
				if inst.Name == pkg+"/latency" {
					return sdkmetric.Stream{
						Name:        strings.Replace(inst.Name, "/latency", "/completed_calls", 1),
						Description: "Count of method calls by provider, method and status.",
						Aggregation: sdkmetric.DefaultAggregationSelector(sdkmetric.InstrumentKindCounter),
						AttributeFilter: func(kv attribute.KeyValue) bool {
							return kv.Key == methodKey || kv.Key == statusKey
						},
					}, true
				}
			}
			return sdkmetric.Stream{}, false
		},
	}
}

// CounterView returns summation views that add up individual measurements the counter takes.
func CounterView(pkg string, meterName string, description string) []sdkmetric.View {
	return []sdkmetric.View{
		// View for gauge counts.
		func(inst sdkmetric.Instrument) (sdkmetric.Stream, bool) {
			if inst.Kind == sdkmetric.InstrumentKindCounter {
				if inst.Name == pkg+meterName {
					return sdkmetric.Stream{
						Name:        inst.Name,
						Description: description,
						Aggregation: sdkmetric.DefaultAggregationSelector(sdkmetric.InstrumentKindCounter),
					}, true
				}
			}
			return sdkmetric.Stream{}, false
		},
	}
}

// LatencyMeasure returns the measure for method call latency used by Go CDK APIs.
func LatencyMeasure(pkg string) metric.Float64Histogram {
	attrs := []attribute.KeyValue{
		packageKey.String(pkg),
	}

	pkgMeter := otel.Meter(pkg, metric.WithInstrumentationAttributes(attrs...))

	m, err := pkgMeter.Float64Histogram(
		pkg+"/latency",
		metric.WithDescription("Latency distribution of method calls"),
		metric.WithUnit(unitMilliseconds),
	)

	if err != nil {
		// The only possible errors are from invalid key or value names, and those are programming
		// errors that will be found during testing.
		panic(fmt.Sprintf("fullName=%q, provider=%q: %v", pkg, pkgMeter, err))
	}

	return m
}

// DimensionlessMeasure creates a simple counter specifically for dimensionless measurements.
func DimensionlessMeasure(pkg string, meterName string, description string) metric.Int64Counter {
	attrs := []attribute.KeyValue{
		packageKey.String(pkg),
	}

	pkgMeter := otel.Meter(pkg, metric.WithInstrumentationAttributes(attrs...))

	m, err := pkgMeter.Int64Counter(
		pkg+meterName,
		metric.WithDescription(description),
		metric.WithUnit(unitDimensionless),
	)

	if err != nil {
		// The only possible errors are from invalid key or value names,
		// and those are programming errors that will be found during testing.
		panic(fmt.Sprintf("fullName=%q, provider=%q: %v", pkg, pkgMeter, err))
	}
	return m
}

// BytesMeasure creates a counter for bytes measurements.
func BytesMeasure(pkg string, meterName string, description string) metric.Int64Counter {
	attrs := []attribute.KeyValue{
		packageKey.String(pkg),
	}

	pkgMeter := otel.Meter(pkg, metric.WithInstrumentationAttributes(attrs...))
	m, err := pkgMeter.Int64Counter(pkg+meterName, metric.WithDescription(description), metric.WithUnit(unitBytes))

	if err != nil {
		// The only possible errors are from invalid key or value names, and those are programming
		// errors that will be found during testing.
		panic(fmt.Sprintf("fullName=%q, provider=%q: %v", pkg, pkgMeter, err))
	}
	return m
}
