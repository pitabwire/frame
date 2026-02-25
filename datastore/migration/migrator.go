package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pitabwire/util"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/pitabwire/frame/data"
)

// Migration Our simple table holding all the migration data.
type Migration struct {
	data.BaseModel

	Name        string `gorm:"type:text;uniqueIndex:idx_migrations_name"`
	Patch       string `gorm:"type:text"`
	RevertPatch string `gorm:"type:text"`
	AppliedAt   sql.NullTime
}

type Patch struct {
	// Name is a simple description/name of this migration.
	Name string
	// Patch is the SQL to execute for an upgrade.
	Patch string
	// RevertPatch is the SQL to execute for a downgrade.
	RevertPatch string
}

type datastoreMigrator struct {
	dbGetter func(ctx context.Context) *gorm.DB
	logger   *util.LogEntry
}

func NewMigrator(ctx context.Context, dbGetter func(ctx context.Context) *gorm.DB) Migrator {
	return &datastoreMigrator{
		dbGetter: dbGetter,
		logger:   util.Log(ctx),
	}
}

func (m *datastoreMigrator) DB(ctx context.Context) *gorm.DB {
	return m.dbGetter(ctx)
}

func (m *datastoreMigrator) ScanMigrationFiles(ctx context.Context, migrationsDirPath string) error {
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
			m.logger.WithError(err0).
				WithField("file", filename).
				Error("ScanMigrationFiles -- Problem reading migration file content")
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

		err0 = m.SaveMigrationString(ctx, filename, string(migrationPatch), revertPatch)
		if err0 != nil {
			m.logger.WithError(err0).
				WithField("file", filename).
				Error("ScanMigrationFiles -- new migration could not be saved")
			return err0
		}
	}
	return nil
}

func (m *datastoreMigrator) SaveMigrationString(
	ctx context.Context,
	filename string,
	migrationPatch string,
	revertPatch string,
) error {
	if m.DB(ctx) == nil {
		return errors.New("save migration: no database configured")
	}

	// If a file name exists, save with the name it has
	_, err := os.Stat(filename)
	if errors.Is(err, os.ErrNotExist) {
		filename = fmt.Sprintf("string:%s", filename)
	}

	migration := Migration{}

	err = m.DB(ctx).Model(&migration).First(&migration, "name = ?", filename).Error
	if err != nil {
		if !data.ErrorIsNoRows(err) {
			return fmt.Errorf("save migration lookup failed: %w", err)
		}

		migration = Migration{
			Name:        filename,
			Patch:       migrationPatch,
			RevertPatch: revertPatch,
		}
		err = m.DB(ctx).Create(&migration).Error
		if err != nil {
			return fmt.Errorf("save migration insert failed: %w", err)
		}
		return nil
	}

	if !migration.AppliedAt.Valid && migration.Patch != migrationPatch {
		err = m.DB(ctx).Model(&migration).
			Where("applied_at IS NULL").
			Update("patch", migrationPatch).Error
		if err != nil {
			return fmt.Errorf("save migration patch update failed: %w", err)
		}
	}
	if !migration.AppliedAt.Valid && revertPatch != "" && migration.RevertPatch != revertPatch {
		err = m.DB(ctx).Model(&migration).
			Where("applied_at IS NULL").
			Update("revert_patch", revertPatch).Error
		if err != nil {
			return fmt.Errorf("save migration revert patch update failed: %w", err)
		}
	}

	return nil
}

func (m *datastoreMigrator) ApplyNewMigrations(ctx context.Context) error {
	db := m.DB(ctx)
	if db == nil {
		return errors.New("apply migrations: no database configured")
	}

	var unAppliedMigrations []*Migration
	err := db.Where("applied_at IS NULL").Order("name ASC").Find(&unAppliedMigrations).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			m.logger.Info("ApplyNewMigrations -- No migrations found to be applied ")
			return nil
		}
		return err
	}

	for _, migration := range unAppliedMigrations {
		err = db.Transaction(func(tx *gorm.DB) error {
			var lockRow Migration
			lockErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				First(&lockRow, "id = ?", migration.ID).Error
			if lockErr != nil {
				if data.ErrorIsNoRows(lockErr) {
					return nil
				}
				return lockErr
			}

			if lockRow.AppliedAt.Valid {
				return nil
			}

			if execErr := tx.Exec(lockRow.Patch).Error; execErr != nil {
				return execErr
			}

			return tx.Exec(
				"UPDATE migrations SET applied_at = ? WHERE id = ? AND applied_at IS NULL",
				time.Now().UTC(),
				lockRow.ID,
			).Error
		})
		if err != nil {
			return err
		}

		m.logger.WithField("migration", migration.Name).Debug("ApplyNewMigrations -- Successfully applied migration")
	}

	return nil
}
