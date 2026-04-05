package providers

import (
	"fmt"

	"github.com/kelos-dev/token-refresher/internal/config"
	"github.com/kelos-dev/token-refresher/internal/providerapi"
	"github.com/kelos-dev/token-refresher/internal/providers/codex"
)

func New(cfg config.Config) (providerapi.Provider, error) {
	switch cfg.Provider {
	case config.ProviderCodex:
		return codex.New(cfg.Codex), nil
	default:
		return nil, fmt.Errorf("unsupported provider %q", cfg.Provider)
	}
}
