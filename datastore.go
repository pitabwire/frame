package frame

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"gocloud.dev/server/health/sqlhealth"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/plugin/opentelemetry/tracing"
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
		if authClaim != nil &&
			authClaim.TenantID != "" &&
			authClaim.PartitionID != "" &&
			!authClaim.isSystem() {
			return db.Where("tenant_id = ? AND partition_id = ?", authClaim.TenantID, authClaim.PartitionID)
		}
		return db
	}
}

// DBPropertiesToMap converts the supplied db json content into a golang map
func DBPropertiesToMap(props json.Marshaler) map[string]string {

	payload := make(map[string]string)

	if props == nil {
		return payload
	}

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

// DBPropertiesFromMap converts a map into a JSONMap object
func DBPropertiesFromMap(propsMap map[string]string) datatypes.JSONMap {
	jsonMap := make(datatypes.JSONMap)

	if propsMap == nil {
		return jsonMap
	}

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

// DBErrorIsRecordNotFound validate if supplied error is because of record missing in DB
func DBErrorIsRecordNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

// DB obtains an already instantiated db connection with the option
// to specify if you want write or read only db connection
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

// DatastoreCon Option method to store a connection that will be utilized when connecting to the database
func DatastoreCon(postgresqlConnection string, readOnly bool) Option {
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

		config, err := pgx.ParseConfig(postgresqlConnection)
		if err != nil {
			log.Printf("Datastore -- problem parsing database connection : %s", err)
		}

		db := stdlib.OpenDB(*config)
		gormDB, _ := gorm.Open(
			postgres.New(postgres.Config{Conn: db}),
			&gorm.Config{SkipDefaultTransaction: true},
		)

		_ = gormDB.Use(tracing.NewPlugin())

		s.AddCleanupMethod(func(ctx context.Context) {
			_ = db.Close()
		})
		if readOnly {
			s.dataStore.readDatabase = append(s.dataStore.readDatabase, gormDB)
		} else {
			s.dataStore.writeDatabase = append(s.dataStore.writeDatabase, gormDB)
		}

		addSQLHealthChecker(s, db)

	}
}

func Datastore(ctx context.Context) Option {
	return func(s *Service) {
		config, ok := s.Config().(ConfigurationDatabase)
		if !ok {
			s.L().Warn("configuration object not of type : ConfigurationDatabase")
			return
		}

		primaryDatabase := DatastoreCon(config.GetDatabasePrimaryHostURL(), false)
		primaryDatabase(s)
		replicaURL := config.GetDatabaseReplicaHostURL()
		if replicaURL == "" {
			replicaURL = config.GetDatabasePrimaryHostURL()
		}
		replicaDatabase := DatastoreCon(replicaURL, true)
		replicaDatabase(s)
	}
}

// addSqlHealthChecker returns a health check for the database.
func addSQLHealthChecker(s *Service, db *sql.DB) {
	dbCheck := sqlhealth.New(db)
	s.AddHealthCheck(dbCheck)
	s.AddCleanupMethod(func(ctx context.Context) {
		dbCheck.Stop()
	})
}
