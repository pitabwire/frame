package framedata

import (
	"context"
	"crypto/md5"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// migrator implements the Migrator interface
type migrator struct {
	datastore DatastoreManager
	config    Config
	logger    Logger
	
	migrationsFS fs.FS
}

// NewMigrator creates a new migrator instance
func NewMigrator(datastore DatastoreManager, config Config, logger Logger, migrationsFS fs.FS) Migrator {
	return &migrator{
		datastore:    datastore,
		config:       config,
		logger:       logger,
		migrationsFS: migrationsFS,
	}
}

// Migrate applies all pending migrations
func (m *migrator) Migrate(ctx context.Context) error {
	if err := m.ensureMigrationsTable(ctx); err != nil {
		return fmt.Errorf("failed to ensure migrations table: %w", err)
	}

	pendingMigrations, err := m.GetPendingMigrations(ctx)
	if err != nil {
		return fmt.Errorf("failed to get pending migrations: %w", err)
	}

	if len(pendingMigrations) == 0 {
		if m.logger != nil {
			m.logger.Info("No pending migrations to apply")
		}
		return nil
	}

	if m.logger != nil {
		m.logger.WithField("count", len(pendingMigrations)).Info("Applying pending migrations")
	}

	for _, migration := range pendingMigrations {
		if err := m.applyMigration(ctx, migration); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", migration.ID, err)
		}
	}

	if m.logger != nil {
		m.logger.WithField("count", len(pendingMigrations)).Info("Successfully applied all pending migrations")
	}

	return nil
}

// MigrateUp applies a specific number of migrations
func (m *migrator) MigrateUp(ctx context.Context, steps int) error {
	if steps <= 0 {
		return fmt.Errorf("steps must be positive, got %d", steps)
	}

	if err := m.ensureMigrationsTable(ctx); err != nil {
		return fmt.Errorf("failed to ensure migrations table: %w", err)
	}

	pendingMigrations, err := m.GetPendingMigrations(ctx)
	if err != nil {
		return fmt.Errorf("failed to get pending migrations: %w", err)
	}

	if len(pendingMigrations) == 0 {
		if m.logger != nil {
			m.logger.Info("No pending migrations to apply")
		}
		return nil
	}

	// Limit to requested steps
	if steps < len(pendingMigrations) {
		pendingMigrations = pendingMigrations[:steps]
	}

	if m.logger != nil {
		m.logger.WithField("count", len(pendingMigrations)).Info("Applying migrations")
	}

	for _, migration := range pendingMigrations {
		if err := m.applyMigration(ctx, migration); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", migration.ID, err)
		}
	}

	return nil
}

// MigrateDown reverts a specific number of migrations
func (m *migrator) MigrateDown(ctx context.Context, steps int) error {
	if steps <= 0 {
		return fmt.Errorf("steps must be positive, got %d", steps)
	}

	appliedMigrations, err := m.GetAppliedMigrations(ctx)
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	if len(appliedMigrations) == 0 {
		if m.logger != nil {
			m.logger.Info("No applied migrations to revert")
		}
		return nil
	}

	// Sort by version descending for rollback
	sort.Slice(appliedMigrations, func(i, j int) bool {
		return appliedMigrations[i].Version > appliedMigrations[j].Version
	})

	// Limit to requested steps
	if steps < len(appliedMigrations) {
		appliedMigrations = appliedMigrations[:steps]
	}

	if m.logger != nil {
		m.logger.WithField("count", len(appliedMigrations)).Info("Reverting migrations")
	}

	for _, migration := range appliedMigrations {
		if err := m.revertMigration(ctx, migration); err != nil {
			return fmt.Errorf("failed to revert migration %s: %w", migration.ID, err)
		}
	}

	return nil
}

