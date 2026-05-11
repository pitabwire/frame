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
//   - tenant_id is a single value; partition_id may be a list (claim.GetPartitionIDs())
//     so principals that legitimately span multiple partitions (a SACCO operator with
//     access to several branches, an analyst aggregating across groups) see rows from
//     every partition they belong to. Single-partition callers behave as before — the
//     list has one element and the SQL becomes a degenerate IN (?).
//   - Empty tenant_id will match only records with empty tenant_id (no cross-tenant leakage)
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

		partitions := authClaim.GetPartitionIDs()
		if len(partitions) == 0 {
			// Match the previous behaviour: empty partition matches only
			// rows with empty partition_id, never leaks across partitions.
			partitions = []string{""}
		}

		return db.Where(
			fmt.Sprintf("%stenant_id = ? AND %spartition_id IN ?", table, table),
			authClaim.GetTenantID(),
			partitions,
		)
	}
}
