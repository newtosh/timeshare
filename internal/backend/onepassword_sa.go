package backend

import (
	"context"
	"fmt"
	"time"

	onepassword "github.com/1password/onepassword-sdk-go"
	"timeshare/internal/config"
)

// defaultServiceAccountTTL is used when the request did not carry an
// explicit TTL. Service-account reads have no interactive approval step to
// avoid, so this TTL exists purely to bound how long a value sits in the
// daemon's in-memory cache before a fresh SDK call re-validates it.
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
		onepassword.WithIntegrationInfo("timeshare", "0.1.0"),
	)
	if err != nil {
		return "", 0, fmt.Errorf("%w: %v", ErrAuthFailed, err)
	}

	reference := fmt.Sprintf("op://%s/%s/password", cfg.Vault, secretName)
	value, err := client.Secrets().Resolve(ctx, reference)
	if err != nil {
		return "", 0, fmt.Errorf("%w: %v", ErrItemNotFound, err)
	}

	return value, defaultServiceAccountTTL, nil
}