// GetAppliedMigrations returns all applied migrations
func (m *migrator) GetAppliedMigrations(ctx context.Context) ([]Migration, error) {
	if err := m.ensureMigrationsTable(ctx); err != nil {
		return nil, err
	}

	db, err := m.datastore.GetReadOnlyConnection(ctx)
	if err != nil {
		return nil, err
	}

	tableName := m.getMigrationsTableName()
	query := fmt.Sprintf("SELECT id, description, version, applied_at, checksum FROM %s ORDER BY version ASC", tableName)

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query applied migrations: %w", err)
	}
	defer rows.Close()

	var migrations []Migration
	for rows.Next() {
		var migration Migration
		if err := rows.Scan(&migration.ID, &migration.Description, &migration.Version, &migration.AppliedAt, &migration.Checksum); err != nil {
			return nil, fmt.Errorf("failed to scan migration row: %w", err)
		}
		migrations = append(migrations, migration)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating migration rows: %w", err)
	}

	return migrations, nil
}

// GetPendingMigrations returns all pending migrations
func (m *migrator) GetPendingMigrations(ctx context.Context) ([]Migration, error) {
	allMigrations, err := m.loadMigrationsFromFS()
	if err != nil {
		return nil, err
	}

	appliedMigrations, err := m.GetAppliedMigrations(ctx)
	if err != nil {
		return nil, err
	}

	// Create a map of applied migration IDs for quick lookup
	appliedMap := make(map[string]bool)
	for _, applied := range appliedMigrations {
		appliedMap[applied.ID] = true
	}

	// Filter out applied migrations
	var pendingMigrations []Migration
	for _, migration := range allMigrations {
		if !appliedMap[migration.ID] {
			pendingMigrations = append(pendingMigrations, migration)
		}
	}

	// Sort by version
	sort.Slice(pendingMigrations, func(i, j int) bool {
		return pendingMigrations[i].Version < pendingMigrations[j].Version
	})

	return pendingMigrations, nil
}

// ValidateMigrations validates all migrations
func (m *migrator) ValidateMigrations(ctx context.Context) error {
	allMigrations, err := m.loadMigrationsFromFS()
	if err != nil {
		return err
	}

	appliedMigrations, err := m.GetAppliedMigrations(ctx)
	if err != nil {
		return err
	}

	// Create a map of applied migrations for validation
	appliedMap := make(map[string]Migration)
	for _, applied := range appliedMigrations {
		appliedMap[applied.ID] = applied
	}

	// Validate applied migrations against filesystem
	for _, fsMigration := range allMigrations {
		if applied, exists := appliedMap[fsMigration.ID]; exists {
			// Validate checksum
			if applied.Checksum != fsMigration.Checksum {
				return fmt.Errorf("migration %s has been modified after application (checksum mismatch)", fsMigration.ID)
			}
		}
	}

	if m.logger != nil {
		m.logger.WithField("totalMigrations", len(allMigrations)).WithField("appliedMigrations", len(appliedMigrations)).Info("Migration validation completed successfully")
	}

	return nil
}

