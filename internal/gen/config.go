package gen

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the pyffi-gen.yaml configuration.
type Config struct {
	Module        string            `yaml:"module"`
	Package       string            `yaml:"package"`
	Out           string            `yaml:"out"`
	Include       []string          `yaml:"include"`
	Exclude       []string          `yaml:"exclude"`
	TypeOverrides map[string]string `yaml:"type_overrides"`
	DocStyle      string            `yaml:"doc_style"`
	Test          bool              `yaml:"test"`
	Python        string            `yaml:"python"`
	Dependencies  []string          `yaml:"dependencies"`
}

// LoadConfig reads a YAML config file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("pyffi-gen: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("pyffi-gen: invalid config: %w", err)
	}
	return &cfg, nil
}

// Defaults fills in unset fields with sensible defaults.
func (c *Config) Defaults() {
	if c.Package == "" && c.Module != "" {
		c.Package = strings.ReplaceAll(c.Module, ".", "_")
	}
	if c.Out == "" && c.Package != "" {
		c.Out = "./gen/" + c.Package
	}
	if c.DocStyle == "" {
		c.DocStyle = "godoc"
	}
}

// Validate checks for required fields.
func (c *Config) Validate() error {
	if c.Module == "" {
		return fmt.Errorf("pyffi-gen: --module is required")
	}
	return nil
}
