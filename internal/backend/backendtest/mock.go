// Package backendtest provides a Backend implementation for tests in other
// packages (daemon, client, cli) that need to exercise backend-calling code
// without a real 1Password dependency.
package backendtest

import (
	"context"
	"time"

	"timeshare/internal/config"
)

type Mock struct {
	ValueFor map[string]string
	TTL      time.Duration
	Err      error
	Calls    []string
}

func (m *Mock) Resolve(_ context.Context, _ config.Config, secretName string) (string, time.Duration, error) {
	m.Calls = append(m.Calls, secretName)
	if m.Err != nil {
		return "", 0, m.Err
	}
	return m.ValueFor[secretName], m.TTL, nil
}