// ensureMigrationsTable creates the migrations table if it doesn't exist
func (m *migrator) ensureMigrationsTable(ctx context.Context) error {
	db, err := m.datastore.GetConnection(ctx)
	if err != nil {
		return err
	}

	tableName := m.getMigrationsTableName()
	createTableSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id VARCHAR(255) PRIMARY KEY,
			description TEXT NOT NULL,
			version BIGINT NOT NULL,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			checksum VARCHAR(32) NOT NULL,
			UNIQUE(version)
		)
	`, tableName)

	if _, err := db.ExecContext(ctx, createTableSQL); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	return nil
}

// applyMigration applies a single migration
func (m *migrator) applyMigration(ctx context.Context, migration Migration) error {
	if m.logger != nil {
		m.logger.WithField("migrationID", migration.ID).WithField("version", migration.Version).Info("Applying migration")
	}

	// Read migration content
	content, err := m.readMigrationContent(migration.ID)
	if err != nil {
		return err
	}

	// Begin transaction
	tx, err := m.datastore.BeginTransaction(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Execute migration SQL
	if _, err := tx.ExecContext(ctx, content); err != nil {
		return fmt.Errorf("failed to execute migration SQL: %w", err)
	}

	// Record migration as applied
	tableName := m.getMigrationsTableName()
	insertSQL := fmt.Sprintf("INSERT INTO %s (id, description, version, checksum) VALUES ($1, $2, $3, $4)", tableName)
	if _, err := tx.ExecContext(ctx, insertSQL, migration.ID, migration.Description, migration.Version, migration.Checksum); err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration transaction: %w", err)
	}

	if m.logger != nil {
		m.logger.WithField("migrationID", migration.ID).Info("Migration applied successfully")
	}

	return nil
}

// revertMigration reverts a single migration
func (m *migrator) revertMigration(ctx context.Context, migration Migration) error {
	if m.logger != nil {
		m.logger.WithField("migrationID", migration.ID).WithField("version", migration.Version).Info("Reverting migration")
	}

	// Read rollback content
	rollbackContent, err := m.readRollbackContent(migration.ID)
	if err != nil {
		return err
	}

	// Begin transaction
	tx, err := m.datastore.BeginTransaction(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Execute rollback SQL
	if _, err := tx.ExecContext(ctx, rollbackContent); err != nil {
		return fmt.Errorf("failed to execute rollback SQL: %w", err)
	}

	// Remove migration record
	tableName := m.getMigrationsTableName()
	deleteSQL := fmt.Sprintf("DELETE FROM %s WHERE id = $1", tableName)
	if _, err := tx.ExecContext(ctx, deleteSQL, migration.ID); err != nil {
		return fmt.Errorf("failed to remove migration record: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit rollback transaction: %w", err)
	}

	if m.logger != nil {
		m.logger.WithField("migrationID", migration.ID).Info("Migration reverted successfully")
	}

	return nil
}

// loadMigrationsFromFS loads all migrations from the filesystem
func (m *migrator) loadMigrationsFromFS() ([]Migration, error) {
	if m.migrationsFS == nil {
		return nil, fmt.Errorf("migrations filesystem not configured")
	}

	var migrations []Migration

	err := fs.WalkDir(m.migrationsFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || !strings.HasSuffix(path, ".up.sql") {
			return nil
		}

		migration, err := m.parseMigrationFile(path)
		if err != nil {
			return fmt.Errorf("failed to parse migration file %s: %w", path, err)
		}

		migrations = append(migrations, migration)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk migrations directory: %w", err)
	}

	return migrations, nil
}

// parseMigrationFile parses a migration file and returns a Migration
func (m *migrator) parseMigrationFile(path string) (Migration, error) {
	// Extract version and description from filename
	// Expected format: {version}_{description}.up.sql
	filename := filepath.Base(path)
	filename = strings.TrimSuffix(filename, ".up.sql")
	
	parts := strings.SplitN(filename, "_", 2)
	if len(parts) != 2 {
		return Migration{}, fmt.Errorf("invalid migration filename format: %s", filename)
	}

	version, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return Migration{}, fmt.Errorf("invalid version in filename %s: %w", filename, err)
	}

	description := strings.ReplaceAll(parts[1], "_", " ")

	// Read file content for checksum
	content, err := fs.ReadFile(m.migrationsFS, path)
	if err != nil {
		return Migration{}, fmt.Errorf("failed to read migration file %s: %w", path, err)
	}

	// Calculate checksum
	checksum := fmt.Sprintf("%x", md5.Sum(content))

	return Migration{
		ID:          filename,
		Description: description,
		Version:     version,
		Checksum:    checksum,
	}, nil
}

// readMigrationContent reads the content of a migration file
func (m *migrator) readMigrationContent(migrationID string) (string, error) {
	path := migrationID + ".up.sql"
	content, err := fs.ReadFile(m.migrationsFS, path)
	if err != nil {
		return "", fmt.Errorf("failed to read migration file %s: %w", path, err)
	}
	return string(content), nil
}

// readRollbackContent reads the content of a rollback file
func (m *migrator) readRollbackContent(migrationID string) (string, error) {
	path := migrationID + ".down.sql"
	content, err := fs.ReadFile(m.migrationsFS, path)
	if err != nil {
		return "", fmt.Errorf("failed to read rollback file %s: %w", path, err)
	}
	return string(content), nil
}

// getMigrationsTableName returns the migrations table name
func (m *migrator) getMigrationsTableName() string {
	if m.config != nil {
		if tableName := m.config.GetDatabaseMigrationsTable(); tableName != "" {
			return tableName
		}
	}
	return "schema_migrations" // Default table name
}
