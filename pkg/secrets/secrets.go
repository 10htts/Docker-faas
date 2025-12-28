package secrets

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

const (
	// DefaultSecretsPath is where secrets are stored on the host
	DefaultSecretsPath = "/var/openfaas/secrets"
	// ContainerSecretsPath is where secrets are mounted in containers
	ContainerSecretsPath = "/var/openfaas/secrets"
)

// SecretManager manages secrets for functions
type SecretManager struct {
	basePath string
	mu       sync.RWMutex
	logger   *logrus.Logger
}

// NewSecretManager creates a new secret manager
func NewSecretManager(basePath string, logger *logrus.Logger) (*SecretManager, error) {
	if basePath == "" {
		basePath = DefaultSecretsPath
	}

	// Create base directory if it doesn't exist
	if err := os.MkdirAll(basePath, 0700); err != nil {
		return nil, fmt.Errorf("failed to create secrets directory: %w", err)
	}

	return &SecretManager{
		basePath: basePath,
		logger:   logger,
	}, nil
}

// CreateSecret creates a new secret
func (sm *SecretManager) CreateSecret(name, value string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	secretPath := filepath.Join(sm.basePath, name)

	// Check if secret already exists
	if _, err := os.Stat(secretPath); err == nil {
		return fmt.Errorf("secret already exists: %s", name)
	}

	// Decode if base64 encoded, otherwise use raw value
	data := []byte(value)
	if decoded, err := base64.StdEncoding.DecodeString(value); err == nil {
		data = decoded
	}

	// Write secret with restricted permissions (owner read-only)
	if err := ioutil.WriteFile(secretPath, data, 0400); err != nil {
		return fmt.Errorf("failed to write secret: %w", err)
	}

	sm.logger.Infof("Created secret: %s", name)
	return nil
}

// UpdateSecret updates an existing secret
func (sm *SecretManager) UpdateSecret(name, value string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	secretPath := filepath.Join(sm.basePath, name)

	// Check if secret exists
	if _, err := os.Stat(secretPath); os.IsNotExist(err) {
		return fmt.Errorf("secret not found: %s", name)
	}

	// Decode if base64 encoded, otherwise use raw value
	data := []byte(value)
	if decoded, err := base64.StdEncoding.DecodeString(value); err == nil {
		data = decoded
	}

	// Remove old secret
	os.Remove(secretPath)

	// Write new secret with restricted permissions
	if err := ioutil.WriteFile(secretPath, data, 0400); err != nil {
		return fmt.Errorf("failed to update secret: %w", err)
	}

	sm.logger.Infof("Updated secret: %s", name)
	return nil
}

// DeleteSecret deletes a secret
func (sm *SecretManager) DeleteSecret(name string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	secretPath := filepath.Join(sm.basePath, name)

	// Check if secret exists
	if _, err := os.Stat(secretPath); os.IsNotExist(err) {
		return fmt.Errorf("secret not found: %s", name)
	}

	if err := os.Remove(secretPath); err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	sm.logger.Infof("Deleted secret: %s", name)
	return nil
}

// GetSecret retrieves a secret value
func (sm *SecretManager) GetSecret(name string) (string, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	secretPath := filepath.Join(sm.basePath, name)

	data, err := ioutil.ReadFile(secretPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("secret not found: %s", name)
		}
		return "", fmt.Errorf("failed to read secret: %w", err)
	}

	return string(data), nil
}

// ListSecrets lists all available secrets
func (sm *SecretManager) ListSecrets() ([]string, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	files, err := ioutil.ReadDir(sm.basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}

	secrets := make([]string, 0, len(files))
	for _, file := range files {
		if !file.IsDir() {
			secrets = append(secrets, file.Name())
		}
	}

	return secrets, nil
}

// SecretExists checks if a secret exists
func (sm *SecretManager) SecretExists(name string) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	secretPath := filepath.Join(sm.basePath, name)
	_, err := os.Stat(secretPath)
	return err == nil
}

// GetSecretPath returns the path to a secret file
func (sm *SecretManager) GetSecretPath(name string) string {
	return filepath.Join(sm.basePath, name)
}

// GetBasePath returns the base secrets path
func (sm *SecretManager) GetBasePath() string {
	return sm.basePath
}

// ValidateSecrets validates that all required secrets exist
func (sm *SecretManager) ValidateSecrets(secretNames []string) error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	missing := []string{}
	for _, name := range secretNames {
		secretPath := filepath.Join(sm.basePath, name)
		if _, err := os.Stat(secretPath); os.IsNotExist(err) {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing secrets: %v", missing)
	}

	return nil
}

// EnsureSecrets creates any missing secrets with empty values.
func (sm *SecretManager) EnsureSecrets(secretNames []string) ([]string, error) {
	created := []string{}
	for _, name := range secretNames {
		if strings.TrimSpace(name) == "" {
			continue
		}
		if sm.SecretExists(name) {
			continue
		}
		if err := sm.CreateSecret(name, ""); err != nil {
			if sm.SecretExists(name) {
				continue
			}
			return created, err
		}
		created = append(created, name)
	}
	return created, nil
}
