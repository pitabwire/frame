package frame

import (
	"context"
	"gorm.io/gorm"
	"io/ioutil"
	"testing"
)

const testDatastoreConnection = "postgres://frame:secret@localhost:5423/framedatabase?sslmode=disable"

func TestService_Datastore(t *testing.T) {
	ctx := context.Background()
	mainDb := Datastore(ctx, testDatastoreConnection, false)

	srv := NewService("Test Srv", mainDb)

	if srv.name != "Test Srv" {
		t.Errorf("s")
	}

	w := srv.DB(ctx, false)
	if w == nil {
		t.Errorf("No default service could be instantiated")
		return
	}

	r := srv.DB(ctx, true)
	if r == nil {
		t.Errorf("Could not get read db instantiated")
		return
	}

	wd, _ := w.DB()
	rd, _ := r.DB()
	if wd != rd {
		t.Errorf("Read and write db services should not be different ")
	}

	srv.Stop()
}

func TestService_DatastoreRead(t *testing.T) {
	ctx := context.Background()
	mainDb := Datastore(ctx, testDatastoreConnection, false)
	readDb := Datastore(ctx, testDatastoreConnection, true)

	srv := NewService("Test Srv", mainDb, readDb)

	w := srv.DB(ctx, false)
	r := srv.DB(ctx, true)
	if w == nil || r == nil {
		t.Errorf("Read and write services setup but one couldn't be found")
		return
	}

	wd, _ := w.DB()
	rd, _ := r.DB()
	if wd == rd {
		t.Errorf("Read and write db services are same but we set different")
	}

}

func TestService_DatastoreNotSet(t *testing.T) {
	ctx := context.Background()

	srv := NewService("Test Srv")

	w := srv.DB(ctx, false)
	if w != nil {
		t.Errorf("When no connection is set no db is expected")
	}

}

func TestService_MigrateDatastore(t *testing.T) {

	ctx := context.Background()
	mainDb := Datastore(ctx, testDatastoreConnection, false)

	srv := NewService("Test Migrations Srv", mainDb)
	srv.DB(ctx, false).Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&Migration{})

	migrationPath := "./migrations/default"

	err := srv.MigrateDatastore(ctx, migrationPath)
	if err != nil {
		t.Errorf("Could not migrate successfully because : %v", err)
	}

}

func TestSaveNewMigrations(t *testing.T) {

	ctx := context.Background()
	mainDb := Datastore(ctx, testDatastoreConnection, false)
	srv := NewService("Test Migrations Srv", mainDb)

	srv.DB(ctx, false).Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&Migration{})

	err := srv.MigrateDatastore(ctx, "./tests_runner/migrations/default")
	if err != nil {
		t.Errorf("Could not migrate successfully because : %v", err)
		return
	}

	migrationPath := "./tests_runner/migrations/scans/scanned_select.sql"

	err = srv.DB(ctx, false).Where("name = ?", migrationPath).Unscoped().Delete(&Migration{}).Error
	if err != nil {
		t.Errorf("Could not ensure migrations are clean%v", err)
		return
	}

	migrationContent, err := ioutil.ReadFile(migrationPath)
	if err != nil {
		t.Errorf("Could not read scanned migration %v", err)
		return
	}



	err = saveNewMigrations(srv.DB(ctx, false), migrationPath, string(migrationContent))
	if err != nil {
		t.Errorf("Could not save new migration %v", err)
		return
	}

	migration := Migration{Name: migrationPath}
	err = srv.DB(ctx, false).First(&migration, "name = ?", migrationPath).Error
	if err != nil || migration.ID == "" {
		t.Errorf("Migration was not saved successfully %v", err)
		return
	}

	updateSql := "SELECT 2;"
	err = saveNewMigrations(srv.DB(ctx, false), migrationPath, updateSql)
	if err != nil {
		t.Errorf("Could not update unapplied migration %v", err)
		return
	}

	updatedMigration := Migration{Name: migrationPath}
	err = srv.DB(ctx, false).First(&updatedMigration, "name = ?", migrationPath).Error
	if err != nil {
		t.Errorf("Migration was not updated successfully %v", err)
		return
	}

	if migration.ID != updatedMigration.ID {
		t.Errorf("Migration ids do not match %s and %s", migration.ID, updatedMigration.ID)
		return
	}

	if updatedMigration.Patch != updateSql {
		t.Errorf("Migration was not updated successfully %s to %s", updatedMigration.Patch, updateSql)
		return
	}

}



func TestApplyMigrations(t *testing.T) {

	ctx := context.Background()
	mainDb := Datastore(ctx, testDatastoreConnection, false)
	srv := NewService("Test Migrations Srv", mainDb)

	srv.DB(ctx, false).Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&Migration{})

	err := srv.MigrateDatastore(ctx, "./tests_runner/migrations/default")
	if err != nil {
		t.Errorf("Could not migrate successfully because : %v", err)
		return
	}

	migrationPath := "./tests_runner/migrations/applied/apply_select.sql"

	err = srv.DB(ctx, false).Where("name = ?", migrationPath).Unscoped().Delete(&Migration{}).Error
	if err != nil {
		t.Errorf("Could not ensure migrations are clean%v", err)
		return
	}

	migrationContent, err := ioutil.ReadFile(migrationPath)
	if err != nil {
		t.Errorf("Could not read scanned migration %v", err)
		return
	}


	err = saveNewMigrations(srv.DB(ctx, false), migrationPath, string(migrationContent))
	if err != nil {
		t.Errorf("Could not save new migration %v", err)
		return
	}

	migration := Migration{Name: migrationPath}
	err = srv.DB(ctx, false).First(&migration, "name = ?", migrationPath).Error
	if err != nil || migration.AppliedAt.Valid {
		t.Errorf("Migration was not applied successfully %v", err)
		return
	}

	err = applyNewMigrations(srv.DB(ctx, false))
	if err != nil {
		t.Errorf("Could not save new migration %v", err)
		return
	}

	appliedMigration := Migration{Name: migrationPath}
	err = srv.DB(ctx, false).First(&appliedMigration, "name = ?", migrationPath).Error
	if err != nil || !appliedMigration.AppliedAt.Valid {
		t.Errorf("Migration was not applied successfully %v", err)
		return
	}


}
