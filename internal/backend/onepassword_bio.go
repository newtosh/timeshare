package backend

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"timeshare/internal/config"
)

// defaultBiometricTTL matches 1Password's own documented CLI session
// inactivity window, so timeshare's cache doesn't outlive what 1Password
// itself considers a valid unlocked session.
const defaultBiometricTTL = 10 * time.Minute

type OnePasswordBiometric struct{}

func NewOnePasswordBiometric() *OnePasswordBiometric {
	return &OnePasswordBiometric{}
}

func (b *OnePasswordBiometric) Resolve(ctx context.Context, cfg config.Config, secretName string) (string, time.Duration, error) {
	reference := fmt.Sprintf("op://%s/%s/password", cfg.Vault, secretName)

	cmd := exec.CommandContext(ctx, "op", "read", reference) //nolint:gosec // fixed binary name "op"; this IS the app's job (shell out to the 1Password CLI)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if strings.Contains(msg, "not found") {
			return "", 0, fmt.Errorf("%w: %s", ErrItemNotFound, msg)
		}
		return "", 0, fmt.Errorf("%w: %s", ErrAuthFailed, msg)
	}

	return strings.TrimRight(stdout.String(), "\n"), defaultBiometricTTL, nil
}
