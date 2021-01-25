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

	db *gorm.DB
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

	return db.WithContext(ctx)
}

func Datastore(ctx context.Context, postgresqlConnection string, readOnly bool) Option {
	return func(s *Service) {

		if s.dataStore == nil {
			s.dataStore = &store{
				writeDatabase: []*gorm.DB{},
				readDatabase:  []*gorm.DB{},
			}
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
//This will signal to Kubernetes or other orchestrators that the server should not receive
// traffic until the server is able to connect to its database.
func addSqlHealthChecker(s *Service, db *sql.DB) {
	dbCheck := sqlhealth.New(db)
	s.AddHealthCheck(dbCheck)
	s.AddCleanupMethod(func() {
		dbCheck.Stop()
	})
}

// PerformMigration finds missing migrations and records them in the database,
// We use the fragmenta_metadata table to do this
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

		var migration Migration

		filename := filepath.Base(file)
		filename = strings.Replace(filename, ".sql", "", 1)

		migration.Name = filename
		migrationPatch, err := ioutil.ReadFile(file)

		if err != nil {
			log.Printf("Problem reading migration file content : %v", err)
			continue
		}

		result := db.Where("name = ?", filename).Find(&migration)
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {

			log.Printf("migration %s is unapplied", file)
			migration.Patch = string(migrationPatch)

			err = db.Create(&migration).Error
			if err != nil {
				log.Printf("There is an error :%v adding migration :%s", err, file)
			}
		} else {

			if migration.AppliedAt == nil {

				if migration.Patch != string(migrationPatch) {
					err = db.Model(&migration).Where("name = ?", filename).Update("patch", string(migrationPatch)).Error

					if err != nil {
						log.Printf("There is an error updating migration :%s", file)
					}
				}
			}

		}
	}
	return nil
}

func applyNewMigrations(db *gorm.DB) error {

	var unAppliedMigrations []Migration
	if err := db.Where("applied_at IS NULL").Find(&unAppliedMigrations).Error; err != nil {
		return err
	}

	for _, migration := range unAppliedMigrations {

		if err := db.Exec(migration.Patch).Error; err != nil {
			return err
		}

		db.Model(&migration).UpdateColumn("applied_at", time.Now())
		log.Printf("Successfully applied the file : %v", fmt.Sprintf("%s.sql", migration.Name))
	}

	return nil
}
