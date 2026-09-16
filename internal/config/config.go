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
	ModeBiometric       Mode = "biometric"
)

type Config struct {
	Vault string        `yaml:"vault"`
	Mode  Mode          `yaml:"mode"`
	TTL   time.Duration `yaml:"ttl"`
	Items []string      `yaml:"items"`
}

type rawConfig struct {
	Vault string   `yaml:"vault"`
	Mode  string   `yaml:"mode"`
	TTL   string   `yaml:"ttl"`
	Items []string `yaml:"items"`
}

// Load reads and validates a .timeshare.yml file at path.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
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

	if len(raw.Items) == 0 {
		return Config{}, fmt.Errorf("config must list at least one item under items")
	}

	return Config{Vault: raw.Vault, Mode: mode, TTL: ttl, Items: raw.Items}, nil
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
