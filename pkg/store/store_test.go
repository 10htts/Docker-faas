package store

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/docker-faas/docker-faas/pkg/types"
)

func TestStore(t *testing.T) {
	// Create temporary database
	dbPath := "test.db"
	defer os.Remove(dbPath)

	store, err := NewStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	t.Run("CreateFunction", func(t *testing.T) {
		metadata := &types.FunctionMetadata{
			Name:       "test-func",
			Image:      "test/image:latest",
			EnvProcess: "python handler.py",
			EnvVars:    EncodeMap(map[string]string{"KEY": "value"}),
			Labels:     EncodeMap(map[string]string{"label": "test"}),
			Network:    "docker-faas-net",
			Replicas:   2,
		}

		err := store.CreateFunction(metadata)
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

func TestEncodeDecodeMap(t *testing.T) {
	original := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}

	encoded := EncodeMap(original)
	assert.NotEmpty(t, encoded)

	decoded := DecodeMap(encoded)
	assert.Equal(t, original, decoded)
}

func TestEncodeDecodeSlice(t *testing.T) {
	original := []string{"secret1", "secret2", "secret3"}

	encoded := EncodeSlice(original)
	assert.NotEmpty(t, encoded)

	decoded := DecodeSlice(encoded)
	assert.Equal(t, original, decoded)
}

func TestEncodeDecodeEmptyValues(t *testing.T) {
	// Empty map
	emptyMap := EncodeMap(nil)
	assert.Empty(t, emptyMap)
	decoded := DecodeMap("")
	assert.NotNil(t, decoded)
	assert.Len(t, decoded, 0)

	// Empty slice
	emptySlice := EncodeSlice(nil)
	assert.Empty(t, emptySlice)
	decodedSlice := DecodeSlice("")
	assert.NotNil(t, decodedSlice)
	assert.Len(t, decodedSlice, 0)
}
