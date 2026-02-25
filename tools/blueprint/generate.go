package blueprint

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type GenerateOptions struct {
	OutDir string
}

func (bp *Blueprint) Generate(opts GenerateOptions) error {
	if err := bp.Validate(); err != nil {
		return err
	}

	services, err := bp.normalizedServices()
	if err != nil {
		return err
	}

	outDir := strings.TrimSpace(opts.OutDir)
	if outDir == "" {
		outDir = "."
	}

	modulePath, _ := moduleFromGoMod(outDir)

	mode := bp.runtimeMode()
	if len(services) == 1 && mode != "monolith" {
		return generatePolylith(outDir, modulePath, services[0])
	}

	return generateMonolith(outDir, modulePath, services)
}

func generatePolylith(outDir, modulePath string, svc ServiceSpec) error {
	if strings.TrimSpace(svc.Name) == "" {
		return errors.New("service name is required")
	}

	appDir := filepath.Join(outDir, "apps", svc.Name)
	cmdDir := filepath.Join(appDir, "cmd")
	handlersDir := filepath.Join(outDir, "pkg", "handlers", svc.Name)
	queuesDir := filepath.Join(outDir, "pkg", "queues", svc.Name)
	pluginsDir := filepath.Join(outDir, "pkg", "plugins")

	// #nosec G301 -- scaffolding directories should be readable by developers.
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		return err
	}
	// #nosec G301 -- scaffolding directories should be readable by developers.
	if err := os.MkdirAll(handlersDir, 0o755); err != nil {
		return err
	}
	// #nosec G301 -- scaffolding directories should be readable by developers.
	if err := os.MkdirAll(queuesDir, 0o755); err != nil {
		return err
	}
	// #nosec G301 -- scaffolding directories should be readable by developers.
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		return err
	}

	module := modulePath
	if module == "" {
		module = svc.Module
	}
	if module == "" {
		module = "example.com/project"
	}

	handlerPkgPath := fmt.Sprintf("%s/pkg/handlers/%s", module, svc.Name)
	queuePkgPath := fmt.Sprintf("%s/pkg/queues/%s", module, svc.Name)

	if err := writeFile(
		filepath.Join(handlersDir, "routes.go"),
		renderHandlers(svc, "handlers"),
	); err != nil {
		return err
	}
	if err := writeFile(
		filepath.Join(queuesDir, "queues.go"),
		renderQueueHandlers(svc, "queues"),
	); err != nil {
		return err
	}
	if err := writeFile(
		filepath.Join(cmdDir, "main.go"),
		renderMainPolylith(svc, handlerPkgPath, queuePkgPath),
	); err != nil {
		return err
	}

	return nil
}

func generateMonolith(outDir, modulePath string, services []ServiceSpec) error {
	sortServices(services)

	cmdDir := filepath.Join(outDir, "cmd")
	// #nosec G301 -- scaffolding directories should be readable by developers.
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		return err
	}

	module := modulePath
	if module == "" {
		module = "example.com/project"
	}

	servicePkgs := make([]string, 0, len(services))
	queuePkgs := make([]string, 0, len(services))

	for _, svc := range services {
		serviceDir := filepath.Join(outDir, "pkg", "services", svc.Name)
		queuesDir := filepath.Join(outDir, "pkg", "queues", svc.Name)
		pluginsDir := filepath.Join(outDir, "pkg", "plugins")

		// #nosec G301 -- scaffolding directories should be readable by developers.
		if err := os.MkdirAll(serviceDir, 0o755); err != nil {
			return err
		}
		// #nosec G301 -- scaffolding directories should be readable by developers.
		if err := os.MkdirAll(queuesDir, 0o755); err != nil {
			return err
		}
		// #nosec G301 -- scaffolding directories should be readable by developers.
		if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
			return err
		}

		pkgPath := fmt.Sprintf("%s/pkg/services/%s", module, svc.Name)
		queuePkg := fmt.Sprintf("%s/pkg/queues/%s", module, svc.Name)

		if err := writeFile(
			filepath.Join(serviceDir, "routes.go"),
			renderHandlers(svc, servicePackageName(svc.Name)),
		); err != nil {
			return err
		}
		if err := writeFile(
			filepath.Join(queuesDir, "queues.go"),
			renderQueueHandlers(svc, "queues"),
		); err != nil {
			return err
		}
		servicePkgs = append(servicePkgs, pkgPath)
		queuePkgs = append(queuePkgs, queuePkg)
	}

	if err := writeFile(
		filepath.Join(cmdDir, "main.go"),
		renderMainMonolith(services, servicePkgs, queuePkgs),
	); err != nil {
		return err
	}

	return nil
}

