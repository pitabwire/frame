package blueprint

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var handlerNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (bp *Blueprint) Validate() error {
	if bp == nil {
		return errors.New("blueprint is nil")
	}
	if strings.TrimSpace(bp.SchemaVersion) == "" {
		return errors.New("schema_version is required")
	}

	services, err := bp.normalizedServices()
	if err != nil {
		return err
	}
	if len(services) == 0 {
		return errors.New("no services defined")
	}

	for i, svc := range services {
		if strings.TrimSpace(svc.Name) == "" {
			return fmt.Errorf("service[%d].name is required", i)
		}
		for j, route := range svc.HTTP {
			if strings.TrimSpace(route.Route) == "" {
				return fmt.Errorf("service[%d].http[%d].route is required", i, j)
			}
			if strings.TrimSpace(route.Method) == "" {
				return fmt.Errorf("service[%d].http[%d].method is required", i, j)
			}
			if strings.TrimSpace(route.Handler) == "" {
				return fmt.Errorf("service[%d].http[%d].handler is required", i, j)
			}
			if !handlerNameRe.MatchString(route.Handler) {
				return fmt.Errorf("service[%d].http[%d].handler is not a valid identifier", i, j)
			}
		}
		for j, q := range svc.Queues {
			if strings.TrimSpace(q.URL) == "" {
				return fmt.Errorf("service[%d].queues[%d].url is required", i, j)
			}
			if strings.TrimSpace(q.Publisher) == "" && strings.TrimSpace(q.Subscriber) == "" {
				return fmt.Errorf("service[%d].queues[%d] must have publisher or subscriber", i, j)
			}
			if strings.TrimSpace(q.Subscriber) != "" {
				if strings.TrimSpace(q.Handler) == "" {
					return fmt.Errorf("service[%d].queues[%d].handler is required for subscriber", i, j)
				}
				if !handlerNameRe.MatchString(q.Handler) {
					return fmt.Errorf("service[%d].queues[%d].handler is not a valid identifier", i, j)
				}
			}
		}
	}

	return nil
}

func (bp *Blueprint) normalizedServices() ([]ServiceSpec, error) {
	var services []ServiceSpec

	if len(bp.Services) > 0 {
		services = append(services, bp.Services...)
	}
	if bp.Service != nil {
		services = append(services, *bp.Service)
	}
	if len(services) == 0 {
		if len(bp.HTTP) > 0 || len(bp.Plugins) > 0 || len(bp.Queues) > 0 || bp.Datastore != nil {
			return nil, errors.New("top-level http/plugins/queues/datastore require a service")
		}
		return nil, nil
	}

	for i := range services {
		if len(services[i].HTTP) == 0 && len(bp.HTTP) > 0 {
			services[i].HTTP = append([]HTTPRoute{}, bp.HTTP...)
		}
		if len(services[i].Plugins) == 0 && len(bp.Plugins) > 0 {
			services[i].Plugins = append([]string{}, bp.Plugins...)
		}
		if len(services[i].Queues) == 0 && len(bp.Queues) > 0 {
			services[i].Queues = append([]QueueSpec{}, bp.Queues...)
		}
		if services[i].Datastore == nil && bp.Datastore != nil {
			services[i].Datastore = bp.Datastore
		}
	}

	return services, nil
}
