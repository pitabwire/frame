package testpostgres

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pitabwire/util"
	"github.com/testcontainers/testcontainers-go"
	tcPostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/tests/testdef"
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

type postgreSQLDependancy struct {
	image    string
	username string
	password string
	dbname   string
	conn     frame.DataSource

	postgresContainer *tcPostgres.PostgresContainer
}

func NewPGDep() testdef.TestResource {
	return NewPGDepWithCred(PostgresqlDBImage, DBUser, DBPassword, DBName)
}

func NewPGDepWithCred(pgImage, pgUserName, pgPassword, pgDBName string) testdef.TestResource {
	return &postgreSQLDependancy{
		image:    pgImage,
		username: pgUserName,
		password: pgPassword,
		dbname:   pgDBName,
	}
}

// Setup creates a PostgreSQL testcontainer and sets the container.
func (pgd *postgreSQLDependancy) Setup(ctx context.Context, _ *testcontainers.DockerNetwork) error {
	log := util.Log(ctx)

	log.Info("Setting up PostgreSQL container...")

	pgContainer, err := tcPostgres.Run(ctx, pgd.image,
		tcPostgres.WithDatabase(pgd.dbname),
		tcPostgres.WithUsername(pgd.username),
		tcPostgres.WithPassword(pgd.password),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(OccurrenceValue).
				WithStartupTimeout(TimeoutInSeconds*time.Second)),
	)
	if err != nil {
		return fmt.Errorf("failed to start postgres container: %w", err)
	}

	conn, err := pgContainer.ConnectionString(ctx)
	if err != nil {
		return fmt.Errorf("failed to get connection string for postgres container: %w", err)
	}

	pgd.conn = frame.DataSource(conn)

	pgd.postgresContainer = pgContainer

	return nil
}

func (pgd *postgreSQLDependancy) GetDS() frame.DataSource {
	return pgd.conn
}

// GetRandomisedDS Prepare a postgres connection string for testing.
// Returns the connection string to use and a close function which must be called when the test finishes.
// Calling this function twice will return the same database, which will have data from previous tests
// unless close() is called.
func (pgd *postgreSQLDependancy) GetRandomisedDS(
	ctx context.Context,
	randomisedPrefix string,
) (frame.DataSource, func(context.Context), error) {
	connectionURI, err := pgd.conn.ToURI()
	if err != nil {
		return "", func(_ context.Context) {}, err
	}

	newDatabaseName := suffixedDatabaseName(connectionURI, randomisedPrefix)

	connectionURI, err = ensureDatabaseExists(ctx, connectionURI, newDatabaseName)
	if err != nil {
		return "", func(_ context.Context) {}, err
	}

	suffixedPgURIStr := connectionURI.String()
	return frame.DataSource(suffixedPgURIStr), func(_ context.Context) {
		_ = clearDatabase(ctx, suffixedPgURIStr)
	}, nil
}

func (pgd *postgreSQLDependancy) Cleanup(ctx context.Context) {
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

func suffixedDatabaseName(currentUri *url.URL, randomnesPrefix string) string {
	return fmt.Sprintf("%s_%s", currentUri.Path, randomnesPrefix)
}
