package setup

import (
	"os"
	"strings"
)

// Command is the conventional argv[0] that marks a process as a setup job
// (e.g. Cloud Run Job args: ["setup", "migrate", "permissions"]).
const Command = "setup"

// Selection describes whether this process should run setup (and which steps).
// It is pure data — no I/O — so tests and CLIs can build it without a Registry.
//
//	sel := setup.Select(os.Args[1:], doSetupFlag, setupTasksCSV)
//	if sel.Active {
//	    return registry.Run(ctx, sel.Names...)
//	}
type Selection struct {
	// Active is true when this process is a setup job (not a long-running server).
	Active bool

	// Names is the ordered step list to run. Empty with Active true means
	// “run every registered step” (Registry.Run treats empty as all).
	Names []string
}

// Select builds a Selection from process argv and optional flags.
//
// Precedence for names when Active:
//  1. argv after Command: `setup migrate permissions` → [migrate, permissions]
//  2. else tasksCSV (comma-separated), if non-empty
//  3. else empty (run all registered)
//
// Active is true when:
//   - argv[0] == Command ("setup"), or
//   - doSetup is true, or
//   - tasksCSV is non-empty
//
// Legacy bare `migrate` as argv[0] does NOT activate setup mode — callers
// keep separate DoDatabaseMigrate paths for backwards compatibility.
func Select(args []string, doSetup bool, tasksCSV string) Selection {
	args = normalizeArgs(args)
	csvNames := splitCSV(tasksCSV)

	if len(args) > 0 && args[0] == Command {
		sel := Selection{Active: true}
		if len(args) > 1 {
			sel.Names = filterEmpty(args[1:])
		}
		return sel
	}

	if doSetup || len(csvNames) > 0 {
		return Selection{Active: true, Names: csvNames}
	}

	return Selection{}
}

// SelectFromOS is Select(os.Args[1:], doSetup, tasksCSV).
func SelectFromOS(doSetup bool, tasksCSV string) Selection {
	return Select(os.Args[1:], doSetup, tasksCSV)
}

func normalizeArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, 0, len(args))
	for _, a := range args {
		a = strings.TrimSpace(a)
		if a != "" {
			out = append(out, a)
		}
	}
	return out
}

func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return filterEmpty(strings.Split(s, ","))
}
