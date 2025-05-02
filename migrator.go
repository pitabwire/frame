package frame

import (
	"context"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type MigrationPatch struct {
	// Name is a simple description/name of this migration.
	Name string
	// Patch is the SQL to execute for an upgrade.
	Patch string
	// RevertPatch is the SQL to execute for a downgrade.
	RevertPatch string
}

type migrator struct {
	pool *Pool
}

func (m *migrator) DB(ctx context.Context) *gorm.DB {
	return m.pool.DB(ctx, false)
}

func (m *migrator) scanForNewMigrations(ctx context.Context, migrationsDirPath string) error {

	// Get a list of migration files
	files, err := filepath.Glob(migrationsDirPath + "/*.sql")
	if err != nil {
		return err
	}

	sort.Strings(files)

	for _, file := range files {
		filename := filepath.Base(file)

		// Skip _down.sql files
		if strings.HasSuffix(filename, "_down.sql") {
			continue
		}

		migrationPatch, err0 := os.ReadFile(file)
		if err0 != nil {
			log.Printf("scanForNewMigrations -- Problem reading migration file content : %s", err0)
			continue
		}

		revertPatch := ""
		if strings.HasSuffix(filename, "_up.sql") {
			// Try to find matching _down.sql file
			downFilename := strings.TrimSuffix(filename, "_up.sql") + "_down.sql"
			downFilePath := filepath.Join(migrationsDirPath, downFilename)
			if _, err := os.Stat(downFilePath); err == nil {
				downPatch, err := os.ReadFile(downFilePath)
				if err == nil {
					revertPatch = string(downPatch)
				}
			}
		}
		// For files not ending with _up.sql or _down.sql, revertPatch remains ""

		err0 = m.saveMigrationString(ctx, filename, string(migrationPatch), revertPatch)
		if err0 != nil {
			log.Printf("scanForNewMigrations -- new migration :%s could not be processed because: %s", file, err0)
			return err0
		}
	}
	return nil
}

func (m *migrator) saveMigrationString(ctx context.Context, filename string, migrationPatch string, revertPatch string) error {

	//If a file name exists, save with the name it has
	_, err := os.Stat(filename)
	if errors.Is(err, os.ErrNotExist) {
		filename = fmt.Sprintf("string:%s", filename)
	}

	migration := Migration{}

	err = m.DB(ctx).Model(&migration).First(&migration, "name = ?", filename).Error
	if err != nil {

		if !DBErrorIsRecordNotFound(err) {
			return err
		}

		migration = Migration{
			Name:        filename,
			Patch:       migrationPatch,
			RevertPatch: revertPatch,
		}
		err = m.DB(ctx).Create(&migration).Error
		if err != nil {
			return err
		}

		return nil
	}

	if !migration.AppliedAt.Valid && migration.Patch != migrationPatch {
		err = m.DB(ctx).Model(&migration).Update("patch", migrationPatch).Error
		if err != nil {
			return err
		}
	}
	if !migration.AppliedAt.Valid && revertPatch != "" && migration.RevertPatch != revertPatch {
		err = m.DB(ctx).Model(&migration).Update("revert_patch", revertPatch).Error
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *migrator) applyNewMigrations(ctx context.Context) error {

	var unAppliedMigrations []*Migration
	err := m.DB(ctx).Where("applied_at IS NULL").Find(&unAppliedMigrations).Error
	if err != nil {

		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("applyNewMigrations -- No migrations found to be applied ")
			return nil
		}
		return err
	}

	for _, migration := range unAppliedMigrations {

		err = m.DB(ctx).Exec(migration.Patch).Error
		if err != nil {
			return err
		}

		err = m.DB(ctx).Model(migration).Update("applied_at", time.Now()).Error
		if err != nil {
			return err
		}

		log.Printf("applyNewMigrations -- Successfully applied the file : %v", fmt.Sprintf("%s.sql", migration.Name))
	}

	return nil
}

func (s *Service) SaveMigration(ctx context.Context, migrationPatches ...MigrationPatch) error {
	pool := s.DBPool()
	return s.SaveMigrationWithPool(ctx, pool, migrationPatches...)
}

func (s *Service) SaveMigrationWithPool(ctx context.Context, pool *Pool, migrationPatches ...MigrationPatch) error {
	migrationExecutor := migrator{pool: pool}
	for _, migrationPatch := range migrationPatches {
		err := migrationExecutor.saveMigrationString(ctx, migrationPatch.Name, migrationPatch.Patch, migrationPatch.RevertPatch)
		if err != nil {
			return err
		}
	}
	return nil
}

// MigrateDatastore finds missing migrations and records them in the database
func (s *Service) MigrateDatastore(ctx context.Context, migrationsDirPath string, migrations ...any) error {
	pool := s.DBPool()
	return s.MigratePool(ctx, pool, migrationsDirPath, migrations...)
}

// MigratePool finds missing migrations and records them in the database
func (s *Service) MigratePool(ctx context.Context, pool *Pool, migrationsDirPath string, migrations ...any) error {
	if migrationsDirPath == "" {
		migrationsDirPath = "./migrations/0001"
	}

	migrations = append([]any{&Migration{}}, migrations...)
	// Migrate the schema
	err := s.DB(ctx, false).AutoMigrate(migrations...)
	if err != nil {
		s.L(ctx).WithError(err).Error("MigrateDatastore -- couldn't automigrate")
		return err
	}

	migrationExecutor := migrator{pool: pool}

	err = migrationExecutor.scanForNewMigrations(ctx, migrationsDirPath)
	if err != nil {
		log.Printf("MigrateDatastore -- Error scanning for new migrations : %s ", err)
		return err
	}

	err = migrationExecutor.applyNewMigrations(ctx)
	if err != nil {
		log.Printf("MigrateDatastore -- There was an error applying migrations : %s ", err)
		return err
	}
	return nil
}
