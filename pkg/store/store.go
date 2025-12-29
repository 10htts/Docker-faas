package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/docker-faas/docker-faas/pkg/metrics"
	"github.com/docker-faas/docker-faas/pkg/types"
	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
)

// Store manages function metadata persistence
type Store struct {
	db *sql.DB
}

// NewStore creates a new store instance
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	store := &Store{db: db}
	if err := store.initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	return store, nil
}

// initialize creates the database schema using migrations
func (s *Store) initialize() error {
	// Use migration manager to ensure schema is up to date
	logger := logrus.New()
	logger.SetOutput(os.Stdout)
	logger.SetLevel(logrus.InfoLevel)

	migrationManager := NewMigrationManager(s.db, logger)
	if err := migrationManager.ApplyMigrations(); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	return nil
}

// CreateFunction stores a new function
func (s *Store) CreateFunction(metadata *types.FunctionMetadata) (err error) {
	start := time.Now()
	defer func() {
		metrics.RecordDBOperation("create_function", time.Since(start).Seconds(), err)
	}()

	query := `
	INSERT INTO functions (name, image, env_process, env_vars, labels, secrets, network, replicas, limits, requests, read_only, debug, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := s.db.Exec(query,
		metadata.Name,
		metadata.Image,
		metadata.EnvProcess,
		metadata.EnvVars,
		metadata.Labels,
		metadata.Secrets,
		metadata.Network,
		metadata.Replicas,
		metadata.Limits,
		metadata.Requests,
		metadata.ReadOnly,
		metadata.Debug,
		time.Now(),
		time.Now(),
	)

	if err != nil {
		return fmt.Errorf("failed to create function: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}

	metadata.ID = id
	return nil
}

// GetFunction retrieves a function by name
func (s *Store) GetFunction(name string) (metadata *types.FunctionMetadata, err error) {
	start := time.Now()
	defer func() {
		metrics.RecordDBOperation("get_function", time.Since(start).Seconds(), err)
	}()

	query := `
	SELECT id, name, image, env_process, env_vars, labels, secrets, network, replicas, limits, requests, read_only, debug, created_at, updated_at
	FROM functions WHERE name = ?
	`

	var result types.FunctionMetadata
	err = s.db.QueryRow(query, name).Scan(
		&result.ID,
		&result.Name,
		&result.Image,
		&result.EnvProcess,
		&result.EnvVars,
		&result.Labels,
		&result.Secrets,
		&result.Network,
		&result.Replicas,
		&result.Limits,
		&result.Requests,
		&result.ReadOnly,
		&result.Debug,
		&result.CreatedAt,
		&result.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		err = fmt.Errorf("function not found: %s", name)
		return nil, err
	}
	if err != nil {
		err = fmt.Errorf("failed to get function: %w", err)
		return nil, err
	}

	metadata = &result
	return metadata, nil
}

// ListFunctions retrieves all functions
func (s *Store) ListFunctions() (functions []*types.FunctionMetadata, err error) {
	start := time.Now()
	defer func() {
		metrics.RecordDBOperation("list_functions", time.Since(start).Seconds(), err)
	}()

	query := `
	SELECT id, name, image, env_process, env_vars, labels, secrets, network, replicas, limits, requests, read_only, debug, created_at, updated_at
	FROM functions ORDER BY created_at DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list functions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var metadata types.FunctionMetadata
		err := rows.Scan(
			&metadata.ID,
			&metadata.Name,
			&metadata.Image,
			&metadata.EnvProcess,
			&metadata.EnvVars,
			&metadata.Labels,
			&metadata.Secrets,
			&metadata.Network,
			&metadata.Replicas,
			&metadata.Limits,
			&metadata.Requests,
			&metadata.ReadOnly,
			&metadata.Debug,
			&metadata.CreatedAt,
			&metadata.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan function: %w", err)
		}
		functions = append(functions, &metadata)
	}

	return functions, nil
}

// UpdateFunction updates an existing function
func (s *Store) UpdateFunction(metadata *types.FunctionMetadata) (err error) {
	start := time.Now()
	defer func() {
		metrics.RecordDBOperation("update_function", time.Since(start).Seconds(), err)
	}()

	query := `
	UPDATE functions
	SET image = ?, env_process = ?, env_vars = ?, labels = ?, secrets = ?, network = ?, replicas = ?, limits = ?, requests = ?, read_only = ?, debug = ?, updated_at = ?
	WHERE name = ?
	`

	result, err := s.db.Exec(query,
		metadata.Image,
		metadata.EnvProcess,
		metadata.EnvVars,
		metadata.Labels,
		metadata.Secrets,
		metadata.Network,
		metadata.Replicas,
		metadata.Limits,
		metadata.Requests,
		metadata.ReadOnly,
		metadata.Debug,
		time.Now(),
		metadata.Name,
	)

	if err != nil {
		return fmt.Errorf("failed to update function: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("function not found: %s", metadata.Name)
	}

	return nil
}

// DeleteFunction removes a function
func (s *Store) DeleteFunction(name string) (err error) {
	start := time.Now()
	defer func() {
		metrics.RecordDBOperation("delete_function", time.Since(start).Seconds(), err)
	}()

	query := `DELETE FROM functions WHERE name = ?`

	result, err := s.db.Exec(query, name)
	if err != nil {
		return fmt.Errorf("failed to delete function: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("function not found: %s", name)
	}

	return nil
}

// UpdateReplicas updates the replica count for a function
func (s *Store) UpdateReplicas(name string, replicas int) (err error) {
	start := time.Now()
	defer func() {
		metrics.RecordDBOperation("update_replicas", time.Since(start).Seconds(), err)
	}()

	query := `UPDATE functions SET replicas = ?, updated_at = ? WHERE name = ?`

	result, err := s.db.Exec(query, replicas, time.Now(), name)
	if err != nil {
		return fmt.Errorf("failed to update replicas: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("function not found: %s", name)
	}

	return nil
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.db.Close()
}

// HealthCheck validates database connectivity.
func (s *Store) HealthCheck(ctx context.Context) (err error) {
	start := time.Now()
	defer func() {
		metrics.RecordDBOperation("health_check", time.Since(start).Seconds(), err)
	}()
	err = s.db.PingContext(ctx)
	return err
}

// Helper functions for JSON encoding/decoding

// EncodeMap encodes a map to JSON string
func EncodeMap(m map[string]string) (string, error) {
	if m == nil {
		return "", nil
	}
	data, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// DecodeMap decodes a JSON string to map
func DecodeMap(s string) map[string]string {
	if s == "" {
		return make(map[string]string)
	}
	var m map[string]string
	json.Unmarshal([]byte(s), &m)
	return m
}

// EncodeSlice encodes a slice to JSON string
func EncodeSlice(s []string) (string, error) {
	if s == nil {
		return "", nil
	}
	data, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// DecodeSlice decodes a JSON string to slice
func DecodeSlice(s string) []string {
	if s == "" {
		return []string{}
	}
	var slice []string
	json.Unmarshal([]byte(s), &slice)
	return slice
}
