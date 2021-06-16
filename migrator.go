package frame

import (
	"context"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"
	"time"
)

type migrator struct {
	service *Service
}

func (m *migrator)DB(ctx context.Context) *gorm.DB {
	return m.service.DB(ctx, false).Session(&gorm.Session{Context: ctx, PrepareStmt: false})
}


func (m *migrator) scanForNewMigrations(ctx context.Context, migrationsDirPath string) error {

	// Get a list of migration files
	files, err := filepath.Glob(migrationsDirPath + "/*.sql")
	if err != nil {
		return err
	}

	for _, file := range files {

		filename := filepath.Base(file)
		filename = strings.Replace(filename, ".sql", "", 1)

		migrationPatch, err := ioutil.ReadFile(file)

		if err != nil {
			log.Printf("Problem reading migration file content : %v", err)
			continue
		}

		err = m.saveNewMigrations(ctx, filename, string(migrationPatch))
		if err != nil {
			log.Printf("new migration :%s could not be processed because: %+v", file, err)
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
			log.Printf("No migrations found to be applied ")
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

		log.Printf("Successfully applied the file : %v", fmt.Sprintf("%s.sql", migration.Name))
	}

	return nil
}

