package codex

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kelos-dev/token-refresher/internal/config"
	"github.com/kelos-dev/token-refresher/internal/providerapi"
)

type Provider struct {
	cfg    config.CodexConfig
	runner runner
}

type runner func(ctx context.Context, homeDir string, cfg config.CodexConfig) error

type authPayload struct {
	Tokens struct {
		AccessToken string `json:"access_token"`
	} `json:"tokens"`
}

type jwtClaims struct {
	Exp int64 `json:"exp"`
}

func New(cfg config.CodexConfig) providerapi.Provider {
	return &Provider{
		cfg:    cfg,
		runner: runCodexExec,
	}
}

func (p *Provider) Name() string {
	return "codex"
}

func (p *Provider) Refresh(ctx context.Context, authJSON []byte) (providerapi.Result, error) {
	expiresAt, expiryErr := accessTokenExpiry(authJSON)
	if p.cfg.RefreshPolicy == "threshold" && expiryErr == nil && time.Until(*expiresAt) > p.cfg.RefreshWindow {
		return providerapi.Result{
			UpdatedAuth:      authJSON,
			Changed:          false,
			AttemptedRefresh: false,
			ExpiresAt:        expiresAt,
			Reason: fmt.Sprintf(
				"access token remains valid until %s; refresh threshold is %s",
				expiresAt.UTC().Format(time.RFC3339),
				p.cfg.RefreshWindow,
			),
		}, nil
	}

	tempHome, err := os.MkdirTemp("", "token-refresher-codex-*")
	if err != nil {
		return providerapi.Result{}, fmt.Errorf("create temp home: %w", err)
	}
	defer os.RemoveAll(tempHome)

	authPath := filepath.Join(tempHome, p.cfg.AuthSubdir, p.cfg.AuthFileName)
	if err := os.MkdirAll(filepath.Dir(authPath), 0o700); err != nil {
		return providerapi.Result{}, fmt.Errorf("create auth directory: %w", err)
	}

	if err := os.WriteFile(authPath, authJSON, 0o600); err != nil {
		return providerapi.Result{}, fmt.Errorf("write auth file: %w", err)
	}

	if err := p.runner(ctx, tempHome, p.cfg); err != nil {
		return providerapi.Result{}, err
	}

	updatedAuth, err := os.ReadFile(authPath)
	if err != nil {
		return providerapi.Result{}, fmt.Errorf("read refreshed auth file: %w", err)
	}

	updatedExpiry, _ := accessTokenExpiry(updatedAuth)
	changed := !bytes.Equal(authJSON, updatedAuth)
	reason := "codex exec completed without changing auth.json"
	if changed {
		reason = "codex exec completed and updated auth.json"
	}
	if expiryErr != nil {
		reason = reason + "; original access token expiry was unavailable"
	}

	return providerapi.Result{
		UpdatedAuth:      updatedAuth,
		Changed:          changed,
		AttemptedRefresh: true,
		ExpiresAt:        updatedExpiry,
		Reason:           reason,
	}, nil
}

func runCodexExec(ctx context.Context, homeDir string, cfg config.CodexConfig) error {
	cmdCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	cmd := exec.CommandContext(
		cmdCtx,
		cfg.Command,
		"exec",
		"--skip-git-repo-check",
		"--sandbox",
		"read-only",
		"--color",
		"never",
		cfg.Prompt,
	)
	cmd.Env = withEnv(os.Environ(), "HOME="+homeDir)

	output, err := cmd.CombinedOutput()
	if err != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("codex exec timed out after %s", cfg.Timeout)
		}
		return fmt.Errorf("codex exec failed: %w: %s", err, trimOutput(output, 2048))
	}

	return nil
}

func accessTokenExpiry(authJSON []byte) (*time.Time, error) {
	var payload authPayload
	if err := json.Unmarshal(authJSON, &payload); err != nil {
		return nil, fmt.Errorf("decode auth.json: %w", err)
	}

	token := payload.Tokens.AccessToken
	if token == "" {
		return nil, fmt.Errorf("access token missing")
	}

	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("access token is not a JWT")
	}

	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode JWT payload: %w", err)
	}

	var claims jwtClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, fmt.Errorf("decode JWT claims: %w", err)
	}

	if claims.Exp == 0 {
		return nil, fmt.Errorf("JWT exp claim missing")
	}

	expiresAt := time.Unix(claims.Exp, 0).UTC()
	return &expiresAt, nil
}

func withEnv(base []string, keyValue string) []string {
	key, _, ok := strings.Cut(keyValue, "=")
	if !ok {
		return append(base, keyValue)
	}

	prefix := key + "="
	filtered := make([]string, 0, len(base)+1)
	for _, entry := range base {
		if strings.HasPrefix(entry, prefix) {
			continue
		}
		filtered = append(filtered, entry)
	}

	return append(filtered, keyValue)
}

func trimOutput(output []byte, limit int) string {
	text := strings.TrimSpace(string(output))
	if text == "" {
		return "<no output>"
	}
	if len(text) <= limit {
		return text
	}
	return text[len(text)-limit:]
}
