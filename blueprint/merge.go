package blueprint

import "fmt"

func Merge(base, overlay Blueprint) (Blueprint, error) {
	out := base

	if overlay.Service != "" {
		if out.Service == "" {
			out.Service = overlay.Service
		} else if out.Service != overlay.Service {
			return Blueprint{}, fmt.Errorf("service mismatch: base %q overlay %q", out.Service, overlay.Service)
		}
	}

	out.HTTP = mergeHTTP(out.HTTP, overlay.HTTP)
	out.Plugins = mergePlugins(out.Plugins, overlay.Plugins)
	out.Queues = mergeQueues(out.Queues, overlay.Queues)

	return out, nil
}

func mergeHTTP(base, overlay []HTTPRoute) []HTTPRoute {
	return mergeList(
		base,
		overlay,
		func(item HTTPRoute) string { return item.Name },
		func(_ HTTPRoute, src HTTPRoute) HTTPRoute { return src },
		func(item HTTPRoute) bool { return item.Override },
		func(item HTTPRoute) bool { return item.Remove },
	)
}

func mergePlugins(base, overlay []Plugin) []Plugin {
	return mergeList(
		base,
		overlay,
		func(item Plugin) string { return item.Name },
		func(_ Plugin, src Plugin) Plugin { return src },
		func(item Plugin) bool { return item.Override },
		func(item Plugin) bool { return item.Remove },
	)
}

func mergeQueues(base, overlay []Queue) []Queue {
	return mergeList(
		base,
		overlay,
		func(item Queue) string { return item.Name },
		func(_ Queue, src Queue) Queue { return src },
		func(item Queue) bool { return item.Override },
		func(item Queue) bool { return item.Remove },
	)
}

type keyFunc[T any] func(T) string
type replaceFunc[T any] func(dst, src T) T

type flagFunc[T any] func(T) bool

func mergeList[T any](
	base, overlay []T,
	key keyFunc[T],
	replace replaceFunc[T],
	override flagFunc[T],
	remove flagFunc[T],
) []T {
	index := make(map[string]int, len(base))
	out := make([]T, 0, len(base)+len(overlay))
	for i, item := range base {
		k := key(item)
		if k == "" {
			continue
		}
		index[k] = len(out)
		out = append(out, item)
		_ = i
	}

	for _, item := range overlay {
		k := key(item)
		if k == "" {
			continue
		}
		if remove(item) {
			if idx, ok := index[k]; ok {
				out[idx] = *new(T)
				delete(index, k)
			}
			continue
		}
		if idx, ok := index[k]; ok {
			if override(item) {
				out[idx] = replace(out[idx], item)
			}
			continue
		}
		index[k] = len(out)
		out = append(out, item)
	}

	// Compact removed items
	clean := out[:0]
	for _, item := range out {
		if key(item) == "" {
			continue
		}
		clean = append(clean, item)
	}
	return clean
}
