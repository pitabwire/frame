package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/pitabwire/frame/blueprint"
)

const (
	minArgsCommand     = 2
	minArgsRouteMethod = 2
	minArgsQueuePub    = 2
	minArgsQueueSub    = 3
	argRoute           = 0
	argMethod          = 1
	argHandler         = 2
	schemaVersion      = "0.1"
	defaultSubcommand  = 1
)

func main() {
	if len(os.Args) < minArgsCommand {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "build":
		exitOnErr(cmdBuild(os.Args[2:]))
	case "validate":
		exitOnErr(cmdValidate(os.Args[2:]))
	case "explain":
		exitOnErr(cmdExplain(os.Args[2:]))
	case "g":
		exitOnErr(cmdGenerate(os.Args[2:]))
	case "help", "-h", "--help":
		usage()
	default:
		// #nosec G705 -- CLI output is not rendered in an HTML context.
		fmt.Fprintf(os.Stderr, "unknown command: %q\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stdout, "frame <command> [args]")
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "Commands:")
	fmt.Fprintln(os.Stdout, "  build <blueprint> [--out DIR]")
	fmt.Fprintln(os.Stdout, "  validate <blueprint>")
	fmt.Fprintln(os.Stdout, "  explain <blueprint>")
	fmt.Fprintln(
		os.Stdout,
		"  g service <name> [--blueprint FILE] [--module MOD] [--mode monolith|polylith] [--port :8080]",
	)
	fmt.Fprintln(os.Stdout, "  g http <route> <method> [--handler NAME] [--service NAME] [--blueprint FILE]")
	fmt.Fprintln(os.Stdout, "  g queue publisher <ref> <url> [--service NAME] [--blueprint FILE]")
	fmt.Fprintln(os.Stdout, "  g queue subscriber <ref> <url> <handler> [--service NAME] [--blueprint FILE]")
}

func cmdBuild(args []string) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	outDir := fs.String("out", "_generated", "output directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("blueprint file is required")
	}
	path := fs.Arg(0)
	bp, err := blueprint.LoadFile(path)
	if err != nil {
		return err
	}
	if genErr := bp.Generate(blueprint.GenerateOptions{OutDir: *outDir}); genErr != nil {
		return genErr
	}
	return nil
}

func cmdValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("blueprint file is required")
	}
	path := fs.Arg(0)
	bp, err := blueprint.LoadFile(path)
	if err != nil {
		return err
	}
	return bp.Validate()
}

func cmdExplain(args []string) error {
	fs := flag.NewFlagSet("explain", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("blueprint file is required")
	}
	path := fs.Arg(0)
	bp, err := blueprint.LoadFile(path)
	if err != nil {
		return err
	}
	out, err := bp.Explain()
	if err != nil {
		return err
	}
	_, _ = fmt.Fprint(os.Stdout, out)
	return nil
}

func cmdGenerate(args []string) error {
	if len(args) < defaultSubcommand {
		return errors.New("subcommand required")
	}
	cmd := args[0]
	args = args[1:]

	switch cmd {
	case "service":
		return cmdGenerateService(args)
	case "http":
		return cmdGenerateHTTP(args)
	case "queue":
		return cmdGenerateQueue(args)
	default:
		return fmt.Errorf("unknown generator: %s", cmd)
	}
}

func cmdGenerateService(args []string) error {
	fs := flag.NewFlagSet("service", flag.ContinueOnError)
	bpFile := fs.String("blueprint", "blueprint.yaml", "blueprint file")
	module := fs.String("module", "", "module path")
	mode := fs.String("mode", "polylith", "runtime mode")
	port := fs.String("port", ":8080", "http port")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < defaultSubcommand {
		return errors.New("service name is required")
	}
	name := fs.Arg(0)

	bp, err := loadOrCreateBlueprint(*bpFile)
	if err != nil {
		return err
	}

	if bp.Service == nil {
		bp.Service = &blueprint.ServiceSpec{}
	}
	bp.SchemaVersion = schemaVersion
	bp.RuntimeMode = strings.ToLower(*mode)
	if strings.ToLower(*mode) == "monolith" {
		if bp.Services == nil {
			bp.Services = []blueprint.ServiceSpec{}
		}
		bp.Services = append(bp.Services, blueprint.ServiceSpec{Name: name, Port: *port, Module: *module})
		if bp.Service != nil {
			bp.Service = nil
		}
	} else {
		bp.Service.Name = name
		bp.Service.Port = *port
		if *module != "" {
			bp.Service.Module = *module
		}
	}

	return blueprint.WriteFile(*bpFile, bp)
}

