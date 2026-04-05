package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/kelos-dev/token-refresher/internal/config"
	"github.com/kelos-dev/token-refresher/internal/kube"
	"github.com/kelos-dev/token-refresher/internal/providers"
)

func main() {
	log.SetFlags(0)

	if err := run(); err != nil {
		log.Fatalf("token-refresher: %v", err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load(os.Args[1:])
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	client, err := kube.NewInClusterClient()
	if err != nil {
		return fmt.Errorf("create kubernetes client: %w", err)
	}

	provider, err := providers.New(cfg)
	if err != nil {
		return fmt.Errorf("load provider: %w", err)
	}

	secret, authJSON, err := client.GetSecretKey(ctx, cfg.SecretNamespace, cfg.SecretName, cfg.SecretKey)
	if err != nil {
		return fmt.Errorf("get secret %s/%s[%s]: %w", cfg.SecretNamespace, cfg.SecretName, cfg.SecretKey, err)
	}

	log.Printf(
		"provider=%s loaded secret=%s/%s key=%s bytes=%d",
		provider.Name(),
		cfg.SecretNamespace,
		cfg.SecretName,
		cfg.SecretKey,
		len(authJSON),
	)

	result, err := provider.Refresh(ctx, authJSON)
	if err != nil {
		return fmt.Errorf("refresh auth data: %w", err)
	}

	if result.ExpiresAt != nil {
		log.Printf(
			"provider=%s attempted_refresh=%t changed=%t expires_at=%s reason=%s",
			provider.Name(),
			result.AttemptedRefresh,
			result.Changed,
			result.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z"),
			result.Reason,
		)
	} else {
		log.Printf(
			"provider=%s attempted_refresh=%t changed=%t reason=%s",
			provider.Name(),
			result.AttemptedRefresh,
			result.Changed,
			result.Reason,
		)
	}

	if !result.Changed {
		log.Printf("provider=%s no secret update required", provider.Name())
		return nil
	}

	if err := client.UpdateSecretKey(ctx, &secret, cfg.SecretKey, result.UpdatedAuth); err != nil {
		return fmt.Errorf("update secret %s/%s[%s]: %w", cfg.SecretNamespace, cfg.SecretName, cfg.SecretKey, err)
	}

	log.Printf(
		"provider=%s updated secret=%s/%s key=%s bytes=%d",
		provider.Name(),
		cfg.SecretNamespace,
		cfg.SecretName,
		cfg.SecretKey,
		len(result.UpdatedAuth),
	)

	return nil
}
