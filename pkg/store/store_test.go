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
