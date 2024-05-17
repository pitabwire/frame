package frame

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"gocloud.dev/server/health/sqlhealth"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"math/rand"
	"strconv"
	"strings"
)

type store struct {
	writeDatabase []*gorm.DB
	readDatabase  []*gorm.DB
}

func tenantPartition(ctx context.Context) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		authClaim := ClaimsFromContext(ctx)
		if authClaim != nil &&
			authClaim.GetTenantId() != "" &&
			authClaim.GetPartitionId() != "" &&
			!authClaim.isSystem() {
			return db.Where("tenant_id = ? AND partition_id = ?", authClaim.GetTenantId(), authClaim.GetPartitionId())
		}
		return db
	}
}

// DBPropertiesToMap converts the supplied db json content into a golang map
func DBPropertiesToMap(props datatypes.JSONMap) map[string]string {

	if props == nil {
		return make(map[string]string, len(props))
	}

	payload := make(map[string]string, len(props))

	for k, val := range props {

		switch v := val.(type) {
		case nil:
			payload[k] = ""
		case string:
			payload[k] = v

		case bool:
			payload[k] = strconv.FormatBool(v)
		case int, int64, int32, int16, int8:
			payload[k] = strconv.FormatInt(int64(val.(int)), 10)

		case float32, float64:
			payload[k] = strconv.FormatFloat(val.(float64), 'g', -1, 64)
		default:

			marVal, err1 := json.Marshal(val)
			if err1 != nil {
				payload[k] = fmt.Sprintf("%v", val)
				continue
			}
			payload[k] = string(marVal)
		}
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

		jsonMap[k] = val

		if !strings.HasPrefix(val, "{") && !strings.HasPrefix(val, "[") {
			continue
		}

		var prop any
		// Determine if the JSON is an object or an array and unmarshal accordingly
		if err := json.Unmarshal([]byte(val), &prop); err != nil {
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
			logger := s.L()
			logger.Error("DB -- attempting use a database when none is setup")
			return nil
		}

		randomIndex := 0
		if writeCount > 1 {
			randomIndex = rand.Intn(writeCount)
		}
		db = s.dataStore.writeDatabase[randomIndex]
	}

	partitionedDb := db.WithContext(ctx).Scopes(tenantPartition(ctx))

	config, ok := s.Config().(ConfigurationLogLevel)
	if ok && config.LoggingLevelIsDebug() {
		return partitionedDb.Debug()
	}

	return partitionedDb
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
			logger := s.L().WithError(err).WithField("pgConnection", postgresqlConnection)
			logger.Error("Datastore -- problem parsing database connection")
		}

		db := stdlib.OpenDB(*config)
		gormDB, _ := gorm.Open(
			postgres.New(postgres.Config{Conn: db, PreferSimpleProtocol: true}),
			&gorm.Config{SkipDefaultTransaction: true},
		)

		//_ = gormDB.Use(tracing.NewPlugin())

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
