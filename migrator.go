package frame

import (
	"context"
	"errors"
	"fmt"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
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
	pool   *Pool
	logger *logrus.Entry
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
			m.logger.WithError(err0).WithField("file", filename).Error("scanForNewMigrations -- Problem reading migration file content")
			continue
		}

		revertPatch := ""
		if strings.HasSuffix(filename, "_up.sql") {
			// Try to find matching _down.sql file
			downFilename := strings.TrimSuffix(filename, "_up.sql") + "_down.sql"
			downFilePath := filepath.Join(migrationsDirPath, downFilename)
			if _, err0 = os.Stat(downFilePath); err0 == nil {
				downPatch, err1 := os.ReadFile(downFilePath)
				if err1 == nil {
					revertPatch = string(downPatch)
				}
			}
		}
		// For files not ending with _up.sql or _down.sql, revertPatch remains ""

		err0 = m.saveMigrationString(ctx, filename, string(migrationPatch), revertPatch)
		if err0 != nil {
			m.logger.WithError(err0).WithField("file", filename).Error("scanForNewMigrations -- new migration could not be saved")
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

		if !ErrorIsNoRows(err) {
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
			m.logger.Info("applyNewMigrations -- No migrations found to be applied ")
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

		m.logger.WithField("migration", migration.Name).Debug("applyNewMigrations -- Successfully applied migration")
	}

	return nil
}

func (s *Service) newMigrator(ctx context.Context, poolOpts ...*Pool) *migrator {
	var pool *Pool
	if len(poolOpts) > 0 {
		pool = poolOpts[0]
	} else {
		pool = s.DBPool()
	}

	return &migrator{
		pool:   pool,
		logger: s.L(ctx),
	}
}

func (s *Service) SaveMigration(ctx context.Context, migrationPatches ...*MigrationPatch) error {
	pool := s.DBPool()
	return s.SaveMigrationWithPool(ctx, pool, migrationPatches...)
}

func (s *Service) SaveMigrationWithPool(ctx context.Context, pool *Pool, migrationPatches ...*MigrationPatch) error {
	migrationExecutor := s.newMigrator(ctx, pool)
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

	migrtor := pool.DB(ctx, false).Migrator()
	// Ensure the migration schema exists
	if !migrtor.HasTable(&Migration{}) {

		err := migrtor.CreateTable(&Migration{})
		if err != nil {
			s.L(ctx).WithError(err).Error("MigrateDatastore -- couldn't create migration table")
			return err
		}
	}

	if len(migrations) > 0 {
		// Migrate the schema
		err := migrtor.AutoMigrate(migrations...)
		if err != nil {
			s.L(ctx).WithError(err).Error("MigrateDatastore -- couldn't auto migrate")
			return err
		}
	}

	migrationExecutor := s.newMigrator(ctx, pool)

	err := migrationExecutor.scanForNewMigrations(ctx, migrationsDirPath)
	if err != nil {
		s.L(ctx).WithError(err).Error("MigrateDatastore -- Error scanning for new migrations")
		return err
	}

	err = migrationExecutor.applyNewMigrations(ctx)
	if err != nil {
		s.L(ctx).WithError(err).Error("MigrateDatastore -- Error applying migrations ")
		return err
	}
	return nil
}