func renderHandlers(svc ServiceSpec, pkgName string) string {
	var b strings.Builder
	b.WriteString("package ")
	b.WriteString(pkgName)
	b.WriteString("\n\n")
	b.WriteString("import (\n\t\"net/http\"\n)\n\n")

	b.WriteString("func RegisterRoutes(mux *http.ServeMux) {\n")
	for _, r := range svc.HTTP {
		fmt.Fprintf(&b, "\tmux.HandleFunc(%q, %s)\n", r.Route, r.Handler)
	}
	b.WriteString("}\n\n")

	for _, r := range svc.HTTP {
		fmt.Fprintf(&b, "func %s(w http.ResponseWriter, r *http.Request) {\n", r.Handler)
		b.WriteString(methodCheck(r.Method))
		b.WriteString("\tw.Write([]byte(\"ok\"))\n")
		b.WriteString("}\n\n")
	}

	return b.String()
}

func renderQueueHandlers(svc ServiceSpec, pkgName string) string {
	var b strings.Builder
	b.WriteString("package ")
	b.WriteString(pkgName)
	b.WriteString("\n\n")
	b.WriteString("import (\n\t\"context\"\n\t\"log\"\n\t\"github.com/pitabwire/frame/queue\"\n)\n\n")

	seen := map[string]bool{}
	for _, q := range svc.Queues {
		if strings.TrimSpace(q.Subscriber) == "" {
			continue
		}
		h := strings.TrimSpace(q.Handler)
		if h == "" {
			h = "Handler"
		}
		if seen[h] {
			continue
		}
		seen[h] = true

		fmt.Fprintf(&b, "type %s struct{}\n\n", h)
		fmt.Fprintf(
			&b,
			"func (h %s) Handle(ctx context.Context, metadata map[string]string, message []byte) error {\n",
			h,
		)
		b.WriteString("\tlog.Printf(\"queue message: %s\", string(message))\n")
		b.WriteString("\treturn nil\n}\n\n")
		fmt.Fprintf(&b, "var _ queue.SubscribeWorker = %s{}\n\n", h)
	}

	if len(seen) == 0 {
		b.WriteString("type Handler struct{}\n\n")
		b.WriteString(
			"func (h Handler) Handle(ctx context.Context, metadata map[string]string, message []byte) error {\n",
		)
		b.WriteString("\tlog.Printf(\"queue message: %s\", string(message))\n")
		b.WriteString("\treturn nil\n}\n\n")
		b.WriteString("var _ queue.SubscribeWorker = Handler{}\n")
	}

	return b.String()
}

func renderMainPolylith(svc ServiceSpec, handlersPath, queuesPath string) string {
	plugins := resolvePlugins(svc)
	queueOpts := renderQueueOptions(svc)

	var b strings.Builder
	b.WriteString("package main\n\n")
	b.WriteString("import (\n\t\"log\"\n\t\"net/http\"\n\n\t\"github.com/pitabwire/frame\"\n")
	fmt.Fprintf(&b, "\t%q\n", handlersPath)
	if queueOpts != "" {
		fmt.Fprintf(&b, "\t%q\n", queuesPath)
	}
	b.WriteString(")\n\n")

	b.WriteString("func main() {\n")
	b.WriteString("\tmux := http.NewServeMux()\n")
	b.WriteString("\thandlers.RegisterRoutes(mux)\n\n")
	b.WriteString("\tctx, svc := frame.NewService(\n")
	fmt.Fprintf(&b, "\t\tframe.WithName(%q),\n", svc.Name)
	b.WriteString("\t\tframe.WithHTTPHandler(mux),\n")
	for _, opt := range plugins {
		fmt.Fprintf(&b, "\t\t%s,\n", opt)
	}
	for _, opt := range queueOptsLines(queueOpts) {
		fmt.Fprintf(&b, "\t\t%s,\n", opt)
	}
	b.WriteString("\t)\n\n")
	fmt.Fprintf(&b, "\tif err := svc.Run(ctx, %q); err != nil {\n", defaultPort(svc.Port))
	b.WriteString("\t\tlog.Fatal(err)\n\t}\n")
	b.WriteString("}\n")

	return b.String()
}

