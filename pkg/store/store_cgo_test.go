//go:build cgo
// +build cgo

package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker-faas/docker-faas/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore(t *testing.T) {
	// Create temporary database
	dbPath := "test.db"
	defer os.Remove(dbPath)

	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	t.Run("CreateFunction", func(t *testing.T) {
		envVars, err := EncodeMap(map[string]string{"KEY": "value"})
		require.NoError(t, err)
		labels, err := EncodeMap(map[string]string{"label": "test"})
		require.NoError(t, err)
		annotations, err := EncodeMap(map[string]string{"topic": "events"})
		require.NoError(t, err)

		metadata := &types.FunctionMetadata{
			Name:        "test-func",
			Image:       "test/image:latest",
			EnvProcess:  "python handler.py",
			EnvVars:     envVars,
			Labels:      labels,
			Annotations: annotations,
			Network:     "docker-faas-net",
			Replicas:    2,
		}

		err = store.CreateFunction(metadata)
		assert.NoError(t, err)
		assert.Greater(t, metadata.ID, int64(0))
	})

	t.Run("GetFunction", func(t *testing.T) {
		fn, err := store.GetFunction("test-func")
		require.NoError(t, err)
		assert.Equal(t, "test-func", fn.Name)
		assert.Equal(t, "test/image:latest", fn.Image)
		assert.Equal(t, 2, fn.Replicas)
		assert.Equal(t, map[string]string{"topic": "events"}, DecodeMap(fn.Annotations))
	})

	t.Run("AnnotationsRoundTripThroughUpdate", func(t *testing.T) {
		fn, err := store.GetFunction("test-func")
		require.NoError(t, err)

		annotations, err := EncodeMap(map[string]string{"topic": "orders", "com.example/tier": "gold"})
		require.NoError(t, err)
		fn.Annotations = annotations
		require.NoError(t, store.UpdateFunction(fn))

		updated, err := store.GetFunction("test-func")
		require.NoError(t, err)
		assert.Equal(t,
			map[string]string{"topic": "orders", "com.example/tier": "gold"},
			DecodeMap(updated.Annotations))
	})

	t.Run("ListFunctions", func(t *testing.T) {
		functions, err := store.ListFunctions()
		require.NoError(t, err)
		assert.Len(t, functions, 1)
		assert.Equal(t, "test-func", functions[0].Name)
	})

	t.Run("UpdateFunction", func(t *testing.T) {
		metadata := &types.FunctionMetadata{
			Name:     "test-func",
			Image:    "test/image:v2",
			Network:  "docker-faas-net",
			Replicas: 3,
		}

		err := store.UpdateFunction(metadata)
		assert.NoError(t, err)

		fn, err := store.GetFunction("test-func")
		require.NoError(t, err)
		assert.Equal(t, "test/image:v2", fn.Image)
		assert.Equal(t, 3, fn.Replicas)
	})

	t.Run("UpdateReplicas", func(t *testing.T) {
		err := store.UpdateReplicas("test-func", 5)
		assert.NoError(t, err)

		fn, err := store.GetFunction("test-func")
		require.NoError(t, err)
		assert.Equal(t, 5, fn.Replicas)
	})

	t.Run("DeleteFunction", func(t *testing.T) {
		err := store.DeleteFunction("test-func")
		assert.NoError(t, err)

		_, err = store.GetFunction("test-func")
		assert.Error(t, err)
	})
}

// TestMigration_OldSchemaGainsAnnotations proves that a database created
// before migration 3 (no annotations column) opens cleanly, keeps its legacy
// rows readable, and gains a working annotations column.
func TestMigration_OldSchemaGainsAnnotations(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "old-schema.db")

	// Build a v2-era database by hand: migrations 1 and 2 applied and
	// recorded, no annotations column, one legacy row.
	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	for _, migration := range Migrations[:2] {
		_, err = db.Exec(migration.Up)
		require.NoError(t, err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		description TEXT NOT NULL,
		applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`)
	require.NoError(t, err)
	for _, migration := range Migrations[:2] {
		_, err = db.Exec(`INSERT INTO schema_migrations (version, description) VALUES (?, ?)`,
			migration.Version, migration.Description)
		require.NoError(t, err)
	}
	_, err = db.Exec(`INSERT INTO functions (name, image, network, replicas, created_at, updated_at)
		VALUES ('legacy', 'legacy/image:1', 'net', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	// Opening the store must apply migration 3 without disturbing old data.
	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	legacy, err := store.GetFunction("legacy")
	require.NoError(t, err)
	assert.Equal(t, "legacy", legacy.Name)
	assert.Equal(t, "", legacy.Annotations)

	// New rows round-trip annotations after the migration.
	annotations, err := EncodeMap(map[string]string{"topic": "events"})
	require.NoError(t, err)
	require.NoError(t, store.CreateFunction(&types.FunctionMetadata{
		Name:        "annotated",
		Image:       "test/image:2",
		Network:     "net",
		Replicas:    1,
		Annotations: annotations,
	}))
	created, err := store.GetFunction("annotated")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"topic": "events"}, DecodeMap(created.Annotations))

	// Reopening an already-migrated database must be a no-op (no double ALTER).
	require.NoError(t, store.Close())
	reopened, err := NewStore(dbPath)
	require.NoError(t, err)
	defer reopened.Close()
	again, err := reopened.GetFunction("annotated")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"topic": "events"}, DecodeMap(again.Annotations))
}
