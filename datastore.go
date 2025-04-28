package frame

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/driver/postgres"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/XSAM/otelsql"

	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const defaultStoreName = "__default__"

type Pool struct {
	ctx context.Context

	readIdx             uint64            // atomic counter for round-robin
	writeIdx            uint64            // atomic counter for round-robin
	mu                  sync.RWMutex      // protects db slices
	allReadDBs          map[*gorm.DB]bool // track all read DBs
	allWriteDBs         map[*gorm.DB]bool // track all write DBs
	lastHealthCheckTime time.Time

	healthCheckCancel  context.CancelFunc
	healthCheckStopped <-chan struct{}
	shouldDoMigrations bool
}

func newStore(ctx context.Context, srv *Service) *Pool {
	store := &Pool{
		ctx:                ctx,
		allReadDBs:         make(map[*gorm.DB]bool),
		allWriteDBs:        make(map[*gorm.DB]bool),
		healthCheckStopped: make(chan struct{}),
		shouldDoMigrations: true,
	}

	srv.AddCleanupMethod(store.cleanup)

	return store
}

// addConnection safely adds a DB connection to the pool.
func (s *Pool) addConnection(db *gorm.DB, readOnly bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if readOnly {
		if !s.allReadDBs[db] {
			s.allReadDBs[db] = true
		}
	} else {
		if !s.allWriteDBs[db] {
			s.allWriteDBs[db] = true
		}
	}
}

// markHealthy marks a DB connection as healthy or unhealthy it from the pool.
func (s *Pool) markHealthy(readOnly bool, db *gorm.DB, isHealthy bool, err error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if readOnly {
		s.allReadDBs[db] = isHealthy
	} else {
		s.allWriteDBs[db] = isHealthy
	}

	return err
}

// CheckHealth iterates all known DBs, pings them, and updates pool membership accordingly.
func (s *Pool) CheckHealth() error {

	healthCheckError := ""

	if s.DB(s.ctx, true) == nil {
		healthCheckError = "No healthy read db available"
	}
	if s.DB(s.ctx, false) == nil {
		healthCheckError += "No healthy write db available"
	}

	now := time.Now()
	if s.lastHealthCheckTime.IsZero() || now.Sub(s.lastHealthCheckTime) > 5*time.Minute {

		ctx, cancel := context.WithCancel(s.ctx)
		s.healthCheckCancel = cancel
		stopped := make(chan struct{})
		s.healthCheckStopped = stopped
		go func(ctx context.Context) {
			defer func() {
				close(stopped)
			}()

			for db := range s.allWriteDBs {
				_ = s.pingWithTimeoutAndRetry(ctx, false, db)
			}
			for db := range s.allReadDBs {
				_ = s.pingWithTimeoutAndRetry(ctx, true, db)
			}
			s.lastHealthCheckTime = now

		}(ctx)
	}
	if healthCheckError != "" {
		return errors.New(healthCheckError)
	}

	return nil
}

func (s *Pool) cleanup(_ context.Context) {

	if s.healthCheckCancel != nil {
		s.healthCheckCancel()
		<-s.healthCheckStopped
	}

	for db := range s.allReadDBs {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}
	for db := range s.allWriteDBs {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}
}

func (s *Pool) pingWithTimeoutAndRetry(ctx context.Context, readOnly bool, db *gorm.DB) error {
	var timer *time.Timer
	wait := 250 * time.Millisecond
	const maxWait = 30 * time.Second

	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	for {
		sqlDB, err := db.DB()
		if err != nil {
			return s.markHealthy(readOnly, db, false, err)
		}
		err = sqlDB.PingContext(ctx)
		if err == nil {
			return s.markHealthy(readOnly, db, true, nil)
		}
		if timer == nil {
			timer = time.NewTimer(wait)
		} else {
			// Timer already fired, so resetting does not race.
			timer.Reset(wait)
		}
		select {
		case <-timer.C:
			if wait < maxWait {
				// Back off next ping.
				wait *= 2
				if wait > maxWait {
					wait = maxWait
				}
			}
		case <-ctx.Done():
			return s.markHealthy(readOnly, db, false, ctx.Err())
		}
	}
}

// DB Returns a random item from the slice, or an error if the slice is empty
func (s *Pool) DB(ctx context.Context, readOnly bool) *gorm.DB {
	var pool []*gorm.DB
	var idx *uint64

	s.mu.RLock()
	if readOnly {
		for db, healthy := range s.allReadDBs {
			if healthy {
				pool = append(pool, db)
			}
		}
		idx = &s.readIdx
		if len(pool) != 0 {
			// This check ensures we are able to use the write db if no more read dbs exist
			return s.selectOne(pool, idx)
		}
	}

	for db, healthy := range s.allWriteDBs {
		if healthy {
			pool = append(pool, db)
		}
	}
	idx = &s.writeIdx

	s.mu.RUnlock()
	db := s.selectOne(pool, idx)

	if db == nil {
		return nil
	}

	return db.Session(&gorm.Session{NewDB: true}).WithContext(ctx).Scopes(tenantPartition(ctx))
}

// selectOne uses atomic round-robin for high concurrency.
func (s *Pool) selectOne(pool []*gorm.DB, idx *uint64) *gorm.DB {
	if len(pool) == 0 {
		return nil
	}
	pos := atomic.AddUint64(idx, 1)
	return pool[int(pos-1)%len(pool)]
}

