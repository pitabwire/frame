package scopes

import (
	"context"

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
			return db
		}

		return db.Where("tenant_id = ? AND partition_id = ?", authClaim.GetTenantID(), authClaim.GetPartitionID())
	}
}
