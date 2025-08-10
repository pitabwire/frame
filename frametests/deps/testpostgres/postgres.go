package testpostgres

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pitabwire/util"
	"github.com/testcontainers/testcontainers-go"
	tcPostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/pitabwire/frame"
	"github.com/pitabwire/frame/frametests/definition"
)

const (
	// Database configuration.
	postgreSQLMaxIdentifiersCharLength = 60

	// PostgresqlDBImage is the PostgreSQL Image.
	PostgresqlDBImage = "postgres:latest"

	// DBUser is the default username for the PostgreSQL test database.
	DBUser = "frame"
	// DBPassword is the default password for the PostgreSQL test database.
	DBPassword = "fr@m3"
	// DBName is the default database name for the PostgreSQL test database.
	DBName = "frame_test"
	// DBPort is the default port for the PostgreSQL test database.
	DBPort = "5432"

	// OccurrenceValue is the number of occurrences to wait for in the log pattern.
	OccurrenceValue = 2
	// TimeoutInSeconds is the timeout duration for container startup in seconds.
	TimeoutInSeconds = 60
)

type postgreSQLDependancy struct {
	opts         definition.ContainerOpts
	dbname       string
	conn         frame.DataSource
	internalConn frame.DataSource

	container *tcPostgres.PostgresContainer
}

func New() definition.TestResource {
	return NewWithOpts(DBName)
}

func NewWithOpts(dbName string, containerOpts ...definition.ContainerOption) definition.TestResource {
	opts := definition.ContainerOpts{
		ImageName:      PostgresqlDBImage,
		UserName:       DBUser,
		Password:       DBPassword,
		Port:           DBPort,
		UseHostMode:    false,
		DisableLogging: true,
	}
	opts.Setup(containerOpts...)

	return &postgreSQLDependancy{
		opts:   opts,
		dbname: dbName,
	}
}

func (d *postgreSQLDependancy) Name() string {
	return d.opts.ImageName
}

func (d *postgreSQLDependancy) Container() testcontainers.Container {
	return d.container
}

// Setup creates a PostgreSQL testcontainer and sets the container.
func (d *postgreSQLDependancy) Setup(ctx context.Context, ntwk *testcontainers.DockerNetwork) error {
	containerCustomize := []testcontainers.ContainerCustomizer{

		tcPostgres.WithDatabase(d.dbname),
		tcPostgres.WithUsername(d.opts.UserName),
		tcPostgres.WithPassword(d.opts.Password),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(OccurrenceValue).
				WithStartupTimeout(TimeoutInSeconds * time.Second)),
	}

	if !d.opts.DisableLogging {
		containerCustomize = append(
			containerCustomize,
			testcontainers.WithLogConsumerConfig(definition.LogConfig(ctx, d.opts.LoggingTimeout)),
		)
	}

	if d.opts.UseHostMode {
		containerCustomize = append(containerCustomize, testcontainers.WithHostConfigModifier(
			func(hostConfig *container.HostConfig) {
				hostConfig.NetworkMode = definition.HostNetworkingMode
			}))
	} else {
		containerCustomize = append(containerCustomize,
			network.WithNetwork([]string{ntwk.Name}, ntwk),
			network.WithNetworkName([]string{"postgres", "db-postgres"}, ntwk.Name))
	}

	pgContainer, err := tcPostgres.Run(ctx, d.opts.ImageName, containerCustomize...)
	if err != nil {
		return fmt.Errorf("failed to start postgres container: %w", err)
	}

	conn, err := pgContainer.ConnectionString(ctx)
	if err != nil {
		return fmt.Errorf("failed to get connection string for postgres container: %w", err)
	}

	d.conn = frame.DataSource(conn)

	internalIP, err := pgContainer.ContainerIP(ctx)
	if err != nil {
		return fmt.Errorf("failed to get internal host ip for postgres container: %w", err)
	}

	connStr := fmt.Sprintf(
		"postgres://%s:%s@%s/%s",
		d.opts.UserName,
		d.opts.Password,
		net.JoinHostPort(internalIP, d.opts.Port),
		d.dbname,
	)

	d.internalConn = frame.DataSource(connStr)

	d.container = pgContainer

	return nil
}

func (d *postgreSQLDependancy) GetDS() frame.DataSource {
	return d.conn
}

func (d *postgreSQLDependancy) GetInternalDS() frame.DataSource {
	return d.internalConn
}

// GetRandomisedDS Prepare a postgres connection string for testing.
// Returns the connection string to use and a close function which must be called when the test finishes.
// Calling this function twice will return the same database, which will have data from previous tests
// unless close() is called.
func (d *postgreSQLDependancy) GetRandomisedDS(
	ctx context.Context,
	randomisedPrefix string,
) (frame.DataSource, func(context.Context), error) {
	connectionURI, err := d.GetDS().ToURI()
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

func (d *postgreSQLDependancy) Cleanup(ctx context.Context) {
	if d.container != nil {
		if err := d.container.Terminate(ctx); err != nil {
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

// suffixedDatabaseName generates a valid PostgreSQL database name from the given URL path and random prefix.
// It ensures the name complies with PostgreSQL identifier rules and length constraints.
func suffixedDatabaseName(currentURI *url.URL, randomnesPrefix string) string {
	// Extract the path, remove slashes, and ensure we have some content
	pathPart := strings.ReplaceAll(currentURI.Path, "/", "")
	if pathPart == "" {
		pathPart = "db"
	}

	// PostgreSQL identifiers are limited to 63 bytes
	// Allow space for the random prefix and underscore
	maxPathLength := postgreSQLMaxIdentifiersCharLength - len(randomnesPrefix)
	if len(pathPart) > maxPathLength {
		pathPart = pathPart[:maxPathLength]
	}

	// Generate the database name, ensuring it starts with a letter
	// PostgreSQL identifiers must start with a letter or underscore
	result := fmt.Sprintf("%s_%s", pathPart, randomnesPrefix)

	// Replace any characters that aren't valid in PostgreSQL identifiers
	// Valid: letters, digits, and underscores
	re := regexp.MustCompile(`[^a-zA-Z0-9_]`)
	result = re.ReplaceAllString(result, "_")

	return strings.ToLower(result) // PostgreSQL folds unquoted identifiers to lowercase
}
