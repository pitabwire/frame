package blueprint

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

func Merge(base, overlay Blueprint) (Blueprint, error) {
	out := base

	if err := mergeSchema(&out, overlay); err != nil {
		return Blueprint{}, err
	}

	if err := mergeRuntimeMode(&out, overlay); err != nil {
		return Blueprint{}, err
	}

	if overlay.Service != nil {
		if out.Service == nil {
			cloned := *overlay.Service
			out.Service = &cloned
		} else {
			merged, err := mergeServiceSpec(*out.Service, *overlay.Service)
			if err != nil {
				return Blueprint{}, err
			}
			out.Service = &merged
		}
	}

	if len(overlay.Services) > 0 {
		out.Services = mergeServices(out.Services, overlay.Services)
	}

	out.HTTP = mergeHTTP(out.HTTP, overlay.HTTP)
	out.Plugins = mergeStrings(out.Plugins, overlay.Plugins)
	out.Queues = mergeQueues(out.Queues, overlay.Queues)
	out.Datastore = mergeDatastore(out.Datastore, overlay.Datastore)

	return out, nil
}

func mergeSchema(out *Blueprint, overlay Blueprint) error {
	if strings.TrimSpace(overlay.SchemaVersion) == "" {
		return nil
	}
	if strings.TrimSpace(out.SchemaVersion) == "" {
		out.SchemaVersion = overlay.SchemaVersion
		return nil
	}
	if out.SchemaVersion != overlay.SchemaVersion {
		return fmt.Errorf("schema_version mismatch: base %q overlay %q", out.SchemaVersion, overlay.SchemaVersion)
	}
	return nil
}

func mergeRuntimeMode(out *Blueprint, overlay Blueprint) error {
	mode := strings.TrimSpace(overlay.RuntimeMode)
	if mode == "" {
		return nil
	}
	if strings.TrimSpace(out.RuntimeMode) == "" {
		out.RuntimeMode = mode
		return nil
	}
	if !strings.EqualFold(out.RuntimeMode, mode) {
		return fmt.Errorf("runtime_mode mismatch: base %q overlay %q", out.RuntimeMode, overlay.RuntimeMode)
	}
	return nil
}

func mergeServices(base, overlay []ServiceSpec) []ServiceSpec {
	index := make(map[string]int, len(base))
	out := make([]ServiceSpec, 0, len(base)+len(overlay))
	for _, svc := range base {
		name := strings.TrimSpace(svc.Name)
		if name == "" {
			continue
		}
		index[name] = len(out)
		out = append(out, svc)
	}

	for _, svc := range overlay {
		name := strings.TrimSpace(svc.Name)
		if name == "" {
			continue
		}
		if idx, ok := index[name]; ok {
			merged, err := mergeServiceSpec(out[idx], svc)
			if err != nil {
				continue
			}
			out[idx] = merged
			continue
		}
		index[name] = len(out)
		out = append(out, svc)
	}

	return out
}

func mergeServiceSpec(base, overlay ServiceSpec) (ServiceSpec, error) {
	name := strings.TrimSpace(overlay.Name)
	if name != "" && strings.TrimSpace(base.Name) != "" && base.Name != name {
		return ServiceSpec{}, fmt.Errorf("service mismatch: base %q overlay %q", base.Name, overlay.Name)
	}
	out := base
	if out.Name == "" {
		out.Name = overlay.Name
	}

	if err := mergeStringField("module", &out.Module, overlay.Module); err != nil {
		return ServiceSpec{}, err
	}
	if err := mergeStringField("runtime_mode", &out.RuntimeMode, overlay.RuntimeMode); err != nil {
		return ServiceSpec{}, err
	}
	if err := mergeStringField("service_group", &out.ServiceGroup, overlay.ServiceGroup); err != nil {
		return ServiceSpec{}, err
	}
	if err := mergeStringField("port", &out.Port, overlay.Port); err != nil {
		return ServiceSpec{}, err
	}

	if overlay.ServiceID != nil {
		if out.ServiceID == nil {
			out.ServiceID = overlay.ServiceID
		} else if !reflect.DeepEqual(out.ServiceID, overlay.ServiceID) {
			return ServiceSpec{}, errors.New("service_id mismatch")
		}
	}

	out.HTTP = mergeHTTP(out.HTTP, overlay.HTTP)
	out.Plugins = mergeStrings(out.Plugins, overlay.Plugins)
	out.Queues = mergeQueues(out.Queues, overlay.Queues)
	out.Datastore = mergeDatastore(out.Datastore, overlay.Datastore)

	return out, nil
}

