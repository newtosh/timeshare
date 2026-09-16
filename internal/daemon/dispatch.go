package daemon

import (
	"context"
	"fmt"
	"time"

	"timeshare/internal/config"
)

// ModeDispatcher selects a Backend implementation per-request based on
// req.Mode, since one daemon serves projects configured for either mode
// (spec: Goals — both modes required).
type ModeDispatcher struct {
	ServiceAccount func(token string) interface {
		Resolve(ctx context.Context, cfg config.Config, secretName string) (string, time.Duration, error)
	}
	Biometric interface {
		Resolve(ctx context.Context, cfg config.Config, secretName string) (string, time.Duration, error)
	}
	// TokenForVault resolves a vault name to its stored service-account
	// token (OS keychain lookup — implemented alongside the init token-
	// storage follow-up noted in Task 10).
	TokenForVault func(vault string) (string, error)
}

func (d *ModeDispatcher) Resolve(ctx context.Context, cfg config.Config, secretName string) (string, time.Duration, error) {
	switch cfg.Mode {
	case config.ModeBiometric:
		return d.Biometric.Resolve(ctx, cfg, secretName)
	case config.ModeServiceAccount:
		token, err := d.TokenForVault(cfg.Vault)
		if err != nil {
			return "", 0, fmt.Errorf("looking up service account token: %w", err)
		}
		return d.ServiceAccount(token).Resolve(ctx, cfg, secretName)
	default:
		return "", 0, fmt.Errorf("unknown mode %q", cfg.Mode)
	}
}
