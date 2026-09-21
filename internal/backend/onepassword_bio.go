package backend

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/newtosh/timeshare/internal/config"
	"github.com/newtosh/timeshare/internal/onepassword"
)

// defaultBiometricTTL is the fallback cache duration when a resolve
// request carries no TTL of its own. Configured project TTLs win in the
// daemon; this is not a ceiling.
const defaultBiometricTTL = 10 * time.Minute

// readField is a seam over onepassword.ReadField for unit tests.
var readField = onepassword.ReadField

type OnePasswordBiometric struct{}

func NewOnePasswordBiometric() *OnePasswordBiometric {
	return &OnePasswordBiometric{}
}

func (b *OnePasswordBiometric) Resolve(ctx context.Context, cfg config.Config, secretName string) (string, time.Duration, error) {
	_ = ctx
	// Argv-based op item get (not an op:// URI) so titles with "/" or
	// spaces resolve correctly — same approach as SSH fingerprint lookup.
	value, err := readField(cfg.Vault, secretName, secretField(cfg, secretName))
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "not found") || strings.Contains(msg, "isn't an item") {
			return "", 0, fmt.Errorf("%w: %s", ErrItemNotFound, msg)
		}
		return "", 0, fmt.Errorf("%w: %s", ErrAuthFailed, msg)
	}
	return value, defaultBiometricTTL, nil
}

// secretField returns the per-item field override from cfg, or the Login
// default ("password") when unset — Secure Notes need notesPlain, etc.
func secretField(cfg config.Config, secretName string) string {
	if field := cfg.FieldFor(secretName); field != "" {
		return field
	}
	return onepassword.DefaultSecretField
}
