package frame

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"gocloud.dev/postgres"
	"gocloud.dev/server/health/sqlhealth"
	"gorm.io/datatypes"
	gormPostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"log"
	"math/rand"
)

type store struct {
	writeDatabase []*gorm.DB
	readDatabase  []*gorm.DB
}

func tenantPartition(ctx context.Context) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		authClaim := ClaimsFromContext(ctx)
		if authClaim != nil && !authClaim.isSystem() {

			return db.Where("tenant_id = ? AND partition_id = ?", authClaim.TenantID, authClaim.PartitionID)
		} else {
			return db
		}
	}
}

func DBPropertiesToMap(props datatypes.JSONMap) map[string]string {

	payload := make(map[string]string)
	properties := make(map[string]interface{})
	payloadValue, _ := props.MarshalJSON()
	err := json.Unmarshal(payloadValue, &properties)
	if err != nil {
		return payload
	}

	for k, val := range properties {
		marVal, err := json.Marshal(val)
		if err != nil {
			continue
		}
		payload[k] = string(marVal)

	}

	return payload

}

func DBPropertiesFromMap(propsMap map[string]string) datatypes.JSONMap {

	jsonMap := make(datatypes.JSONMap)

	for k, val := range propsMap {
		var prop interface{}
		err := json.Unmarshal([]byte(val), prop)
		if err != nil {
			continue
		}
		jsonMap[k] = prop
	}

	return jsonMap

}

func DBErrorIsRecordNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
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

	return db.WithContext(ctx).Scopes(tenantPartition(ctx))
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
			log.Printf("Datastore -- problem instantiating database : %v", err)
		}

		if db != nil {

			gormDb, _ := gorm.Open(gormPostgres.New(gormPostgres.Config{Conn: db}), &gorm.Config{
				SkipDefaultTransaction: true,
			})

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

	if migrationsDirPath == "" {
		migrationsDirPath = "./migrations/0001"
	}

	migrations = append(migrations, &Migration{})

	// Migrate the schema
	err := s.DB(ctx, false).AutoMigrate(migrations...)
	if err != nil {
		log.Printf("Error scanning for new migrations : %v ", err)
		return err
	}

	migrator := migrator{service: s}

	if err := migrator.scanForNewMigrations(ctx, migrationsDirPath); err != nil {
		log.Printf("Error scanning for new migrations : %v ", err)
		return err
	}

	if err := migrator.applyNewMigrations(ctx); err != nil {
		log.Printf("There was an error applying migrations : %v ", err)
		return err
	}
	return nil
}
