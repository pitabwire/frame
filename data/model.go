package data

import (
	"context"
	"time"

	"github.com/pitabwire/util"
	"github.com/rs/xid"
	"gorm.io/gorm"

	"github.com/pitabwire/frame/security"
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
	if model.ID == "" {
		model.ID = util.IDString()
	}

	authClaim := security.ClaimsFromContext(ctx)
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
