package backend

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/newtosh/timeshare/internal/config"
	"github.com/newtosh/timeshare/internal/version"

	onepassword "github.com/1password/onepassword-sdk-go"
)

// defaultServiceAccountTTL is the fallback cache duration when a resolve
// request carries no TTL of its own. Configured project TTLs win in the
// daemon; this is not a ceiling.
const defaultServiceAccountTTL = 15 * time.Minute

type OnePasswordServiceAccount struct {
	token string
}

func NewOnePasswordServiceAccount(token string) *OnePasswordServiceAccount {
	return &OnePasswordServiceAccount{token: token}
}

func (b *OnePasswordServiceAccount) Resolve(ctx context.Context, cfg config.Config, secretName string) (string, time.Duration, error) {
	client, err := onepassword.NewClient(ctx,
		onepassword.WithServiceAccountToken(b.token),
		onepassword.WithIntegrationInfo("timeshare", strings.TrimPrefix(version.Version, "v")),
	)
	if err != nil {
		return "", 0, fmt.Errorf("%w: %w", ErrAuthFailed, err)
	}

	reference := fmt.Sprintf("op://%s/%s/password", cfg.Vault, secretName)
	value, err := client.Secrets().Resolve(ctx, reference)
	if err != nil {
		return "", 0, fmt.Errorf("%w: %w", ErrItemNotFound, err)
	}

	return value, defaultServiceAccountTTL, nil
}
