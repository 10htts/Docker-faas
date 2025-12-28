package builder

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/docker-faas/docker-faas/pkg/types"
)

// Manifest defines docker-faas.yaml fields for source builds.
type Manifest struct {
	Name                   string                   `yaml:"name"`
	Runtime                string                   `yaml:"runtime"`
	Command                string                   `yaml:"command"`
	Dependencies           []string                 `yaml:"dependencies"`
	Env                    map[string]string        `yaml:"env"`
	Labels                 map[string]string        `yaml:"labels"`
	Secrets                []string                 `yaml:"secrets"`
	Limits                 *types.FunctionLimits    `yaml:"limits"`
	Requests               *types.FunctionResources `yaml:"requests"`
	ReadOnlyRootFilesystem bool                     `yaml:"readOnlyRootFilesystem"`
	Debug                  bool                     `yaml:"debug"`
	Network                string                   `yaml:"network"`
	Build                  []string                 `yaml:"build"`
}

// LoadManifest reads and parses docker-faas.yaml.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseManifest(data)
}

// ParseManifest parses YAML bytes into a manifest.
func ParseManifest(data []byte) (*Manifest, error) {
	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse docker-faas.yaml: %w", err)
	}
	return &manifest, nil
}
