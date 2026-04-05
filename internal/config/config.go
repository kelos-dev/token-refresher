package config

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

const serviceAccountNamespacePath = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

const (
	ProviderCodex = "codex"
)

type Config struct {
	Provider        string
	SecretNamespace string
	SecretName      string
	SecretKey       string
	Codex           CodexConfig
}

type CodexConfig struct {
	Command       string
	Prompt        string
	Timeout       time.Duration
	RefreshPolicy string
	RefreshWindow time.Duration
	AuthSubdir    string
	AuthFileName  string
}

func Load(args []string) (Config, error) {
	cfg := Config{
		Provider:        ProviderCodex,
		SecretNamespace: readNamespaceFile(),
		SecretKey:       "auth.json",
		Codex: CodexConfig{
			Command:       "codex",
			Prompt:        "Reply with the single word ok.",
			Timeout:       10 * time.Minute,
			RefreshPolicy: "always",
			RefreshWindow: 72 * time.Hour,
			AuthSubdir:    ".codex",
			AuthFileName:  "auth.json",
		},
	}

	fs := flag.NewFlagSet("token-refresher", flag.ContinueOnError)
	fs.StringVar(&cfg.Provider, "agent-provider", cfg.Provider, "agent provider to refresh")
	fs.StringVar(&cfg.SecretName, "secret-name", "", "name of the Kubernetes secret containing auth.json")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	cfg.Provider = strings.ToLower(strings.TrimSpace(cfg.Provider))
	cfg.SecretName = strings.TrimSpace(cfg.SecretName)

	if cfg.SecretNamespace == "" {
		return Config{}, fmt.Errorf("failed to determine in-cluster namespace from service account")
	}

	if cfg.SecretName == "" {
		return Config{}, fmt.Errorf("--secret-name must be set")
	}

	switch cfg.Provider {
	case ProviderCodex:
	default:
		return Config{}, fmt.Errorf("unsupported AGENT_PROVIDER %q", cfg.Provider)
	}

	switch cfg.Codex.RefreshPolicy {
	case "always", "threshold":
	default:
		return Config{}, fmt.Errorf("unsupported CODEX_REFRESH_POLICY %q", cfg.Codex.RefreshPolicy)
	}

	if cfg.Codex.Timeout <= 0 {
		return Config{}, fmt.Errorf("CODEX_REFRESH_TIMEOUT must be greater than zero")
	}

	if cfg.Codex.RefreshWindow < 0 {
		return Config{}, fmt.Errorf("CODEX_REFRESH_WINDOW cannot be negative")
	}

	return cfg, nil
}

func readNamespaceFile() string {
	data, err := os.ReadFile(serviceAccountNamespacePath)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(data))
}
