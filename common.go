package frame

import (
	"github.com/rs/xid"
	"gorm.io/gorm"
	"time"
)

// Migration Our simple table holding all the migration data
type BaseModel struct {
	ID         string `gorm:"type:varchar(50);primary_key"`
	CreatedAt  time.Time
	ModifiedAt time.Time
	Version    uint32         `gorm:"DEFAULT 0"`
	DeletedAt  gorm.DeletedAt `sql:"index"`
}

// BeforeCreate Ensures we update a migrations time stamps
func (model *BaseModel) BeforeCreate(db *gorm.DB) error {
	model.ID = xid.New().String()
	model.CreatedAt = time.Now()
	model.ModifiedAt = time.Now()
	model.Version = 1
	return nil
}

// BeforeUpdate Updates time stamp every time we update status of a migration
func (model *BaseModel) BeforeUpdate(db *gorm.DB) error {
	model.ModifiedAt = time.Now()
	model.Version = 1
	return nil
}

// Migration Our simple table holding all the migration data
type Migration struct {
	BaseModel

	Name      string `gorm:"type:varchar(50);unique_index"`
	Patch     string `gorm:"type:text"`
	AppliedAt *time.Time
}

