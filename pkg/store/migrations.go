package store

import (
	"database/sql"
	"fmt"

	"github.com/sirupsen/logrus"
)

// Migration represents a database migration
type Migration struct {
	Version     int
	Description string
	Up          string
	Down        string
}

// Migrations is the ordered list of all database migrations
var Migrations = []Migration{
	{
		Version:     1,
		Description: "Initial schema",
		Up: `
			CREATE TABLE IF NOT EXISTS functions (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name TEXT UNIQUE NOT NULL,
				image TEXT NOT NULL,
				env_process TEXT,
				env_vars TEXT,
				labels TEXT,
				secrets TEXT,
				network TEXT NOT NULL,
				replicas INTEGER NOT NULL DEFAULT 1,
				limits TEXT,
				requests TEXT,
				read_only BOOLEAN NOT NULL DEFAULT 0,
				created_at TIMESTAMP NOT NULL,
				updated_at TIMESTAMP NOT NULL
			);
			CREATE INDEX IF NOT EXISTS idx_functions_name ON functions(name);
			CREATE INDEX IF NOT EXISTS idx_functions_created_at ON functions(created_at);
		`,
		Down: `DROP TABLE IF EXISTS functions;`,
	},
	{
		Version:     2,
		Description: "Add debug column for v2.0",
		Up: `
			ALTER TABLE functions ADD COLUMN debug BOOLEAN NOT NULL DEFAULT 0;
		`,
		Down: `
			-- SQLite doesn't support DROP COLUMN directly
			-- Create new table without debug column, copy data, rename
			CREATE TABLE functions_backup AS SELECT
				id, name, image, env_process, env_vars, labels, secrets,
				network, replicas, limits, requests, read_only, created_at, updated_at
			FROM functions;
			DROP TABLE functions;
			ALTER TABLE functions_backup RENAME TO functions;
			CREATE INDEX IF NOT EXISTS idx_functions_name ON functions(name);
			CREATE INDEX IF NOT EXISTS idx_functions_created_at ON functions(created_at);
		`,
	},
}

// MigrationManager handles database migrations
type MigrationManager struct {
	db     *sql.DB
	logger *logrus.Logger
}

// NewMigrationManager creates a new migration manager
func NewMigrationManager(db *sql.DB, logger *logrus.Logger) *MigrationManager {
	return &MigrationManager{
		db:     db,
		logger: logger,
	}
}

// ensureMigrationsTable creates the migrations tracking table if it doesn't exist
func (m *MigrationManager) ensureMigrationsTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			description TEXT NOT NULL,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`
	_, err := m.db.Exec(query)
	return err
}

// GetCurrentVersion returns the current schema version
func (m *MigrationManager) GetCurrentVersion() (int, error) {
	if err := m.ensureMigrationsTable(); err != nil {
		return 0, err
	}

	var version int
	err := m.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version)
	if err != nil {
		return 0, err
	}
	return version, nil
}

// ApplyMigrations applies all pending migrations
func (m *MigrationManager) ApplyMigrations() error {
	if err := m.ensureMigrationsTable(); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	currentVersion, err := m.GetCurrentVersion()
	if err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	m.logger.Infof("Current schema version: %d", currentVersion)

	// Apply pending migrations in order
	for _, migration := range Migrations {
		if migration.Version <= currentVersion {
			continue // Already applied
		}

		m.logger.Infof("Applying migration %d: %s", migration.Version, migration.Description)

		// Begin transaction
		tx, err := m.db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction for migration %d: %w", migration.Version, err)
		}

		// Execute migration
		if _, err := tx.Exec(migration.Up); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to apply migration %d: %w", migration.Version, err)
		}

		// Record migration
		if _, err := tx.Exec(
			"INSERT INTO schema_migrations (version, description) VALUES (?, ?)",
			migration.Version,
			migration.Description,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record migration %d: %w", migration.Version, err)
		}

		// Commit transaction
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %d: %w", migration.Version, err)
		}

		m.logger.Infof("Successfully applied migration %d", migration.Version)
	}

	finalVersion, _ := m.GetCurrentVersion()
	m.logger.Infof("Database schema is up to date (version %d)", finalVersion)
	return nil
}

// Rollback rolls back the last N migrations
func (m *MigrationManager) Rollback(steps int) error {
	currentVersion, err := m.GetCurrentVersion()
	if err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	if currentVersion == 0 {
		return fmt.Errorf("no migrations to rollback")
	}

	for i := 0; i < steps && currentVersion > 0; i++ {
		// Find migration for current version
		var migration *Migration
		for _, m := range Migrations {
			if m.Version == currentVersion {
				migration = &m
				break
			}
		}

		if migration == nil {
			return fmt.Errorf("migration %d not found", currentVersion)
		}

		m.logger.Infof("Rolling back migration %d: %s", migration.Version, migration.Description)

		// Begin transaction
		tx, err := m.db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}

		// Execute rollback
		if _, err := tx.Exec(migration.Down); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to rollback migration %d: %w", migration.Version, err)
		}

		// Remove migration record
		if _, err := tx.Exec("DELETE FROM schema_migrations WHERE version = ?", migration.Version); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to remove migration record %d: %w", migration.Version, err)
		}

		// Commit transaction
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit rollback %d: %w", migration.Version, err)
		}

		m.logger.Infof("Successfully rolled back migration %d", migration.Version)
		currentVersion--
	}

	return nil
}

// GetAppliedMigrations returns a list of applied migrations
func (m *MigrationManager) GetAppliedMigrations() ([]int, error) {
	if err := m.ensureMigrationsTable(); err != nil {
		return nil, err
	}

	rows, err := m.db.Query("SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	versions := []int{}
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}

	return versions, nil
}
