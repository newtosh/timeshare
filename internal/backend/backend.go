package backend

import (
	"context"
	"errors"
	"time"

	"github.com/newtosh/timeshare/internal/config"
)

// ErrAuthFailed indicates the backend could not authenticate (bad or
// revoked token, biometric declined/timeout). The daemon never caches or
// retries this — it is surfaced directly to the caller (spec: Error
// handling).
var ErrAuthFailed = errors.New("backend authentication failed")

// ErrItemNotFound indicates the backend authenticated fine but the named
// secret does not exist in the configured vault.
var ErrItemNotFound = errors.New("secret item not found")

// Backend resolves a named secret to its value and a suggested TTL.
// Implementations: OnePasswordServiceAccount (Mode A), OnePasswordBiometric
// (Mode B). Additional backends (Vault, AWS Secrets Manager) are future
// work behind this same interface (spec: Non-goals).
type Backend interface {
	Resolve(ctx context.Context, cfg config.Config, secretName string) (value string, ttl time.Duration, err error)
}
