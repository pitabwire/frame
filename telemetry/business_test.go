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

package telemetry_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/pitabwire/frame/v2/security"
	"github.com/pitabwire/frame/v2/telemetry"
)

func tenantContext(tenantID, partitionID string) context.Context {
	claims := &security.AuthenticationClaims{TenantID: tenantID, PartitionID: partitionID}
	claims.Subject = "user-" + tenantID
	return claims.ClaimsToContext(context.Background())
}

func collect(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
	return rm
}

func dataPointAttrs(data metricdata.Aggregation) []attribute.Set {
	switch d := data.(type) {
	case metricdata.Sum[int64]:
		out := make([]attribute.Set, 0, len(d.DataPoints))
		for _, dp := range d.DataPoints {
			out = append(out, dp.Attributes)
		}
		return out
	case metricdata.Sum[float64]:
		out := make([]attribute.Set, 0, len(d.DataPoints))
		for _, dp := range d.DataPoints {
			out = append(out, dp.Attributes)
		}
		return out
	case metricdata.Histogram[float64]:
		out := make([]attribute.Set, 0, len(d.DataPoints))
		for _, dp := range d.DataPoints {
			out = append(out, dp.Attributes)
		}
		return out
	default:
		return nil
	}
}

func attrSets(rm metricdata.ResourceMetrics, name string) []attribute.Set {
	var out []attribute.Set
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				out = append(out, dataPointAttrs(m.Data)...)
			}
		}
	}
	return out
}

// Business instruments must attach tenant_id/partition_id from the
// context transparently — no call-site opt-in — while claim-less
// (system) measurements record without tenant attributes, and explicit
// per-call attributes are preserved alongside.
func TestBusinessMetricsTenantScopedTransparently(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() { otel.SetMeterProvider(prev) })

	bm := telemetry.NewBusinessMetrics("biz-test")
	created := bm.Counter("things_created_total", "things created")
	amount := bm.FloatCounter("things_amount_total", "amount of things")
	dur := bm.Histogram("things_duration_ms", "thing duration")

	ctxA := tenantContext("tenant-a", "part-a")
	created.Add(ctxA, 1, attribute.String("kind", "gold"))
	amount.Add(ctxA, 12.5)
	dur.Record(ctxA, 42.0)

	// System path: no claims, no tenant attributes.
	created.Add(context.Background(), 1)

	rm := collect(t, reader)

	sets := attrSets(rm, "things_created_total")
	require.Len(t, sets, 2, "tenant-scoped and system datapoints are distinct series")

	var tenantSet, systemSet *attribute.Set
	for i := range sets {
		if _, ok := sets[i].Value("tenant_id"); ok {
			tenantSet = &sets[i]
		} else {
			systemSet = &sets[i]
		}
	}
	require.NotNil(t, tenantSet, "claims context must yield tenant_id")
	require.NotNil(t, systemSet, "claim-less context must omit tenant attributes")

	tid, _ := tenantSet.Value("tenant_id")
	pid, _ := tenantSet.Value("partition_id")
	kind, _ := tenantSet.Value("kind")
	require.Equal(t, "tenant-a", tid.AsString())
	require.Equal(t, "part-a", pid.AsString())
	require.Equal(t, "gold", kind.AsString(), "explicit attributes preserved alongside tenant attributes")

	for _, name := range []string{"things_amount_total", "things_duration_ms"} {
		s := attrSets(rm, name)
		require.NotEmpty(t, s, name)
		v, ok := s[0].Value("tenant_id")
		require.True(t, ok, name+" must carry tenant_id")
		require.Equal(t, "tenant-a", v.AsString())
	}
}
