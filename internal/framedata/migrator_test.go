package framedata_test

import (
	"os"
	"testing"

	"github.com/pitabwire/util"
	"gorm.io/gorm"

	"github.com/pitabwire/frame"
)

func TestSaveNewMigrations(t *testing.T) {
	testDBURL := util.GetEnv(
		"TEST_DATABASE_URL",
		"postgres://frame:secret@localhost:5435/framedatabase?sslmode=disable",
	)
	ctx, svc := frame.NewService("Test Migrations Srv")

	mainDB := frame.WithDatastoreConnection(testDBURL, false)
	svc.Init(ctx, mainDB)

	svc.DB(ctx, false).Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&frame.Migration{})

	err := svc.MigrateDatastore(ctx, "./tests_runner/migrations/default")
	if err != nil {
		t.Errorf("Could not migrate successfully because : %s", err)
		return
	}

	migrationPath := "./tests_runner/migrations/scans/scanned_select.sql"

	err = svc.DB(ctx, false).Where("name = ?", migrationPath).Unscoped().Delete(&frame.Migration{}).Error
	if err != nil {
		t.Errorf("Could not ensure migrations are clean%s", err)
		return
	}

	migrationContent, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Errorf("Could not read scanned migration %s", err)
		return
	}

	pool := svc.DBPool()
	testMigrator := svc.NewMigrator(ctx, pool)

	err = testMigrator.SaveMigrationString(ctx, migrationPath, string(migrationContent), "")
	if err != nil {
		t.Errorf("Could not save new migration %s", err)
		return
	}

	migration := frame.Migration{Name: migrationPath}
	err = svc.DB(ctx, false).First(&migration, "name = ?", migrationPath).Error
	if err != nil || migration.ID == "" {
		t.Errorf("Migration was not saved successfully %s", err)
		return
	}

	updateSQL := "SELECT 2;"
	err = testMigrator.SaveMigrationString(ctx, migrationPath, updateSQL, "")
	if err != nil {
		t.Errorf("Could not update unapplied migration %s", err)
		return
	}

	updatedMigration := frame.Migration{Name: migrationPath}
	err = svc.DB(ctx, false).First(&updatedMigration, "name = ?", migrationPath).Error
	if err != nil {
		t.Errorf("Migration was not updated successfully %s", err)
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
	testDBURL := util.GetEnv(
		"TEST_DATABASE_URL",
		"postgres://frame:secret@localhost:5435/framedatabase?sslmode=disable",
	)

	defConf, err := frame.ConfigFromEnv[frame.ConfigurationDefault]()
	if err != nil {
		t.Errorf("Could not processFunc test configurations %v", err)
		return
	}
	defConf.DatabaseSlowQueryLogThreshold = "5ms"
	defConf.DatabaseTraceQueries = true
	defConf.LogLevel = "debug"

	ctx, svc := frame.NewService("Test Migrations Srv", frame.WithConfig(&defConf))

	mainDB := frame.WithDatastoreConnection(testDBURL, false)
	svc.Init(ctx, mainDB)

	svc.DB(ctx, false).Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&frame.Migration{})

	err = svc.MigrateDatastore(ctx, "./tests_runner/migrations/default")
	if err != nil {
		t.Errorf("Could not migrate successfully because : %s", err)
		return
	}

	migrationPath := "./tests_runner/migrations/applied/apply_select.sql"

	err = svc.DB(ctx, false).Where("name = ?", migrationPath).Unscoped().Delete(&frame.Migration{}).Error
	if err != nil {
		t.Errorf("Could not ensure migrations are clean%s", err)
		return
	}

	migrationContent, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Errorf("Could not read scanned migration %s", err)
		return
	}

	pool := svc.DBPool()
	testMigrator := svc.NewMigrator(ctx, pool)

	err = testMigrator.SaveMigrationString(ctx, migrationPath, string(migrationContent), "")
	if err != nil {
		t.Errorf("Could not save new migration %s", err)
		return
	}

	migration := frame.Migration{Name: migrationPath}
	err = svc.DB(ctx, false).First(&migration, "name = ?", migrationPath).Error
	if err != nil || migration.AppliedAt.Valid {
		t.Errorf("Migration was not applied successfully %s", err)
		return
	}

	err = testMigrator.ApplyNewMigrations(ctx)
	if err != nil {
		t.Errorf("Could not save new migration %s", err)
		return
	}

	appliedMigration := frame.Migration{Name: migrationPath}
	err = svc.DB(ctx, false).First(&appliedMigration, "name = ?", migrationPath).Error
	if err != nil || !appliedMigration.AppliedAt.Valid {
		t.Errorf("Migration was not applied successfully %s", err)
		return
	}
}

func TestService_MigrateDatastore(t *testing.T) {
	testDBURL := util.GetEnv(
		"TEST_DATABASE_URL",
		"postgres://frame:secret@localhost:5435/framedatabase?sslmode=disable",
	)

	ctx, srv := frame.NewService("Test Migrations Srv")

	mainDB := frame.WithDatastoreConnection(testDBURL, false)
	srv.Init(ctx, mainDB)

	srv.DB(ctx, false).Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&frame.Migration{})

	migrationPath := "./migrations/default"

	err := srv.MigrateDatastore(ctx, migrationPath)
	if err != nil {
		t.Errorf("Could not migrate successfully because : %s", err)
	}
}

func TestService_MigrateDatastoreIDempotency(t *testing.T) {
	testDBURL := util.GetEnv(
		"TEST_DATABASE_URL",
		"postgres://frame:secret@localhost:5435/framedatabase?sslmode=disable",
	)

	ctx, srv := frame.NewService("Test Migrations Srv")

	mainDB := frame.WithDatastoreConnection(testDBURL, false)
	srv.Init(ctx, mainDB)

	srv.DB(ctx, false).Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&frame.Migration{})

	migrationPath := "./migrations/default"

	err := srv.MigrateDatastore(ctx, migrationPath)
	if err != nil {
		t.Errorf("Could not migrate successfully because : %s", err)
	}
	err = srv.MigrateDatastore(ctx, migrationPath)
	if err != nil {
		t.Errorf("Could not migrate successfully second time because : %s", err)
	}
	err = srv.MigrateDatastore(ctx, migrationPath)
	if err != nil {
		t.Errorf("Could not migrate successfully third time because : %s", err)
	}
}
