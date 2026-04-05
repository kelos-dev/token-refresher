package codex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kelos-dev/token-refresher/internal/config"
)

func TestRefreshSkipsWhenTokenIsOutsideThreshold(t *testing.T) {
	t.Parallel()

	authJSON := authFixture(t, time.Now().Add(7*24*time.Hour))

	provider := &Provider{
		cfg: config.CodexConfig{
			RefreshPolicy: "threshold",
			RefreshWindow: 72 * time.Hour,
			AuthSubdir:    ".codex",
			AuthFileName:  "auth.json",
		},
		runner: func(context.Context, string, config.CodexConfig) error {
			t.Fatal("runner should not be called")
			return nil
		},
	}

	result, err := provider.Refresh(context.Background(), authJSON)
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	if result.AttemptedRefresh {
		t.Fatalf("Refresh() attempted refresh = true, want false")
	}

	if result.Changed {
		t.Fatalf("Refresh() changed = true, want false")
	}
}

func TestRefreshUpdatesAuthWhenRunnerMutatesFile(t *testing.T) {
	t.Parallel()

	original := authFixture(t, time.Now().Add(2*time.Hour))
	updated := authFixture(t, time.Now().Add(10*24*time.Hour))

	provider := &Provider{
		cfg: config.CodexConfig{
			RefreshPolicy: "always",
			RefreshWindow: 72 * time.Hour,
			AuthSubdir:    ".codex",
			AuthFileName:  "auth.json",
		},
		runner: func(_ context.Context, homeDir string, cfg config.CodexConfig) error {
			authPath := filepath.Join(homeDir, cfg.AuthSubdir, cfg.AuthFileName)
			return osWriteFile(authPath, updated)
		},
	}

	result, err := provider.Refresh(context.Background(), original)
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	if !result.AttemptedRefresh {
		t.Fatalf("Refresh() attempted refresh = false, want true")
	}

	if !result.Changed {
		t.Fatalf("Refresh() changed = false, want true")
	}

	if string(result.UpdatedAuth) != string(updated) {
		t.Fatalf("Refresh() updated auth mismatch")
	}
}

func authFixture(t *testing.T, expiresAt time.Time) []byte {
	t.Helper()

	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	claims := map[string]any{
		"exp": expiresAt.Unix(),
		"iat": time.Now().Unix(),
	}

	headerBytes, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}

	claimsBytes, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}

	token := base64.RawURLEncoding.EncodeToString(headerBytes) + "." +
		base64.RawURLEncoding.EncodeToString(claimsBytes) + ".signature"

	payload := map[string]any{
		"auth_mode": "chatgpt",
		"tokens": map[string]any{
			"access_token": token,
		},
	}

	authJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal auth.json: %v", err)
	}

	return authJSON
}

func osWriteFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o600)
}
