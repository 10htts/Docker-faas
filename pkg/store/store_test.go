package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEncodeDecodeMap(t *testing.T) {
	original := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}

	encoded, err := EncodeMap(original)
	assert.NoError(t, err)
	assert.NotEmpty(t, encoded)

	decoded := DecodeMap(encoded)
	assert.Equal(t, original, decoded)
}

func TestEncodeDecodeSlice(t *testing.T) {
	original := []string{"secret1", "secret2", "secret3"}

	encoded, err := EncodeSlice(original)
	assert.NoError(t, err)
	assert.NotEmpty(t, encoded)

	decoded := DecodeSlice(encoded)
	assert.Equal(t, original, decoded)
}

// TestMigrations_RegisterAnnotationsMigration locks the migration registry:
// versions are strictly increasing from 1 and the annotations column migration
// is present, so pre-annotations databases converge on open. (The end-to-end
// old-schema migration proof lives in store_cgo_test.go and runs where cgo /
// sqlite is available.)
func TestMigrations_RegisterAnnotationsMigration(t *testing.T) {
	assert.NotEmpty(t, Migrations)
	for i, migration := range Migrations {
		assert.Equal(t, i+1, migration.Version, "migration versions must be contiguous from 1")
	}
	last := Migrations[len(Migrations)-1]
	assert.Equal(t, 3, last.Version)
	assert.Contains(t, last.Up, "ADD COLUMN annotations")
}

func TestEncodeDecodeEmptyValues(t *testing.T) {
	// Empty map
	emptyMap, err := EncodeMap(nil)
	assert.NoError(t, err)
	assert.Empty(t, emptyMap)
	decoded := DecodeMap("")
	assert.NotNil(t, decoded)
	assert.Len(t, decoded, 0)

	// Empty slice
	emptySlice, err := EncodeSlice(nil)
	assert.NoError(t, err)
	assert.Empty(t, emptySlice)
	decodedSlice := DecodeSlice("")
	assert.NotNil(t, decodedSlice)
	assert.Len(t, decodedSlice, 0)
}
