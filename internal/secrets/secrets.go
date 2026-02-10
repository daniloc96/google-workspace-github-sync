package secrets

import (
	"fmt"
	"os"

	"github.com/aws/aws-secretsmanager-caching-go/v2/secretcache"
)

// Manager wraps the Secrets Manager cache client.
type Manager struct {
	cache *secretcache.Cache
}

// NewManager creates a new Secrets Manager cache.
func NewManager() (*Manager, error) {
	cache, err := secretcache.New()
	if err != nil {
		return nil, err
	}
	return &Manager{cache: cache}, nil
}

// GetSecretString retrieves a secret value from Secrets Manager.
func (m *Manager) GetSecretString(secretName string) (string, error) {
	if secretName == "" {
		return "", fmt.Errorf("secret name is required")
	}
	return m.cache.GetSecretString(secretName)
}

// LoadSecretFromFile reads a secret value from a local file.
func LoadSecretFromFile(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("file path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ResolveSecretValue loads a secret from Secrets Manager or falls back to local file.
func ResolveSecretValue(secretName string, filePath string) (string, error) {
	if secretName != "" {
		manager, err := NewManager()
		if err != nil {
			return "", err
		}
		return manager.GetSecretString(secretName)
	}
	return LoadSecretFromFile(filePath)
}
