// HashiCorp Vault secrets backend
package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Vault is a HashiCorp Vault secrets backend
type Vault struct {
	address    string
	token      string
	mountPath  string
	secretPath string
	client     *http.Client
}

// VaultConfig for Vault backend
type VaultConfig struct {
	Address    string `yaml:"address"`     // Vault server address (default: http://127.0.0.1:8200)
	Token      string `yaml:"token"`       // Vault token (or use VAULT_TOKEN env)
	MountPath  string `yaml:"mount_path"`  // KV secrets engine mount (default: secret)
	SecretPath string `yaml:"secret_path"` // Base path for secrets (default: magabot)
}

// NewVault creates a new Vault secrets backend
func NewVault(cfg *VaultConfig) (*Vault, error) {
	address := cfg.Address
	if address == "" {
		address = os.Getenv("VAULT_ADDR")
		if address == "" {
			address = "http://127.0.0.1:8200"
		}
	}

	token := cfg.Token
	if token == "" {
		token = os.Getenv("VAULT_TOKEN")
	}
	if token == "" {
		return nil, fmt.Errorf("vault token required (set VAULT_TOKEN or config)")
	}

	mountPath := cfg.MountPath
	if mountPath == "" {
		mountPath = "secret"
	}

	secretPath := cfg.SecretPath
	if secretPath == "" {
		secretPath = "magabot"
	}

	return &Vault{
		address:    strings.TrimSuffix(address, "/"),
		token:      token,
		mountPath:  mountPath,
		secretPath: secretPath,
		client:     &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// Name returns the backend name
func (v *Vault) Name() string {
	return "vault"
}

// Get retrieves a secret from Vault
func (v *Vault) Get(ctx context.Context, key string) (string, error) {
	// KV v2 API: GET /v1/{mount}/data/{path}
	url := fmt.Sprintf("%s/v1/%s/data/%s/%s", v.address, v.mountPath, v.secretPath, key)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Vault-Token", v.token)

	resp, err := v.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrBackendError, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", ErrNotFound
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("%w: %s", ErrBackendError, string(body))
	}

	var result struct {
		Data struct {
			Data map[string]interface{} `json:"data"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	value, ok := result.Data.Data["value"].(string)
	if !ok {
		return "", ErrNotFound
	}

	return value, nil
}

// Set stores a secret in Vault
func (v *Vault) Set(ctx context.Context, key, value string) error {
	// KV v2 API: POST /v1/{mount}/data/{path}
	url := fmt.Sprintf("%s/v1/%s/data/%s/%s", v.address, v.mountPath, v.secretPath, key)

	payload := map[string]interface{}{
		"data": map[string]string{
			"value": value,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("X-Vault-Token", v.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBackendError, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%w: %s", ErrBackendError, string(respBody))
	}

	return nil
}

// Delete removes a secret from Vault
func (v *Vault) Delete(ctx context.Context, key string) error {
	// KV v2 API: DELETE /v1/{mount}/metadata/{path}
	url := fmt.Sprintf("%s/v1/%s/metadata/%s/%s", v.address, v.mountPath, v.secretPath, key)

	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Vault-Token", v.token)

	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBackendError, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%w: %s", ErrBackendError, string(respBody))
	}

	return nil
}

// List returns all secret keys
func (v *Vault) List(ctx context.Context) ([]string, error) {
	// KV v2 API: LIST /v1/{mount}/metadata/{path}
	url := fmt.Sprintf("%s/v1/%s/metadata/%s", v.address, v.mountPath, v.secretPath)

	req, err := http.NewRequestWithContext(ctx, "LIST", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Vault-Token", v.token)

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBackendError, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return []string{}, nil
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: %s", ErrBackendError, string(respBody))
	}

	var result struct {
		Data struct {
			Keys []string `json:"keys"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Data.Keys, nil
}

// Ping checks if Vault is available and authenticated
func (v *Vault) Ping(ctx context.Context) error {
	url := fmt.Sprintf("%s/v1/auth/token/lookup-self", v.address)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Vault-Token", v.token)

	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBackendError, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("vault authentication failed")
	}

	return nil
}
