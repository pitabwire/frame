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

type migrator struct {
	service *Service
}

func (m *migrator) DB(ctx context.Context) *gorm.DB {
	return m.service.DB(ctx, false)
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
		filename = strings.Replace(filename, ".sql", "", 1)

		migrationPatch, err := os.ReadFile(file)

		if err != nil {
			log.Printf("scanForNewMigrations -- Problem reading migration file content : %+v", err)
			continue
		}

		err = m.saveNewMigrations(ctx, filename, string(migrationPatch))
		if err != nil {
			log.Printf("scanForNewMigrations -- new migration :%s could not be processed because: %+v", file, err)
			return err
		}

	}
	return nil
}

func (m *migrator) saveNewMigrations(ctx context.Context, filename string, migrationPatch string) error {

	migration := Migration{}

	err := m.DB(ctx).Model(&migration).First(&migration, "name = ?", filename).Error
	if err != nil {

		if !DBErrorIsRecordNotFound(err) {
			return err
		}

		migration := Migration{
			Name:  filename,
			Patch: migrationPatch,
		}
		err = m.DB(ctx).Create(&migration).Error
		if err != nil {
			return err
		}

		return nil
	}

	if !migration.AppliedAt.Valid && migration.Patch != migrationPatch {
		err := m.DB(ctx).Model(&migration).Update("patch", migrationPatch).Error
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

		if err := m.DB(ctx).Exec(migration.Patch).Error; err != nil {
			return err
		}

		err := m.DB(ctx).Model(migration).Update("applied_at", time.Now()).Error
		if err != nil {
			return err
		}

		log.Printf("applyNewMigrations -- Successfully applied the file : %v", fmt.Sprintf("%s.sql", migration.Name))
	}

	return nil
}

// MigrateDatastore finds missing migrations and records them in the database
func (s *Service) MigrateDatastore(ctx context.Context, migrationsDirPath string, migrations ...interface{}) error {
	if migrationsDirPath == "" {
		migrationsDirPath = "./migrations/0001"
	}

	migrations = append([]interface{}{&Migration{}}, migrations...)
	// Migrate the schema
	err := s.DB(ctx, false).AutoMigrate(migrations...)
	if err != nil {
		s.L().WithError(err).Error("MigrateDatastore -- couldn't automigrate")
		return err
	}

	migrationExecutor := migrator{service: s}

	if err := migrationExecutor.scanForNewMigrations(ctx, migrationsDirPath); err != nil {
		log.Printf("MigrateDatastore -- Error scanning for new migrations : %+v ", err)
		return err
	}

	if err := migrationExecutor.applyNewMigrations(ctx); err != nil {
		log.Printf("MigrateDatastore -- There was an error applying migrations : %+v ", err)
		return err
	}
	return nil
}
