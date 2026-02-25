package blueprint

import (
	"fmt"
	"strings"
)

func (bp *Blueprint) Explain() (string, error) {
	services, err := bp.normalizedServices()
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Schema: %s\n", bp.SchemaVersion)
	fmt.Fprintf(&b, "Services: %d\n", len(services))
	for _, svc := range services {
		fmt.Fprintf(&b, "- %s (port %s)\n", svc.Name, defaultPort(svc.Port))
		if len(svc.HTTP) > 0 {
			fmt.Fprintf(&b, "  HTTP routes: %d\n", len(svc.HTTP))
		}
		if len(svc.Queues) > 0 {
			fmt.Fprintf(&b, "  Queues: %d\n", len(svc.Queues))
		}
		if len(svc.Plugins) > 0 {
			fmt.Fprintf(&b, "  Plugins: %s\n", strings.Join(svc.Plugins, ", "))
		}
	}

	return b.String(), nil
}

func defaultPort(port string) string {
	if strings.TrimSpace(port) == "" {
		return ":8080"
	}
	return port
}
