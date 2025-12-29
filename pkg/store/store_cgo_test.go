//go:build cgo
// +build cgo

package store

import (
	"os"
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

		metadata := &types.FunctionMetadata{
			Name:       "test-func",
			Image:      "test/image:latest",
			EnvProcess: "python handler.py",
			EnvVars:    envVars,
			Labels:     labels,
			Network:    "docker-faas-net",
			Replicas:   2,
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
