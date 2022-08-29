package frame

import (
	"context"
	"gorm.io/gorm"
	"os"
	"testing"
)

func TestSaveNewMigrations(t *testing.T) {
	ctx := context.Background()
	testDBURL := GetEnv("TEST_DATABASE_URL", "postgres://frame:secret@localhost:5431/framedatabase?sslmode=disable")
	mainDB := Datastore(ctx, testDBURL, false)
	srv := NewService("Test Migrations Srv", mainDB)

	srv.DB(ctx, false).Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&Migration{})

	err := srv.MigrateDatastore(ctx, "./tests_runner/migrations/default")
	if err != nil {
		t.Errorf("Could not migrate successfully because : %+v", err)
		return
	}

	migrationPath := "./tests_runner/migrations/scans/scanned_select.sql"

	err = srv.DB(ctx, false).Where("name = ?", migrationPath).Unscoped().Delete(&Migration{}).Error
	if err != nil {
		t.Errorf("Could not ensure migrations are clean%+v", err)
		return
	}

	migrationContent, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Errorf("Could not read scanned migration %+v", err)
		return
	}

	migrator := migrator{service: srv}

	err = migrator.saveNewMigrations(ctx, migrationPath, string(migrationContent))
	if err != nil {
		t.Errorf("Could not save new migration %+v", err)
		return
	}

	migration := Migration{Name: migrationPath}
	err = srv.DB(ctx, false).First(&migration, "name = ?", migrationPath).Error
	if err != nil || migration.ID == "" {
		t.Errorf("Migration was not saved successfully %+v", err)
		return
	}

	updateSQL := "SELECT 2;"
	err = migrator.saveNewMigrations(ctx, migrationPath, updateSQL)
	if err != nil {
		t.Errorf("Could not update unapplied migration %+v", err)
		return
	}

	updatedMigration := Migration{Name: migrationPath}
	err = srv.DB(ctx, false).First(&updatedMigration, "name = ?", migrationPath).Error
	if err != nil {
		t.Errorf("Migration was not updated successfully %+v", err)
		return
	}

	if migration.ID != updatedMigration.ID {
		t.Errorf("Migration ids do not match %s and %s", migration.ID, updatedMigration.ID)
		return
	}

	if updatedMigration.Patch != updateSQL {
		t.Errorf("Migration was not updated successfully %s to %s", updatedMigration.Patch, updateSQL)
		return
	}
}

func TestApplyMigrations(t *testing.T) {
	ctx := context.Background()
	testDBURL := GetEnv("TEST_DATABASE_URL", "postgres://frame:secret@localhost:5431/framedatabase?sslmode=disable")
	mainDB := Datastore(ctx, testDBURL, false)
	srv := NewService("Test Migrations Srv", mainDB)

	srv.DB(ctx, false).Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&Migration{})

	err := srv.MigrateDatastore(ctx, "./tests_runner/migrations/default")
	if err != nil {
		t.Errorf("Could not migrate successfully because : %+v", err)
		return
	}

	migrationPath := "./tests_runner/migrations/applied/apply_select.sql"

	err = srv.DB(ctx, false).Where("name = ?", migrationPath).Unscoped().Delete(&Migration{}).Error
	if err != nil {
		t.Errorf("Could not ensure migrations are clean%+v", err)
		return
	}

	migrationContent, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Errorf("Could not read scanned migration %+v", err)
		return
	}

	migrator := migrator{service: srv}
	err = migrator.saveNewMigrations(ctx, migrationPath, string(migrationContent))
	if err != nil {
		t.Errorf("Could not save new migration %+v", err)
		return
	}

	migration := Migration{Name: migrationPath}
	err = srv.DB(ctx, false).First(&migration, "name = ?", migrationPath).Error
	if err != nil || migration.AppliedAt.Valid {
		t.Errorf("Migration was not applied successfully %+v", err)
		return
	}

	err = migrator.applyNewMigrations(ctx)
	if err != nil {
		t.Errorf("Could not save new migration %+v", err)
		return
	}

	appliedMigration := Migration{Name: migrationPath}
	err = srv.DB(ctx, false).First(&appliedMigration, "name = ?", migrationPath).Error
	if err != nil || !appliedMigration.AppliedAt.Valid {
		t.Errorf("Migration was not applied successfully %+v", err)
		return
	}

}

func TestService_MigrateDatastore(t *testing.T) {
	ctx := context.Background()
	testDBURL := GetEnv("TEST_DATABASE_URL", "postgres://frame:secret@localhost:5431/framedatabase?sslmode=disable")
	mainDB := Datastore(ctx, testDBURL, false)

	srv := NewService("Test Migrations Srv", mainDB)
	srv.DB(ctx, false).Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&Migration{})

	migrationPath := "./migrations/default"

	err := srv.MigrateDatastore(ctx, migrationPath)
	if err != nil {
		t.Errorf("Could not migrate successfully because : %+v", err)
	}

}
