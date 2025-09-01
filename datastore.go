package frame

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/XSAM/otelsql"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const defaultStoreName = "__default__"
const defaultPingRetryWaitMilliseconds = 250

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
		mu:                 sync.RWMutex{},
	}

	srv.AddCleanupMethod(store.cleanup)

	return store
}

// addNewConnection safely adds a DB connection to the pool.
func (s *Pool) addNewConnection(db *gorm.DB, readOnly bool) {
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
	wait := defaultPingRetryWaitMilliseconds * time.Millisecond
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

// DB Returns a random item from the slice, or an error if the slice is empty.
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

	return db.Session(&gorm.Session{NewDB: true, AllowGlobalUpdate: true}).WithContext(ctx).Scopes(tenantPartition(ctx))
}

// selectOne uses atomic round-robin for high concurrency.
func (s *Pool) selectOne(pool []*gorm.DB, idx *uint64) *gorm.DB {
	if len(pool) == 0 {
		return nil
	}
	pos := atomic.AddUint64(idx, 1)
	return pool[int(pos-1)%len(pool)] //nolint:gosec // G115: index is result of (val % len), always < len and fits in int.
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

		return db.Where("tenant_id = ? AND partition_id = ?", authClaim.GetTenantID(), authClaim.GetPartitionID())
	}
}

// ErrorIsNoRows validate if supplied error is because of record missing in DB.
func ErrorIsNoRows(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, sql.ErrNoRows)
}

func (s *Service) DBPool(name ...string) *Pool {
	dbPoolName := defaultStoreName
	if len(name) > 0 {
		dbPoolName = name[0]
	}

	v, ok := s.dataStores.Load(dbPoolName)
	if !ok {
		return nil
	}

	pVal, ok := v.(*Pool)
	if !ok {
		return nil // Or log an error, depending on desired behavior
	}
	return pVal
}

// DB returns the database connection for the service.
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

// WithDatastoreConnection Option method to store a connection that will be utilized when connecting to the database.
func WithDatastoreConnection(postgresqlConnection string, readOnly bool) Option {
	return WithDatastoreConnectionWithName(defaultStoreName, postgresqlConnection, readOnly)
}
func WithDatastoreConnectionWithName(name string, postgresqlConnection string, readOnly bool) Option {
	return func(ctx context.Context, s *Service) {
		cleanedPostgresqlDSN, err := cleanPostgresDSN(postgresqlConnection)
		if err != nil {
			s.Log(ctx).
				WithError(err).
				WithField("dsn", postgresqlConnection).
				Fatal("could not get a clean postgresql dsn")
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
			s.Log(ctx).WithError(err).WithField("dsn", postgresqlConnection).Error("could not connect to pg now")
			return
		}

		gormDB, err := gorm.Open(
			postgres.New(postgres.Config{
				Conn:                 conn,
				PreferSimpleProtocol: preferSimpleProtocol,
			}),
			&gorm.Config{
				Logger:                 datbaseLogger(ctx, s),
				SkipDefaultTransaction: skipDefaultTransaction,
			},
		)

		if err != nil {
			s.Log(ctx).WithError(err).WithField("dsn", postgresqlConnection).Error("could not connect to gorm now")
			return
		}

		store := s.DBPool(name)
		if store == nil { // Pool not found or was of an incompatible type
			store = newStore(ctx, s)
			s.dataStores.Store(name, store) // Register the new store
		}
		store.addNewConnection(gormDB, readOnly)

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

func WithDatastore() Option {
	return func(ctx context.Context, s *Service) {
		config, ok := s.Config().(ConfigurationDatabase)
		if !ok {
			s.Log(ctx).Warn("configuration object not of type : ConfigurationDatabase")
			return
		}

		for _, primaryDBURL := range config.GetDatabasePrimaryHostURL() {
			primaryDatabase := WithDatastoreConnection(primaryDBURL, false)
			primaryDatabase(ctx, s)
		}

		for _, replicaDBURL := range config.GetDatabaseReplicaHostURL() {
			replicaDatabase := WithDatastoreConnection(replicaDBURL, true)
			replicaDatabase(ctx, s)
		}
	}
}

// cleanPostgresDSN checks if the input is already a DSN, otherwise converts a PostgreSQL URL to DSN.
func cleanPostgresDSN(pgString string) (string, error) {
	trimmed := strings.TrimSpace(pgString)
	// Heuristic: if it contains '=' and does not start with postgres:// or postgresql://, treat as DSN
	lower := strings.ToLower(trimmed)
	if strings.Contains(trimmed, "=") && !strings.HasPrefix(lower, "postgres://") &&
		!strings.HasPrefix(lower, "postgresql://") {
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
