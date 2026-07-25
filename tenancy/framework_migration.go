package tenancy

import "context"

// frameworkMigrationKey is unexported so only this module can mint elevation
// for schema migrations. Public SystemPrincipal.Reason is never an auth signal.
type frameworkMigrationKey struct{}

// WithFrameworkMigration elevates ctx for Frame-owned migrate paths.
//
// For Frame internals only (datastore/pool.Migrate, migration package).
// Application code cannot set the unexported marker from outside this module
// without the type; forging SystemPrincipal{Reason:"migration", AllowGlobal:true}
// alone does not authorize match-all.
//
// Godoc: not part of the application tenancy API.
func WithFrameworkMigration(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if IsFrameworkMigration(ctx) {
		// Ensure principal is present even if marker already set.
		if _, ok := SystemPrincipalFromContext(ctx); ok {
			return ctx
		}
	}
	ctx = context.WithValue(ctx, frameworkMigrationKey{}, true)
	ctx = WithSystemPrincipal(ctx, SystemPrincipal{
		ServiceName: "frame",
		Reason:      "migration", // logs/metrics only
		AllowGlobal: true,
	})
	return ctx
}

// IsFrameworkMigration reports whether ctx carries the unexported migration
// elevation marker.
func IsFrameworkMigration(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(frameworkMigrationKey{}).(bool)
	return v
}

// EnsureFrameworkMigration returns jobCtx with framework migration elevation
// if parent has it (or jobCtx already does). Used by migrator DB factories so
// nested acquires cannot drop elevation.
func EnsureFrameworkMigration(jobCtx, parent context.Context) context.Context {
	if IsFrameworkMigration(jobCtx) {
		return jobCtx
	}
	if IsFrameworkMigration(parent) {
		return WithFrameworkMigration(jobCtx)
	}
	return jobCtx
}
