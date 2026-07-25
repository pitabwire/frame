package postgres

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// ArmingCheck verifies the database role used for application queries
// does not bypass RLS (not superuser, not BYPASSRLS). Intended for
// readiness (/readyz), never liveness.
//
// Construct with a function that returns a *gorm.DB for the app pool
// (not the migration superuser pool).
type ArmingCheck struct {
	// DB returns a session for the app role. Called on each CheckHealth.
	DB func(ctx context.Context) *gorm.DB
	// NameOverride optional checker name (default tenancy_provider_armed).
	NameOverride string
}

// Name implements frame.NamedChecker when used with AddHealthCheck.
func (c *ArmingCheck) Name() string {
	if c.NameOverride != "" {
		return c.NameOverride
	}
	return "tenancy_provider_armed"
}

// CheckHealth reports an error if the current DB role can bypass RLS.
func (c *ArmingCheck) CheckHealth() error {
	if c == nil || c.DB == nil {
		return errors.New("tenancy arming: no database accessor")
	}
	ctx := context.Background()
	db := c.DB(ctx)
	if db == nil {
		return errors.New("tenancy arming: no database session")
	}
	var super, bypass bool
	row := db.WithContext(ctx).Raw(`
		SELECT COALESCE(rolsuper, false), COALESCE(rolbypassrls, false)
		FROM pg_roles WHERE rolname = current_user`).Row()
	if err := row.Scan(&super, &bypass); err != nil {
		return fmt.Errorf("tenancy arming: query role flags: %w", err)
	}
	if super {
		return errors.New("tenancy arming: current_user is superuser (bypasses RLS)")
	}
	if bypass {
		return errors.New("tenancy arming: current_user has BYPASSRLS")
	}
	return nil
}
