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
	"gorm.io/gorm/logger"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

type store struct {
	writeDatabase []*gorm.DB
	readDatabase  []*gorm.DB
	randomSource  rand.Source
}

func newDataStore() *store {
	return &store{
		randomSource:  rand.NewSource(time.Now().UnixNano()),
		readDatabase:  []*gorm.DB{},
		writeDatabase: []*gorm.DB{},
	}
}

// Returns a random item from the slice, or an error if the slice is empty
func (s *Service) getRandomDatastoreConnection(readOnly bool) *gorm.DB {

	var connectionPool []*gorm.DB
	if readOnly {
		connectionPool = s.dataStore.readDatabase
	} else {
		connectionPool = s.dataStore.writeDatabase
	}

	if len(connectionPool) == 0 {
		return nil
	}
	randomIndex := rand.New(s.dataStore.randomSource).Intn(len(connectionPool))
	return connectionPool[randomIndex]
}

func tenantPartition(ctx context.Context) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		authClaim := ClaimsFromContext(ctx)
		if authClaim == nil {
			return db
		}
		if authClaim.GetTenantId() == "" ||
			authClaim.GetPartitionId() == "" {
			if !authClaim.isInternalSystem() {
				_ = db.AddError(errors.New("tenancy scope not present in context"))
				return db
			}
		}

		return db.Where("tenant_id = ? AND partition_id = ?", authClaim.GetTenantId(), authClaim.GetPartitionId())
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
		db = s.getRandomDatastoreConnection(true)
	}

	if db == nil {
		db = s.getRandomDatastoreConnection(false)
		if db == nil {
			s.L(ctx).Error("DB -- attempting to use a database when none is setup")
			return nil
		}
	}

	partitionedDb := db.WithContext(ctx).Scopes(tenantPartition(ctx))

	config, ok := s.Config().(ConfigurationLogLevel)
	if ok && config.LoggingLevelIsDebug() {
		return partitionedDb.Debug()
	}

	return partitionedDb
}

// DatastoreConnection Option method to store a connection that will be utilized when connecting to the database
func DatastoreConnection(ctx context.Context, postgresqlConnection string, readOnly bool) Option {

	return func(s *Service) {
		if s.dataStore == nil {
			s.dataStore = newDataStore()
		}

		dbQueryLogger := logger.Default.LogMode(logger.Warn)
		logConfig, ok := s.Config().(ConfigurationLogLevel)
		if ok {
			if logConfig.LoggingLevelIsDebug() {
				dbQueryLogger = logger.Default.LogMode(logger.Info)
			}
		}

		connConfig, err := pgx.ParseConfig(postgresqlConnection)
		if err != nil {
			log := s.L(ctx).WithError(err).WithField("pgConnection", postgresqlConnection)
			log.Error("Datastore -- problem parsing database connection")
		}

		db := stdlib.OpenDB(*connConfig)

		skipDefaultTx := true
		dbConfig, ok0 := s.Config().(ConfigurationDatabase)
		if ok0 {
			skipDefaultTx = dbConfig.SkipDefaultTransaction()
			// Set connection pool parameters
			db.SetMaxIdleConns(dbConfig.GetMaxIdleConnections())                // Max idle connections
			db.SetMaxOpenConns(dbConfig.GetMaxOpenConnections())                // Max open connections
			db.SetConnMaxLifetime(dbConfig.GetMaxConnectionLifeTimeInSeconds()) // Max connection lifetime

		}

		gormDB, _ := gorm.Open(
			postgres.New(postgres.Config{Conn: db, PreferSimpleProtocol: true}),
			&gorm.Config{
				SkipDefaultTransaction: skipDefaultTx,
				NowFunc: func() time.Time {
					utc, _ := time.LoadLocation("")
					return time.Now().In(utc)
				},
				Logger: dbQueryLogger,
			},
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
			s.L(ctx).Warn("configuration object not of type : ConfigurationDatabase")
			return
		}

		for _, primaryDbURL := range config.GetDatabasePrimaryHostURL() {
			primaryDatabase := DatastoreConnection(ctx, primaryDbURL, false)
			primaryDatabase(s)
		}

		for _, replicaDbURL := range config.GetDatabaseReplicaHostURL() {
			replicaDatabase := DatastoreConnection(ctx, replicaDbURL, true)
			replicaDatabase(s)
		}
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
