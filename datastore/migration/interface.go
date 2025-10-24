package migration

import (
	"context"

	"gorm.io/gorm"
)

type Migrator interface {
	DB(ctx context.Context) *gorm.DB
	ScanMigrationFiles(ctx context.Context, migrationsDirPath string) error
	SaveMigrationString(
		ctx context.Context,
		filename string,
		migrationPatch string,
		revertPatch string,
	) error
	ApplyNewMigrations(ctx context.Context) error
}
