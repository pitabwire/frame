// Copyright 2023-2026 Peter Bwire
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
)

// BusinessMetrics is the standard factory for product/business metrics.
// Every instrument it creates is tenant-scoped TRANSPARENTLY: each
// measurement automatically carries tenant_id and partition_id derived
// from the context's security claims (see TenantAttributes), so call
// sites cannot forget tenant attribution. Unauthenticated/system paths
// simply record without tenant attributes.
//
// Usage:
//
//	var bm = telemetry.NewBusinessMetrics("service-loans")
//	var loansCreated = bm.Counter("loans_created_total", "New loan accounts created")
//	...
//	loansCreated.Add(ctx, 1) // tenant_id/partition_id attached from ctx
type BusinessMetrics struct {
	meter metric.Meter
}

// NewBusinessMetrics returns a factory bound to the named meter on the
// global OTel meter provider (frame's telemetry manager sets it up).
func NewBusinessMetrics(meterName string) *BusinessMetrics {
	return &BusinessMetrics{meter: otel.Meter(meterName)}
}

// merged appends explicit attributes after the tenant attributes so an
// explicit value wins if a caller (unusually) supplies tenant_id itself.
func merged(ctx context.Context, attrs []attribute.KeyValue) metric.MeasurementOption {
	tenant := TenantAttributes(ctx)
	if len(attrs) == 0 && len(tenant) == 0 {
		return metric.WithAttributes()
	}
	all := make([]attribute.KeyValue, 0, len(tenant)+len(attrs))
	all = append(all, tenant...)
	all = append(all, attrs...)
	return metric.WithAttributes(all...)
}

// Counter is a tenant-scoped monotonic int64 counter.
type Counter struct{ inner metric.Int64Counter }

// Add increments the counter; tenant attributes come from ctx.
func (c Counter) Add(ctx context.Context, incr int64, attrs ...attribute.KeyValue) {
	c.inner.Add(ctx, incr, merged(ctx, attrs))
}

// FloatCounter is a tenant-scoped monotonic float64 counter (amounts).
type FloatCounter struct{ inner metric.Float64Counter }

// Add increments the counter; tenant attributes come from ctx.
func (c FloatCounter) Add(ctx context.Context, incr float64, attrs ...attribute.KeyValue) {
	c.inner.Add(ctx, incr, merged(ctx, attrs))
}

// Histogram is a tenant-scoped float64 histogram.
type Histogram struct{ inner metric.Float64Histogram }

// Record records a value; tenant attributes come from ctx.
func (h Histogram) Record(ctx context.Context, value float64, attrs ...attribute.KeyValue) {
	h.inner.Record(ctx, value, merged(ctx, attrs))
}

// Gauge is a tenant-scoped int64 gauge (last-value).
type Gauge struct{ inner metric.Int64Gauge }

// Record sets the gauge; tenant attributes come from ctx.
func (g Gauge) Record(ctx context.Context, value int64, attrs ...attribute.KeyValue) {
	g.inner.Record(ctx, value, merged(ctx, attrs))
}

// FloatGauge is a tenant-scoped float64 gauge (last-value).
type FloatGauge struct{ inner metric.Float64Gauge }

// Record sets the gauge; tenant attributes come from ctx.
func (g FloatGauge) Record(ctx context.Context, value float64, attrs ...attribute.KeyValue) {
	g.inner.Record(ctx, value, merged(ctx, attrs))
}

// Counter creates a tenant-scoped counter. On instrument-creation error
// it returns a no-op-backed instrument (matching LatencyMeasure's
// fallback behaviour) — metrics must never break the service.
func (b *BusinessMetrics) Counter(name, description string, opts ...metric.Int64CounterOption) Counter {
	allOpts := append([]metric.Int64CounterOption{
		metric.WithDescription(description),
		metric.WithUnit(unitDimensionless),
	}, opts...)
	m, err := b.meter.Int64Counter(name, allOpts...)
	if err != nil {
		m, _ = noop.NewMeterProvider().Meter("").Int64Counter(name)
	}
	return Counter{inner: m}
}

// FloatCounter creates a tenant-scoped float64 counter, suited to
// monetary amounts in major units.
func (b *BusinessMetrics) FloatCounter(name, description string, opts ...metric.Float64CounterOption) FloatCounter {
	allOpts := append([]metric.Float64CounterOption{
		metric.WithDescription(description),
		metric.WithUnit(unitDimensionless),
	}, opts...)
	m, err := b.meter.Float64Counter(name, allOpts...)
	if err != nil {
		m, _ = noop.NewMeterProvider().Meter("").Float64Counter(name)
	}
	return FloatCounter{inner: m}
}

// Histogram creates a tenant-scoped histogram in milliseconds by default;
// pass metric.WithUnit to override.
func (b *BusinessMetrics) Histogram(name, description string, opts ...metric.Float64HistogramOption) Histogram {
	allOpts := append([]metric.Float64HistogramOption{
		metric.WithDescription(description),
		metric.WithUnit(unitMilliseconds),
	}, opts...)
	m, err := b.meter.Float64Histogram(name, allOpts...)
	if err != nil {
		m, _ = noop.NewMeterProvider().Meter("").Float64Histogram(name)
	}
	return Histogram{inner: m}
}

// Gauge creates a tenant-scoped int64 gauge.
func (b *BusinessMetrics) Gauge(name, description string, opts ...metric.Int64GaugeOption) Gauge {
	allOpts := append([]metric.Int64GaugeOption{
		metric.WithDescription(description),
		metric.WithUnit(unitDimensionless),
	}, opts...)
	m, err := b.meter.Int64Gauge(name, allOpts...)
	if err != nil {
		m, _ = noop.NewMeterProvider().Meter("").Int64Gauge(name)
	}
	return Gauge{inner: m}
}

// FloatGauge creates a tenant-scoped float64 gauge.
func (b *BusinessMetrics) FloatGauge(name, description string, opts ...metric.Float64GaugeOption) FloatGauge {
	allOpts := append([]metric.Float64GaugeOption{
		metric.WithDescription(description),
		metric.WithUnit(unitDimensionless),
	}, opts...)
	m, err := b.meter.Float64Gauge(name, allOpts...)
	if err != nil {
		m, _ = noop.NewMeterProvider().Meter("").Float64Gauge(name)
	}
	return FloatGauge{inner: m}
}
