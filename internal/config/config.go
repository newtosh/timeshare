package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Mode string

const (
	ModeServiceAccount Mode = "service-account"
	ModeBiometric      Mode = "biometric"
)

type Config struct {
	Vault   string        `yaml:"vault"`
	Mode    Mode          `yaml:"mode"`
	TTL     time.Duration `yaml:"ttl"`
	Items   []string      `yaml:"items"`
	SSHKeys []string      `yaml:"ssh_keys,omitempty"`
}

type rawConfig struct {
	Vault   string   `yaml:"vault"`
	Mode    string   `yaml:"mode"`
	TTL     string   `yaml:"ttl"`
	Items   []string `yaml:"items"`
	SSHKeys []string `yaml:"ssh_keys,omitempty"`
}

// Load reads and validates a .timeshare.yml file at path.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is the caller's own repo's .timeshare.yml, not attacker-controlled input
	if err != nil {
		return Config{}, fmt.Errorf("reading config: %w", err)
	}

	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Config{}, fmt.Errorf("parsing config: %w", err)
	}

	if raw.Vault == "" {
		return Config{}, fmt.Errorf("config missing required field: vault")
	}

	mode := Mode(raw.Mode)
	if mode != ModeServiceAccount && mode != ModeBiometric {
		return Config{}, fmt.Errorf("config field mode must be %q or %q, got %q", ModeServiceAccount, ModeBiometric, raw.Mode)
	}

	ttl, err := time.ParseDuration(raw.TTL)
	if err != nil {
		return Config{}, fmt.Errorf("config field ttl invalid: %w", err)
	}

	if len(raw.Items) == 0 && len(raw.SSHKeys) == 0 {
		return Config{}, fmt.Errorf("config must list at least one entry under items or ssh_keys")
	}

	return Config{Vault: raw.Vault, Mode: mode, TTL: ttl, Items: raw.Items, SSHKeys: raw.SSHKeys}, nil
}

// Write marshals cfg to path as YAML, symmetric with Load. Uses the real
// YAML marshaler (not hand-built string concatenation) so vault/item names
// containing YAML-significant characters — a colon, a leading "-" or "#",
// literal whitespace, anything — round-trip correctly.
func Write(path string, cfg Config) error {
	raw := rawConfig{
		Vault:   cfg.Vault,
		Mode:    string(cfg.Mode),
		TTL:     cfg.TTL.String(),
		Items:   cfg.Items,
		SSHKeys: cfg.SSHKeys,
	}
	data, err := yaml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	//nolint:gosec // .timeshare.yml is meant to be committed to git and world-readable; it never contains a credential
	return os.WriteFile(path, data, 0o644)
}

// Allows reports whether secretName is in this config's item allow-list.
func (c Config) Allows(secretName string) bool {
	for _, item := range c.Items {
		if item == secretName {
			return true
		}
	}
	return false
}
