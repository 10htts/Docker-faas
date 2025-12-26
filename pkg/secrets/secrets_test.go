package secrets

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecretManager(t *testing.T) {
	// Create temporary directory for tests
	tmpDir := filepath.Join(os.TempDir(), "docker-faas-secrets-test")
	defer os.RemoveAll(tmpDir)

	logger := logrus.New()
	logger.SetOutput(os.Stdout)

	sm, err := NewSecretManager(tmpDir, logger)
	require.NoError(t, err)
	require.NotNil(t, sm)

	t.Run("CreateSecret", func(t *testing.T) {
		err := sm.CreateSecret("test-secret", "test-value")
		assert.NoError(t, err)
		assert.True(t, sm.SecretExists("test-secret"))
	})

	t.Run("CreateDuplicateSecret", func(t *testing.T) {
		err := sm.CreateSecret("test-secret", "another-value")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("GetSecret", func(t *testing.T) {
		value, err := sm.GetSecret("test-secret")
		assert.NoError(t, err)
		assert.Equal(t, "test-value", value)
	})

	t.Run("GetNonExistentSecret", func(t *testing.T) {
		_, err := sm.GetSecret("nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("ListSecrets", func(t *testing.T) {
		secrets, err := sm.ListSecrets()
		assert.NoError(t, err)
		assert.Contains(t, secrets, "test-secret")
	})

	t.Run("UpdateSecret", func(t *testing.T) {
		err := sm.UpdateSecret("test-secret", "updated-value")
		assert.NoError(t, err)

		value, err := sm.GetSecret("test-secret")
		assert.NoError(t, err)
		assert.Equal(t, "updated-value", value)
	})

	t.Run("UpdateNonExistentSecret", func(t *testing.T) {
		err := sm.UpdateSecret("nonexistent", "value")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("ValidateSecrets", func(t *testing.T) {
		err := sm.CreateSecret("secret1", "value1")
		require.NoError(t, err)
		err = sm.CreateSecret("secret2", "value2")
		require.NoError(t, err)

		// Valid secrets
		err = sm.ValidateSecrets([]string{"test-secret", "secret1", "secret2"})
		assert.NoError(t, err)

		// Missing secrets
		err = sm.ValidateSecrets([]string{"test-secret", "missing"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing")
	})

	t.Run("DeleteSecret", func(t *testing.T) {
		err := sm.DeleteSecret("test-secret")
		assert.NoError(t, err)
		assert.False(t, sm.SecretExists("test-secret"))
	})

	t.Run("DeleteNonExistentSecret", func(t *testing.T) {
		err := sm.DeleteSecret("nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("Base64EncodedSecret", func(t *testing.T) {
		// Base64 encoded "hello world"
		err := sm.CreateSecret("base64-secret", "aGVsbG8gd29ybGQ=")
		assert.NoError(t, err)

		value, err := sm.GetSecret("base64-secret")
		assert.NoError(t, err)
		assert.Equal(t, "hello world", value)
	})

	t.Run("GetSecretPath", func(t *testing.T) {
		err := sm.CreateSecret("path-test", "value")
		require.NoError(t, err)

		path := sm.GetSecretPath("path-test")
		assert.Equal(t, filepath.Join(tmpDir, "path-test"), path)
	})

	t.Run("GetBasePath", func(t *testing.T) {
		basePath := sm.GetBasePath()
		assert.Equal(t, tmpDir, basePath)
	})
}

func TestSecretManagerConcurrency(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "docker-faas-secrets-concurrent")
	defer os.RemoveAll(tmpDir)

	logger := logrus.New()
	logger.SetOutput(os.Stdout)

	sm, err := NewSecretManager(tmpDir, logger)
	require.NoError(t, err)

	// Test concurrent reads and writes
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func(index int) {
			secretName := fmt.Sprintf("concurrent-secret-%d", index)
			err := sm.CreateSecret(secretName, fmt.Sprintf("value-%d", index))
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	secrets, err := sm.ListSecrets()
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(secrets), 10)
}
