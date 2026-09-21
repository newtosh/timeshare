package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Item is one allow-listed secret in .timeshare.yml. A bare string entry
// (`- DATABASE_URL`) unmarshals to Name only; a mapping can override which
// 1Password field to read (default is the Login "password" field):
//
//   - name: sbg-engtools.gen
//     field: notesPlain
type Item struct {
	Name  string `yaml:"name"`
	Field string `yaml:"field,omitempty"`
}

// UnmarshalYAML accepts either a scalar item name or a {name, field} mapping.
func (i *Item) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var name string
		if err := value.Decode(&name); err != nil {
			return err
		}
		if name == "" {
			return fmt.Errorf("items entry must not be empty")
		}
		*i = Item{Name: name}
		return nil
	case yaml.MappingNode:
		var raw struct {
			Name  string `yaml:"name"`
			Field string `yaml:"field"`
		}
		if err := value.Decode(&raw); err != nil {
			return err
		}
		if raw.Name == "" {
			return fmt.Errorf("items entry missing required field: name")
		}
		*i = Item{Name: raw.Name, Field: raw.Field}
		return nil
	default:
		return fmt.Errorf("items entry must be a string or a mapping with name/field")
	}
}

// MarshalYAML writes a bare string when Field is unset so init-generated
// configs stay compact and backward-compatible on disk.
func (i Item) MarshalYAML() (any, error) {
	if i.Field == "" {
		return i.Name, nil
	}
	return struct {
		Name  string `yaml:"name"`
		Field string `yaml:"field"`
	}{Name: i.Name, Field: i.Field}, nil
}

// ItemsFromNames builds Items with no field overrides (init / flag path).
func ItemsFromNames(names []string) []Item {
	out := make([]Item, len(names))
	for i, name := range names {
		out[i] = Item{Name: name}
	}
	return out
}

// ItemNames returns just the allow-list names (daemon protocol, env keys).
func (c Config) ItemNames() []string {
	names := make([]string, len(c.Items))
	for i, item := range c.Items {
		names[i] = item.Name
	}
	return names
}

// FieldFor returns the configured 1Password field override for name, or
// "" when the item uses the default Login "password" field.
func (c Config) FieldFor(name string) string {
	for _, item := range c.Items {
		if item.Name == name {
			return item.Field
		}
	}
	return ""
}
