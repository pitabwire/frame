package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/pitabwire/frame/tests/definitions"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pitabwire/util"
	"github.com/testcontainers/testcontainers-go"
	tcPostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/pitabwire/frame"
)

const (
	// Database configuration.

	// PostgresqlDBImage is the PostgreSQL Image.
	PostgresqlDBImage = "postgres:17"

	// DBUser is the default username for the PostgreSQL test database.
	DBUser = "frame"
	// DBPassword is the default password for the PostgreSQL test database.
	DBPassword = "fr@m3"
	// DBName is the default database name for the PostgreSQL test database.
	DBName = "frame_test"

	// OccurrenceValue is the number of occurrences to wait for in the log pattern.
	OccurrenceValue = 2
	// TimeoutInSeconds is the timeout duration for container startup in seconds.
	TimeoutInSeconds = 60
)

type PostgreSQLDependancy struct {
	image    string
	username string
	password string
	dbname   string
	conn     frame.DataSource

	postgresContainer *tcPostgres.PostgresContainer
}

func NewPGDep() definitions.Dependancy {
	return NewPGDepWithCred(PostgresqlDBImage, DBUser, DBPassword, DBName)
}

func NewPGDepWithCred(pgImage, pgUserName, pgPassword, pgDBName string) definitions.Dependancy {
	return &PostgreSQLDependancy{
		image:    pgImage,
		username: pgUserName,
		password: pgPassword,
		dbname:   pgDBName,
	}
}

// Setup creates a PostgreSQL testcontainer and sets the container.
func (pgd *PostgreSQLDependancy) Setup(ctx context.Context) error {
	log := util.Log(ctx)

	log.Info("Setting up PostgreSQL container...")

	pgContainer, err := tcPostgres.Run(ctx, PostgresqlDBImage,
		tcPostgres.WithDatabase(DBName),
		tcPostgres.WithUsername(DBUser),
		tcPostgres.WithPassword(DBPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(OccurrenceValue).
				WithStartupTimeout(TimeoutInSeconds*time.Second)),
	)
	if err != nil {
		return fmt.Errorf("failed to start postgres container: %w", err)
	}

	conn, err := pgd.postgresContainer.ConnectionString(ctx)
	if err != nil {
		return fmt.Errorf("failed to get connection string for postgres container: %w", err)
	}

	pgd.conn = frame.DataSource(conn)

	pgd.postgresContainer = pgContainer

	return nil
}

func (pgd *PostgreSQLDependancy) GetDS() frame.DataSource {
	return pgd.conn
}

// GetPrefixedDS Prepare a postgres connection string for testing.
// Returns the connection string to use and a close function which must be called when the test finishes.
// Calling this function twice will return the same database, which will have data from previous tests
// unless close() is called.
func (pgd *PostgreSQLDependancy) GetPrefixedDS(
	ctx context.Context,
	randomisedPrefix string,
) (frame.DataSource, func(context.Context), error) {
	parsedPostgresURI, err := pgd.conn.ToURI()
	if err != nil {
		return "", func(_ context.Context) {}, err
	}

	newDatabaseName, err := generateNewDBName(randomisedPrefix)
	if err != nil {
		return "", func(_ context.Context) {}, err
	}

	connectionURI, err := ensureDatabaseExists(ctx, parsedPostgresURI, newDatabaseName)
	if err != nil {
		return "", func(_ context.Context) {}, err
	}

	postgresURIStr := connectionURI.String()
	return frame.DataSource(postgresURIStr), func(_ context.Context) {
		_ = clearDatabase(ctx, postgresURIStr)
	}, nil
}

func (pgd *PostgreSQLDependancy) Cleanup(ctx context.Context) {
	if pgd.postgresContainer != nil {
		if err := pgd.postgresContainer.Terminate(ctx); err != nil {
			log := util.Log(ctx)
			log.WithError(err).Error("Failed to terminate postgres container")
		}
	}
}

// ensureDatabaseExists checks if a specific database exists and creates it if it does not.
func ensureDatabaseExists(ctx context.Context, postgresURI *url.URL, newDBName string) (*url.URL, error) {
	connectionString := postgresURI.String()
	cfg, err := pgxpool.ParseConfig(connectionString)
	if err != nil {
		return postgresURI, err
	}
	cfg.MaxConns = 20 // Increase pool size for concurrency
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return postgresURI, err
	}

	defer pool.Close()

	if err = pool.Ping(ctx); err != nil {
		return postgresURI, err
	}

	// Check if database exists before trying to create it
	_, err = pool.Exec(ctx, fmt.Sprintf(`CREATE DATABASE %s;`, newDBName))
	if err != nil {
		var pgErr *pgconn.PgError
		ok := errors.As(err, &pgErr)
		if !ok ||
			(pgErr.Code != "42P04" && pgErr.Code != "23505" && (pgErr.Code != "XX000" || !strings.Contains(pgErr.Message, "tuple concurrently updated"))) {
			return postgresURI, err
		}
	}

	dbUserName := postgresURI.User.Username()
	_, err = pool.Exec(ctx, fmt.Sprintf(`GRANT ALL PRIVILEGES ON DATABASE %s TO %s;`, newDBName, dbUserName))
	if err != nil {
		var pgErr *pgconn.PgError
		ok := errors.As(err, &pgErr)
		if !ok || pgErr.Code != "XX000" || !strings.Contains(pgErr.Message, "tuple concurrently updated") {
			return postgresURI, err
		}
	}

	postgresURI.Path = newDBName
	return postgresURI, nil
}

func clearDatabase(ctx context.Context, connectionString string) error {
	pool, err := pgxpool.New(ctx, connectionString)
	if err != nil {
		return err
	}
	defer pool.Close()

	_, err = pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`)
	if err != nil {
		return err
	}
	return nil
}

func generateNewDBName(randomnesPrefix string) (string, error) {
	// we cannot use 'matrix_test' here else 2x concurrently running packages will try to use the same db.
	// instead, hash the current working directory, snaffle the first 16 bytes and append that to matrix_test
	// and use that as the unique db name. We do this because packages are per-directory hence by hashing the
	// working (test) directory we ensure we get a consistent hash and don't hash against concurrent packages.
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256([]byte(wd))
	databaseName := fmt.Sprintf("notifications_test_%s_%s", randomnesPrefix, hex.EncodeToString(hash[:16]))
	return databaseName, nil
}