// SetPoolConfig updates pool sizes and connection lifetimes at runtime for all DBs.
func (s *Pool) SetPoolConfig(maxOpen, maxIdle int, maxLifetime time.Duration) {
	for db := range s.allReadDBs {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.SetMaxOpenConns(maxOpen)
			sqlDB.SetMaxIdleConns(maxIdle)
			sqlDB.SetConnMaxLifetime(maxLifetime)
		}
	}
	for db := range s.allWriteDBs {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.SetMaxOpenConns(maxOpen)
			sqlDB.SetMaxIdleConns(maxIdle)
			sqlDB.SetConnMaxLifetime(maxLifetime)
		}
	}
}

func (s *Pool) CanMigrate() bool {
	return s.shouldDoMigrations
}

func tenantPartition(ctx context.Context) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {

		authClaim := ClaimsFromContext(ctx)
		if authClaim == nil {
			return db
		}

		skipTenancyChecksOnClaims := IsTenancyChecksOnClaimSkipped(ctx)
		if skipTenancyChecksOnClaims {
			return db
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

func (s *Service) DBPool(name string) *Pool {
	v, ok := s.dataStores.Load(name)
	if !ok {
		return nil
	}

	return v.(*Pool)
}

// DB obtains an already instantiated db connection with the option
// to specify if you want write or read only db connection
func (s *Service) DB(ctx context.Context, readOnly bool) *gorm.DB {
	return s.DBWithName(ctx, defaultStoreName, readOnly)
}

func (s *Service) DBWithName(ctx context.Context, name string, readOnly bool) *gorm.DB {

	store := s.DBPool(name)
	if store == nil {
		return nil
	}
	return store.DB(ctx, readOnly)
}

// DatastoreConnection Option method to store a connection that will be utilized when connecting to the database
func DatastoreConnection(ctx context.Context, postgresqlConnection string, readOnly bool) Option {
	return DatastoreConnectionWithName(ctx, defaultStoreName, postgresqlConnection, readOnly)
}
func DatastoreConnectionWithName(ctx context.Context, name string, postgresqlConnection string, readOnly bool) Option {

	return func(s *Service) {

		dbQueryLogger := logger.Default.LogMode(logger.Warn)
		logConfig, ok := s.Config().(ConfigurationLogLevel)
		if ok {
			if logConfig.LoggingLevelIsDebug() {
				dbQueryLogger = logger.Default.LogMode(logger.Info)
			}
		}

		cleanedPostgresqlDSN, err := cleanPostgresDSN(postgresqlConnection)
		if err != nil {
			s.L(ctx).WithError(err).WithField("dsn", postgresqlConnection).Fatal("could not get a clean postgresql dsn")
			return
		}

		preferSimpleProtocol := true
		skipDefaultTransaction := true

		dbConfig, _ := s.Config().(ConfigurationDatabase)
		if dbConfig != nil {
			preferSimpleProtocol = dbConfig.PreferSimpleProtocol()
			skipDefaultTransaction = dbConfig.SkipDefaultTransaction()
		}

		conn, err := otelsql.Open("pgx", cleanedPostgresqlDSN,
			otelsql.WithAttributes(
				semconv.DBSystemNamePostgreSQL,
				attribute.String("service.name", s.name),
			),
		)
		if err != nil {
			s.L(ctx).WithError(err).WithField("dsn", postgresqlConnection).Error("could not connect to pg now")
			return
		}

		gormDB, _ := gorm.Open(
			postgres.New(postgres.Config{
				Conn:                 conn,
				PreferSimpleProtocol: preferSimpleProtocol,
			}),
			&gorm.Config{
				SkipDefaultTransaction: skipDefaultTransaction,
				NowFunc: func() time.Time {
					utc, _ := time.LoadLocation("")
					return time.Now().In(utc)
				},
				Logger: dbQueryLogger,
			},
		)

		if logConfig != nil && logConfig.LoggingLevelIsDebug() {
			gormDB = gormDB.Debug()
		}

		var store *Pool
		v, ok := s.dataStores.Load(name)
		if ok {
			store = v.(*Pool)
		} else {
			store = newStore(ctx, s)
		}

		store.addConnection(gormDB, readOnly)

		s.dataStores.Store(name, store)

		if dbConfig != nil {
			store.shouldDoMigrations = dbConfig.DoDatabaseMigrate()
			store.SetPoolConfig(
				dbConfig.GetMaxOpenConnections(),
				dbConfig.GetMaxIdleConnections(),
				dbConfig.GetMaxConnectionLifeTimeInSeconds(),
			)
		}

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

// cleanPostgresDSN checks if the input is already a DSN, otherwise converts a PostgreSQL URL to DSN.
func cleanPostgresDSN(pgString string) (string, error) {
	trimmed := strings.TrimSpace(pgString)
	// Heuristic: if it contains '=' and does not start with postgres:// or postgresql://, treat as DSN
	lower := strings.ToLower(trimmed)
	if strings.Contains(trimmed, "=") && !strings.HasPrefix(lower, "postgres://") && !strings.HasPrefix(lower, "postgresql://") {
		return trimmed, nil
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return "", err
	}

	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return "", fmt.Errorf("invalid scheme: %s", u.Scheme)
	}

	user := ""
	password := ""
	if u.User != nil {
		user = u.User.Username()
		password, _ = u.User.Password()
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "5432"
	}
	dbname := strings.TrimPrefix(u.Path, "/")

	dsn := []string{
		"host=" + host,
		"port=" + port,
		"user=" + user,
		"password=" + password,
		"dbname=" + dbname,
	}
	for k, vals := range u.Query() {
		for _, v := range vals {
			dsn = append(dsn, fmt.Sprintf("%s=%s", k, v))
		}
	}
	return strings.Join(dsn, " "), nil
}