func cmdGenerateHTTP(args []string) error {
	fs := flag.NewFlagSet("http", flag.ContinueOnError)
	bpFile := fs.String("blueprint", "blueprint.yaml", "blueprint file")
	service := fs.String("service", "", "target service (for monolith)")
	handler := fs.String("handler", "", "handler name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < minArgsRouteMethod {
		return errors.New("route and method are required")
	}
	route := fs.Arg(argRoute)
	method := fs.Arg(argMethod)
	if *handler == "" {
		*handler = defaultHandlerName(route, method)
	}

	bp, err := loadOrCreateBlueprint(*bpFile)
	if err != nil {
		return err
	}
	bp.SchemaVersion = schemaVersion
	serviceSpec := selectService(bp, *service)
	serviceSpec.HTTP = append(serviceSpec.HTTP, blueprint.HTTPRoute{Route: route, Method: method, Handler: *handler})

	return blueprint.WriteFile(*bpFile, bp)
}

func cmdGenerateQueue(args []string) error {
	if len(args) < defaultSubcommand {
		return errors.New("queue subcommand required")
	}
	cmd := args[0]
	args = args[1:]

	switch cmd {
	case "publisher":
		return cmdGenerateQueuePublisher(args)
	case "subscriber":
		return cmdGenerateQueueSubscriber(args)
	default:
		return fmt.Errorf("unknown queue generator: %s", cmd)
	}
}

func cmdGenerateQueuePublisher(args []string) error {
	fs := flag.NewFlagSet("queue-publisher", flag.ContinueOnError)
	bpFile := fs.String("blueprint", "blueprint.yaml", "blueprint file")
	service := fs.String("service", "", "target service (for monolith)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < minArgsQueuePub {
		return errors.New("publisher ref and url are required")
	}
	ref := fs.Arg(argRoute)
	url := fs.Arg(argMethod)

	bp, err := loadOrCreateBlueprint(*bpFile)
	if err != nil {
		return err
	}
	bp.SchemaVersion = schemaVersion
	serviceSpec := selectService(bp, *service)
	serviceSpec.Queues = append(serviceSpec.Queues, blueprint.QueueSpec{Publisher: ref, URL: url})

	return blueprint.WriteFile(*bpFile, bp)
}

func cmdGenerateQueueSubscriber(args []string) error {
	fs := flag.NewFlagSet("queue-subscriber", flag.ContinueOnError)
	bpFile := fs.String("blueprint", "blueprint.yaml", "blueprint file")
	service := fs.String("service", "", "target service (for monolith)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < minArgsQueueSub {
		return errors.New("subscriber ref, url, and handler are required")
	}
	ref := fs.Arg(argRoute)
	url := fs.Arg(argMethod)
	handler := fs.Arg(argHandler)

	bp, err := loadOrCreateBlueprint(*bpFile)
	if err != nil {
		return err
	}
	bp.SchemaVersion = schemaVersion
	serviceSpec := selectService(bp, *service)
	serviceSpec.Queues = append(serviceSpec.Queues, blueprint.QueueSpec{Subscriber: ref, URL: url, Handler: handler})

	return blueprint.WriteFile(*bpFile, bp)
}

func loadOrCreateBlueprint(path string) (*blueprint.Blueprint, error) {
	if _, err := os.Stat(path); err == nil {
		return blueprint.LoadFile(path)
	}

	bp := &blueprint.Blueprint{SchemaVersion: schemaVersion}
	return bp, nil
}

func selectService(bp *blueprint.Blueprint, name string) *blueprint.ServiceSpec {
	if bp == nil {
		return nil
	}
	if len(bp.Services) > 0 {
		for i := range bp.Services {
			if name == "" || bp.Services[i].Name == name {
				return &bp.Services[i]
			}
		}
		if name != "" {
			bp.Services = append(bp.Services, blueprint.ServiceSpec{Name: name})
			return &bp.Services[len(bp.Services)-1]
		}
		return &bp.Services[0]
	}
	if bp.Service == nil {
		bp.Service = &blueprint.ServiceSpec{Name: name}
	}
	if name != "" {
		bp.Service.Name = name
	}
	return bp.Service
}

func defaultHandlerName(route, method string) string {
	clean := strings.Trim(route, "/")
	clean = strings.ReplaceAll(clean, "/", "_")
	if clean == "" {
		clean = "root"
	}
	name := titleWord(strings.ToLower(method)) + titleWord(clean)
	name = strings.ReplaceAll(name, "_", "")
	name = strings.ReplaceAll(name, "-", "")
	return name
}

func titleWord(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func exitOnErr(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
