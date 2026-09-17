// Package tokenstore resolves and stores 1Password service-account tokens,
// one per vault. Lookup checks a per-vault env var first (the headless
// escape hatch) before falling back to the OS keychain via go-keyring.
package tokenstore

import (
	"fmt"
	"os"
	"strings"

	"github.com/zalando/go-keyring"
)

const keyringService = "timeshare"

// Store saves token in the OS keychain under vault's name.
func Store(vault, token string) error {
	return keyring.Set(keyringService, vault, token)
}

// Delete removes vault's stored token from the OS keychain.
func Delete(vault string) error {
	return keyring.Delete(keyringService, vault)
}

// Lookup resolves vault's service-account token. It checks
// TIMESHARE_SA_TOKEN_<SANITIZED_VAULT> first, then the OS keychain.
func Lookup(vault string) (string, error) {
	if v := os.Getenv(envVarName(vault)); v != "" {
		return v, nil
	}

	token, err := keyring.Get(keyringService, vault)
	if err != nil {
		return "", fmt.Errorf("no token for vault %q (set %s or run `timeshare token store %s`): %w", vault, envVarName(vault), vault, err)
	}
	return token, nil
}

// envVarName sanitizes vault into TIMESHARE_SA_TOKEN_<VAULT>: uppercased,
// non-alphanumeric runs collapsed to a single underscore.
func envVarName(vault string) string {
	var b strings.Builder
	b.WriteString("TIMESHARE_SA_TOKEN_")
	prevUnderscore := false
	for _, r := range strings.ToUpper(vault) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevUnderscore = false
			continue
		}
		if !prevUnderscore {
			b.WriteByte('_')
			prevUnderscore = true
		}
	}
	return b.String()
}
