package frame

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"gocloud.dev/postgres"
	"gocloud.dev/server/health/sqlhealth"
	gormPostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"io/ioutil"
	"log"
	"math/rand"
	"path/filepath"
	"strings"
	"time"
)

type store struct {
	writeDatabase []*gorm.DB
	readDatabase  []*gorm.DB
}

func TenantPartition(ctx context.Context) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		authClaim := ClaimsFromContext(ctx)
		if authClaim != nil {
			return db.Where("tenant_id = ? AND partition_id = ?", authClaim.TenantID, authClaim.PartitionID)
		} else {
			return db
		}
	}
}

func (s *Service) DB(ctx context.Context, readOnly bool) *gorm.DB {

	var db *gorm.DB

	if readOnly {

		replicaCount := len(s.dataStore.readDatabase)
		if replicaCount > 0 {
			randomIndex := 0
			if replicaCount > 1 {
				randomIndex = rand.Intn(replicaCount)
			}
			db = s.dataStore.readDatabase[randomIndex]
		}
	}

	if db == nil {

		writeCount := len(s.dataStore.writeDatabase)
		if writeCount == 0 {
			log.Printf("DB -- attempting use a database when none is setup")
			return nil
		}

		randomIndex := 0
		if writeCount > 1 {
			randomIndex = rand.Intn(writeCount)
		}
		db = s.dataStore.writeDatabase[randomIndex]
	}

	return db.WithContext(ctx).Scopes(TenantPartition(ctx))
}

func Datastore(ctx context.Context, postgresqlConnection string, readOnly bool) Option {
	return func(s *Service) {

		if s.dataStore == nil {
			s.dataStore = &store{
				writeDatabase: []*gorm.DB{},
				readDatabase:  []*gorm.DB{},
			}
		}

		if s.dataStore.writeDatabase == nil {
			s.dataStore.writeDatabase = []*gorm.DB{}
		}

		if s.dataStore.readDatabase == nil {
			s.dataStore.readDatabase = []*gorm.DB{}
		}
		db, err := postgres.Open(ctx, postgresqlConnection)
		if err != nil {
			log.Printf("AddDB -- problem instantiating database : %v", err)
		}

		if db != nil {

			gormDb, _ := gorm.Open(gormPostgres.New(gormPostgres.Config{Conn: db}), &gorm.Config{})

			s.AddCleanupMethod(func() {
				_ = db.Close()
			})

			if readOnly {
				s.dataStore.readDatabase = append(s.dataStore.readDatabase, gormDb)
			} else {
				s.dataStore.writeDatabase = append(s.dataStore.writeDatabase, gormDb)
			}

			addSqlHealthChecker(s, db)
		}
	}
}

// addSqlHealthChecker returns a health check for the database.
func addSqlHealthChecker(s *Service, db *sql.DB) {
	dbCheck := sqlhealth.New(db)
	s.AddHealthCheck(dbCheck)
	s.AddCleanupMethod(func() {
		dbCheck.Stop()
	})
}

// MigrateDatastore finds missing migrations and records them in the database
func (s *Service) MigrateDatastore(ctx context.Context, migrationsDirPath string, migrations ...interface{}) error {

	db := s.DB(ctx, false)
	if migrationsDirPath == "" {
		migrationsDirPath = "./migrations/0001"
	}

	migrations = append(migrations, &Migration{})

	// Migrate the schema
	err := db.AutoMigrate(migrations...)
	if err != nil {
		log.Printf("Error scanning for new migrations : %v ", err)
		return err
	}
	if err := scanForNewMigrations(db, migrationsDirPath); err != nil {
		log.Printf("Error scanning for new migrations : %v ", err)
		return err
	}

	if err := applyNewMigrations(db); err != nil {
		log.Printf("There was an error applying migrations : %v ", err)
		return err
	}
	return nil
}

func scanForNewMigrations(db *gorm.DB, migrationsDirPath string) error {

	// Get a list of migration files
	files, err := filepath.Glob(migrationsDirPath + "/*.sql")
	if err != nil {
		return err
	}

	log.Printf("scanForNewMigrations found %d migrations to process", len(files))

	for _, file := range files {

		filename := filepath.Base(file)
		filename = strings.Replace(filename, ".sql", "", 1)

		migrationPatch, err := ioutil.ReadFile(file)

		if err != nil {
			log.Printf("Problem reading migration file content : %v", err)
			continue
		}

		err = saveNewMigrations(db, filename, string(migrationPatch))
		if err != nil {
			log.Printf("new migration :%s could not be processed because: %+v", file, err)
		}

	}
	return nil
}



func saveNewMigrations(db *gorm.DB, filename string, migrationPatch string) error {

	migration := Migration{Name: filename}

	result := db.First(&migration, "name = ?", filename)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {

		log.Printf("migration %s is unapplied", filename)
		migration.Patch = migrationPatch

		err := db.Save(&migration).Error
		if err != nil {
			return err
		}
	} else {

		if !migration.AppliedAt.Valid {

			if migration.Patch != migrationPatch {

				err := db.Model(&migration).Update("patch", string(migrationPatch)).Error

				if err != nil {
					return err
				}
			}
		}

	}

	return nil
}


func applyNewMigrations(db *gorm.DB) error {

	var unAppliedMigrations []Migration
	if err := db.Where("applied_at IS NULL").Find(&unAppliedMigrations).Error; err != nil {

		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("No migrations found to be applied ")
			return nil
		}
		return err
	}

	for _, migration := range unAppliedMigrations {

		if err := db.Exec(migration.Patch).Error; err != nil {
			return err
		}

		migration.AppliedAt = sql.NullTime{
			Time: time.Now(),
			Valid: true,
		}
		if err := db.Model(&migration).Save(migration).Error; err != nil {
			return err
		}

		log.Printf("Successfully applied the file : %v", fmt.Sprintf("%s.sql", migration.Name))
	}

	return nil
}
