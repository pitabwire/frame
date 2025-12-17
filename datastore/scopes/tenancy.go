package scopes

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/pitabwire/frame/security"
)

func TenancyPartition(ctx context.Context) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		authClaim := security.ClaimsFromContext(ctx)
		if authClaim == nil {
			return db
		}

		skipTenancyChecksOnClaims := security.IsTenancyChecksOnClaimSkipped(ctx)
		if skipTenancyChecksOnClaims {
			authClaim = security.SecondaryClaimsFromContext(ctx)
			if authClaim == nil {
				return db
			}
		}

		// Safely retrieve the table name (fallback to empty string if nil)
		table := db.Statement.Table
		if table != "" {
			table += "."
		}

		return db.Where(
			fmt.Sprintf("%stenant_id = ? AND %spartition_id = ?", table, table),
			authClaim.GetTenantID(),
			authClaim.GetPartitionID(),
		)
	}
}