func renderMainMonolith(services []ServiceSpec, servicePkgs []string, queuePkgs []string) string {
	var b strings.Builder
	b.WriteString("package main\n\n")
	b.WriteString("import (\n\t\"log\"\n\t\"net/http\"\n\t\"sync\"\n\n\t\"github.com/pitabwire/frame\"\n")

	for i, pkg := range servicePkgs {
		fmt.Fprintf(&b, "\t%[1]s %q\n", serviceAlias(services[i].Name), pkg)
	}
	for i, pkg := range queuePkgs {
		if len(services[i].Queues) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\t%[1]s %q\n", queueAlias(services[i].Name), pkg)
	}
	b.WriteString(")\n\n")

	b.WriteString("func main() {\n")
	b.WriteString("\tvar wg sync.WaitGroup\n")
	fmt.Fprintf(&b, "\terrCh := make(chan error, %d)\n\n", len(services))

	for _, svc := range services {
		alias := serviceAlias(svc.Name)
		b.WriteString("\t{\n")
		b.WriteString("\t\tmu := http.NewServeMux()\n")
		fmt.Fprintf(&b, "\t\t%s.RegisterRoutes(mu)\n", alias)
		b.WriteString("\t\tctx, s := frame.NewService(\n")
		fmt.Fprintf(&b, "\t\t\tframe.WithName(%q),\n", svc.Name)
		b.WriteString("\t\t\tframe.WithHTTPHandler(mu),\n")
		for _, opt := range resolvePlugins(svc) {
			fmt.Fprintf(&b, "\t\t\t%s,\n", opt)
		}
		for _, opt := range queueOptsLines(renderQueueOptions(svc)) {
			opt = strings.ReplaceAll(opt, "queues.", queueAlias(svc.Name)+".")
			fmt.Fprintf(&b, "\t\t\t%s,\n", opt)
		}
		b.WriteString("\t\t)\n")
		b.WriteString("\t\tport := \"")
		b.WriteString(defaultPort(svc.Port))
		b.WriteString("\"\n")
		b.WriteString("\t\twg.Add(1)\n")
		b.WriteString("\t\tgo func() {\n\t\t\tdefer wg.Done()\n")
		b.WriteString("\t\t\tif err := s.Run(ctx, port); err != nil {\n\t\t\t\terrCh <- err\n\t\t\t}\n\t\t}()\n")
		b.WriteString("\t}\n\n")
	}

	b.WriteString("\tgo func() {\n\t\twg.Wait()\n\t\tclose(errCh)\n\t}()\n\n")
	b.WriteString("\tif err, ok := <-errCh; ok && err != nil {\n\t\tlog.Fatal(err)\n\t}\n")
	b.WriteString("}\n")

	return b.String()
}

func resolvePlugins(svc ServiceSpec) []string {
	var opts []string
	for _, p := range svc.Plugins {
		switch strings.ToLower(strings.TrimSpace(p)) {
		case "telemetry":
			opts = append(opts, "frame.WithTelemetry()")
		case "logger":
			opts = append(opts, "frame.WithLogger()")
		case "datastore":
			opts = append(opts, "frame.WithDatastore()")
		case "cache":
			opts = append(opts, "frame.WithCacheManager()")
		default:
			opts = append(opts, fmt.Sprintf("// TODO: add plugin %s", p))
		}
	}
	return opts
}

func renderQueueOptions(svc ServiceSpec) string {
	var opts []string
	for _, q := range svc.Queues {
		if q.Publisher != "" {
			opts = append(opts, fmt.Sprintf("frame.WithRegisterPublisher(\"%s\", \"%s\")", q.Publisher, q.URL))
		}
		if q.Subscriber != "" {
			h := strings.TrimSpace(q.Handler)
			if h == "" {
				h = "Handler"
			}
			opts = append(
				opts,
				fmt.Sprintf("frame.WithRegisterSubscriber(\"%s\", \"%s\", queues.%s{})", q.Subscriber, q.URL, h),
			)
		}
	}
	if len(opts) == 0 {
		return ""
	}
	return strings.Join(opts, "\n")
}

func queueOptsLines(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, "\n")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func servicePackageName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "handlers"
	}
	return "service_" + sanitizeIdent(name)
}

func serviceAlias(name string) string {
	return "svc_" + sanitizeIdent(name)
}

func sanitizeIdent(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, ".", "_")
	if s == "" {
		return "service"
	}
	return s
}

func methodCheck(method string) string {
	m := strings.TrimSpace(strings.ToUpper(method))
	if m == "" {
		return ""
	}
	switch m {
	case "GET":
		return "\tif r.Method != http.MethodGet {\n\t\tw.WriteHeader(http.StatusMethodNotAllowed)\n\t\treturn\n\t}\n"
	case "POST":
		return "\tif r.Method != http.MethodPost {\n\t\tw.WriteHeader(http.StatusMethodNotAllowed)\n\t\treturn\n\t}\n"
	case "PUT":
		return "\tif r.Method != http.MethodPut {\n\t\tw.WriteHeader(http.StatusMethodNotAllowed)\n\t\treturn\n\t}\n"
	case "DELETE":
		return "\tif r.Method != http.MethodDelete {\n\t\tw.WriteHeader(http.StatusMethodNotAllowed)\n\t\treturn\n\t}\n"
	case "PATCH":
		return "\tif r.Method != http.MethodPatch {\n\t\tw.WriteHeader(http.StatusMethodNotAllowed)\n\t\treturn\n\t}\n"
	default:
		return fmt.Sprintf(
			"\tif r.Method != %q {\n\t\tw.WriteHeader(http.StatusMethodNotAllowed)\n\t\treturn\n\t}\n",
			m,
		)
	}
}

func moduleFromGoMod(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	return "", errors.New("module path not found")
}

func writeFile(path, content string) error {
	// #nosec G306 -- generated files should be readable by the developer.
	return os.WriteFile(path, []byte(content), 0o644)
}

func sortServices(services []ServiceSpec) {
	sort.SliceStable(services, func(i, j int) bool {
		return services[i].Name < services[j].Name
	})
}

func queueAlias(name string) string {
	return "queues_" + sanitizeIdent(name)
}
