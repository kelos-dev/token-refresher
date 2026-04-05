package providerapi

import (
	"context"
	"time"
)

type Provider interface {
	Name() string
	Refresh(ctx context.Context, authJSON []byte) (Result, error)
}

type Result struct {
	UpdatedAuth      []byte
	Changed          bool
	AttemptedRefresh bool
	ExpiresAt        *time.Time
	Reason           string
}
