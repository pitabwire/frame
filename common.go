package frame

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/pitabwire/util"
	"github.com/rs/xid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

type BaseModelI interface {
	GetID() string
	GetVersion() uint
}

// BaseModel base table struct to be extended by other models.
type BaseModel struct {
	ID          string `gorm:"type:varchar(50);primary_key"`
	CreatedAt   time.Time
	ModifiedAt  time.Time
	Version     uint           `gorm:"DEFAULT 0"`
	TenantID    string         `gorm:"type:varchar(50);index:,composite:base_tenancy"`
	PartitionID string         `gorm:"type:varchar(50);index:,composite:base_tenancy"`
	AccessID    string         `gorm:"type:varchar(50);index:,composite:base_tenancy"`
	DeletedAt   gorm.DeletedAt `sql:"index"`
}

func (model *BaseModel) GetID() string {
	return model.ID
}

// GenID creates a new id for model if its not existent.
func (model *BaseModel) GenID(ctx context.Context) {
	if model.ID != "" {
		return
	}

	model.ID = util.IDString()

	authClaim := ClaimsFromContext(ctx)
	if authClaim == nil {
		return
	}

	if model.AccessID == "" && authClaim.GetAccessID() != "" {
		model.AccessID = authClaim.GetAccessID()
	}

	if model.PartitionID == "" && authClaim.GetPartitionID() != "" {
		model.PartitionID = authClaim.GetPartitionID()
	}

	if model.TenantID == "" && authClaim.GetTenantID() != "" {
		model.TenantID = authClaim.GetTenantID()
	}
}

// ValidXID Validates that the supplied string is an xid.
func (model *BaseModel) ValidXID(id string) bool {
	_, err := xid.FromString(id)
	return err == nil
}

func (model *BaseModel) GetVersion() uint {
	return model.Version
}

// BeforeSave Ensures we update a migrations time stamps.
func (model *BaseModel) BeforeSave(db *gorm.DB) error {
	return model.BeforeCreate(db)
}

func (model *BaseModel) BeforeCreate(db *gorm.DB) error {
	if model.Version <= 0 {
		model.CreatedAt = time.Now()
		model.ModifiedAt = time.Now()
		model.Version = 1
	}

	model.GenID(db.Statement.Context)
	return nil
}

// BeforeUpdate Updates time stamp every time we update status of a migration.
func (model *BaseModel) BeforeUpdate(_ *gorm.DB) error {
	model.ModifiedAt = time.Now()
	model.Version++
	return nil
}

func (model *BaseModel) CopyPartitionInfo(parent *BaseModel) {
	if parent == nil {
		return
	}

	if model.TenantID == "" || model.PartitionID == "" {
		model.TenantID = parent.TenantID
		model.PartitionID = parent.PartitionID
		model.AccessID = parent.AccessID
	}
}

// Migration Our simple table holding all the migration data.
type Migration struct {
	BaseModel

	Name        string `gorm:"type:text;uniqueIndex:idx_migrations_name"`
	Patch       string `gorm:"type:text"`
	RevertPatch string `gorm:"type:text"`
	AppliedAt   sql.NullTime
}

// ErrIsNotFound checks if an error represents a "not found" condition.
// It handles multiple error types:
// - Database errors: gorm.ErrRecordNotFound, sql.ErrNoRows (via ErrorIsNoRows)
// - gRPC errors: codes.NotFound
// - Generic errors: error messages containing "not found" (case-insensitive).
func ErrIsNotFound(err error) bool {
	if err == nil {
		return false
	}

	// Check database errors using existing ErrorIsNoRows function
	if ErrorIsNoRows(err) {
		return true
	}

	// Check gRPC status errors
	if st, ok := status.FromError(err); ok {
		return st.Code() == codes.NotFound
	}

	// Check error message for "not found" string (case-insensitive)
	errMsg := strings.ToLower(err.Error())
	if strings.Contains(errMsg, "not found") ||
		strings.Contains(errMsg, "notfound") ||
		strings.Contains(errMsg, "404") {
		return true
	}

	return false
}