func mergeDatastore(base, overlay *DatastoreSpec) *DatastoreSpec {
	if overlay == nil {
		return base
	}
	if base == nil {
		cloned := *overlay
		return &cloned
	}
	out := *base
	if strings.TrimSpace(overlay.PrimaryURLEnv) != "" &&
		strings.TrimSpace(out.PrimaryURLEnv) != "" &&
		overlay.PrimaryURLEnv != out.PrimaryURLEnv {
		return base
	}
	if out.PrimaryURLEnv == "" {
		out.PrimaryURLEnv = overlay.PrimaryURLEnv
	}
	if overlay.Migrate {
		out.Migrate = true
	}
	return &out
}

func mergeHTTP(base, overlay []HTTPRoute) []HTTPRoute {
	return mergeList(
		base,
		overlay,
		httpKey,
		func(_ HTTPRoute, src HTTPRoute) HTTPRoute { return src },
	)
}

func httpKey(item HTTPRoute) string {
	if strings.TrimSpace(item.Name) != "" {
		return strings.TrimSpace(item.Name)
	}
	return strings.ToUpper(strings.TrimSpace(item.Method)) + " " + strings.TrimSpace(item.Route)
}

func mergeQueues(base, overlay []QueueSpec) []QueueSpec {
	return mergeList(
		base,
		overlay,
		queueKey,
		func(_ QueueSpec, src QueueSpec) QueueSpec { return src },
	)
}

func queueKey(item QueueSpec) string {
	if strings.TrimSpace(item.Publisher) != "" {
		return "pub:" + strings.TrimSpace(item.Publisher) + ":" + strings.TrimSpace(item.URL)
	}
	return "sub:" +
		strings.TrimSpace(item.Subscriber) +
		":" +
		strings.TrimSpace(item.URL) +
		":" +
		strings.TrimSpace(item.Handler)
}

func mergeStrings(base, overlay []string) []string {
	seen := make(map[string]struct{}, len(base))
	out := make([]string, 0, len(base)+len(overlay))
	for _, item := range base {
		key := strings.TrimSpace(item)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	for _, item := range overlay {
		key := strings.TrimSpace(item)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

type keyFunc[T any] func(T) string
type replaceFunc[T any] func(dst, src T) T

func mergeList[T any](
	base, overlay []T,
	key keyFunc[T],
	replace replaceFunc[T],
) []T {
	index := make(map[string]int, len(base))
	out := make([]T, 0, len(base)+len(overlay))
	for _, item := range base {
		k := key(item)
		if k == "" {
			continue
		}
		index[k] = len(out)
		out = append(out, item)
	}

	for _, item := range overlay {
		k := key(item)
		if k == "" {
			continue
		}
		if idx, ok := index[k]; ok {
			out[idx] = replace(out[idx], item)
			continue
		}
		index[k] = len(out)
		out = append(out, item)
	}

	return out
}

func mergeStringField(field string, base *string, overlay string) error {
	if strings.TrimSpace(overlay) == "" {
		return nil
	}
	if strings.TrimSpace(*base) == "" {
		*base = overlay
		return nil
	}
	if *base != overlay {
		return fmt.Errorf("%s mismatch: base %q overlay %q", field, *base, overlay)
	}
	return nil
}
