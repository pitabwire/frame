package scopes

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/pitabwire/frame/security"
)

// TenancyPartition applies tenant and partition filtering to database queries.
//
// Behavior:
//   - If no claims in context: returns unscoped db (for cross-tenant services)
//   - If skip tenancy enabled: returns unscoped db (for backend services processing across tenants)
//   - For internal systems: claims are auto-enriched with secondary claims tenancy data
//   - Empty tenant_id/partition_id will match only records with empty values (no cross-tenant leakage)
func TenancyPartition(ctx context.Context) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		authClaim := security.ClaimsFromContext(ctx)
		if authClaim == nil {
			// No claims - allow unscoped access (cross-tenant services like profile service)
			return db
		}

		skipTenancyChecksOnClaims := security.IsTenancyChecksOnClaimSkipped(ctx)
		if skipTenancyChecksOnClaims {
			return db
		}

		// Safely retrieve the table name (fallback to empty string if nil)
		table := db.Statement.Table
		if table != "" {
			table += "."
		}

		// Apply tenancy filter with actual values from claims
		// Empty string values will match only empty records in DB (no cross-tenant leakage)
		return db.Where(
			fmt.Sprintf("%stenant_id = ? AND %spartition_id = ?", table, table),
			authClaim.GetTenantID(),
			authClaim.GetPartitionID(),
		)
	}
}
