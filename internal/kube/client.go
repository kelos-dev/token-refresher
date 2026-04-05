package kube

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	serviceAccountTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	serviceAccountCAPath    = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
)

type Client struct {
	baseURL    *url.URL
	token      string
	httpClient *http.Client
}

type Secret struct {
	APIVersion string            `json:"apiVersion,omitempty"`
	Kind       string            `json:"kind,omitempty"`
	Metadata   SecretMetadata    `json:"metadata"`
	Type       string            `json:"type,omitempty"`
	Immutable  *bool             `json:"immutable,omitempty"`
	Data       map[string]string `json:"data,omitempty"`
}

type SecretMetadata struct {
	Name            string `json:"name,omitempty"`
	Namespace       string `json:"namespace,omitempty"`
	ResourceVersion string `json:"resourceVersion,omitempty"`
}

type apiStatus struct {
	Message string `json:"message"`
	Reason  string `json:"reason"`
	Code    int    `json:"code"`
}

func NewInClusterClient() (*Client, error) {
	host := strings.TrimSpace(os.Getenv("KUBERNETES_SERVICE_HOST"))
	port := strings.TrimSpace(os.Getenv("KUBERNETES_SERVICE_PORT"))
	if host == "" || port == "" {
		return nil, fmt.Errorf("KUBERNETES_SERVICE_HOST and KUBERNETES_SERVICE_PORT must be set")
	}

	tokenBytes, err := os.ReadFile(serviceAccountTokenPath)
	if err != nil {
		return nil, fmt.Errorf("read service account token: %w", err)
	}

	caBytes, err := os.ReadFile(serviceAccountCAPath)
	if err != nil {
		return nil, fmt.Errorf("read service account CA: %w", err)
	}

	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caBytes) {
		return nil, fmt.Errorf("parse service account CA")
	}

	baseURL, err := url.Parse(fmt.Sprintf("https://%s:%s", host, port))
	if err != nil {
		return nil, fmt.Errorf("parse kubernetes URL: %w", err)
	}

	return &Client{
		baseURL: baseURL,
		token:   strings.TrimSpace(string(tokenBytes)),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					MinVersion: tls.VersionTLS12,
					RootCAs:    roots,
				},
			},
		},
	}, nil
}

func (c *Client) GetSecretKey(ctx context.Context, namespace, name, key string) (Secret, []byte, error) {
	secret, err := c.GetSecret(ctx, namespace, name)
	if err != nil {
		return Secret{}, nil, err
	}

	encoded, ok := secret.Data[key]
	if !ok {
		return Secret{}, nil, fmt.Errorf("secret key %q not found", key)
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return Secret{}, nil, fmt.Errorf("decode secret key %q: %w", key, err)
	}

	return secret, decoded, nil
}

func (c *Client) UpdateSecretKey(ctx context.Context, secret *Secret, key string, value []byte) error {
	if secret.Data == nil {
		secret.Data = map[string]string{}
	}

	secret.Data[key] = base64.StdEncoding.EncodeToString(value)
	return c.UpdateSecret(ctx, *secret)
}

func (c *Client) GetSecret(ctx context.Context, namespace, name string) (Secret, error) {
	path := fmt.Sprintf("/api/v1/namespaces/%s/secrets/%s", namespace, name)

	var secret Secret
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &secret); err != nil {
		return Secret{}, err
	}

	return secret, nil
}

func (c *Client) UpdateSecret(ctx context.Context, secret Secret) error {
	path := fmt.Sprintf("/api/v1/namespaces/%s/secrets/%s", secret.Metadata.Namespace, secret.Metadata.Name)
	return c.doJSON(ctx, http.MethodPut, path, secret, nil)
}

func (c *Client) doJSON(ctx context.Context, method, path string, requestBody any, responseBody any) error {
	var body io.Reader
	if requestBody != nil {
		payload, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		body = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL.ResolveReference(&url.URL{Path: path}).String(), body)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("perform request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var status apiStatus
		if err := json.Unmarshal(respBytes, &status); err == nil && status.Message != "" {
			return fmt.Errorf("kubernetes API %s: %s", resp.Status, status.Message)
		}
		return fmt.Errorf("kubernetes API %s: %s", resp.Status, strings.TrimSpace(string(respBytes)))
	}

	if responseBody == nil || len(respBytes) == 0 {
		return nil
	}

	if err := json.Unmarshal(respBytes, responseBody); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}
