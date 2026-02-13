// HashiCorp Vault secrets backend (using official SDK)
package secrets

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	vaultapi "github.com/hashicorp/vault/api"
)

// Vault is a HashiCorp Vault secrets backend
type Vault struct {
	client     *vaultapi.Client
	kv         *vaultapi.KVv2
	mountPath  string
	secretPath string
	watcher    *vaultapi.LifetimeWatcher
	logger     *slog.Logger
}

// VaultConfig for Vault backend
type VaultConfig struct {
	Address       string `yaml:"address"`                   // Vault server address (default: http://127.0.0.1:8200)
	Token         string `yaml:"token"`                     // Vault token (or use VAULT_TOKEN env)
	MountPath     string `yaml:"mount_path"`                // KV secrets engine mount (default: secret)
	SecretPath    string `yaml:"secret_path"`               // Base path for secrets (default: magabot)
	TLSCACert     string `yaml:"tls_ca_cert,omitempty"`     // CA cert file for TLS verification
	TLSClientCert string `yaml:"tls_client_cert,omitempty"` // Client cert for mTLS
	TLSClientKey  string `yaml:"tls_client_key,omitempty"`  // Client key for mTLS
	TLSSkipVerify bool   `yaml:"tls_skip_verify,omitempty"` // Skip TLS verification (insecure)
	Logger        *slog.Logger
}

// NewVault creates a new Vault secrets backend using the official HashiCorp SDK.
// The SDK automatically reads VAULT_ADDR, VAULT_TOKEN, VAULT_CACERT, etc.
func NewVault(cfg *VaultConfig) (*Vault, error) {
	vaultCfg := vaultapi.DefaultConfig()

	if cfg.Address != "" {
		vaultCfg.Address = cfg.Address
	}

	// Configure TLS
	tlsCfg := &vaultapi.TLSConfig{
		CACert:     cfg.TLSCACert,
		ClientCert: cfg.TLSClientCert,
		ClientKey:  cfg.TLSClientKey,
		Insecure:   cfg.TLSSkipVerify,
	}
	if err := vaultCfg.ConfigureTLS(tlsCfg); err != nil {
		return nil, fmt.Errorf("vault TLS config: %w", err)
	}

	client, err := vaultapi.NewClient(vaultCfg)
	if err != nil {
		return nil, fmt.Errorf("vault client: %w", err)
	}

	// Set token: explicit config > env var (SDK reads VAULT_TOKEN automatically)
	token := cfg.Token
	if token == "" {
		token = os.Getenv("VAULT_TOKEN")
	}
	if token == "" {
		return nil, fmt.Errorf("vault token required (set VAULT_TOKEN or config)")
	}
	client.SetToken(token)

	mountPath := cfg.MountPath
	if mountPath == "" {
		mountPath = "secret"
	}

	secretPath := cfg.SecretPath
	if secretPath == "" {
		secretPath = "magabot"
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	v := &Vault{
		client:     client,
		kv:         client.KVv2(mountPath),
		mountPath:  mountPath,
		secretPath: secretPath,
		logger:     logger,
	}

	return v, nil
}

// StartRenewal begins background token renewal. Call this after NewVault if
// the token is renewable (non-root, has a TTL). Safe to call on non-renewable
// tokens — it returns nil without starting anything.
func (v *Vault) StartRenewal() error {
	secret, err := v.client.Auth().Token().LookupSelf()
	if err != nil {
		return fmt.Errorf("token lookup: %w", err)
	}

	// Check if token is renewable
	renewable, _ := secret.TokenIsRenewable()
	ttl, _ := secret.TokenTTL()
	if !renewable || ttl == 0 {
		v.logger.Debug("vault token is not renewable, skipping auto-renewal",
			"renewable", renewable, "ttl", ttl)
		return nil
	}

	watcher, err := v.client.NewLifetimeWatcher(&vaultapi.LifetimeWatcherInput{
		Secret: secret,
	})
	if err != nil {
		return fmt.Errorf("lifetime watcher: %w", err)
	}

	v.watcher = watcher
	go watcher.Start()
	go v.watchRenewal()

	v.logger.Info("vault token auto-renewal started", "ttl", ttl)
	return nil
}

// watchRenewal monitors the lifetime watcher channels for renewals and expiry.
func (v *Vault) watchRenewal() {
	for {
		select {
		case err := <-v.watcher.DoneCh():
			if err != nil {
				v.logger.Error("vault token renewal failed", "error", err)
			} else {
				v.logger.Warn("vault token lifetime ended, secrets may become unavailable")
			}
			return
		case r := <-v.watcher.RenewCh():
			v.logger.Debug("vault token renewed",
				"ttl", r.Secret.LeaseDuration)
		}
	}
}

// Stop halts the background token renewal goroutine.
func (v *Vault) Stop() {
	if v.watcher != nil {
		v.watcher.Stop()
	}
}

// Name returns the backend name
func (v *Vault) Name() string {
	return "vault"
}

// fullPath returns the full secret path for a key.
func (v *Vault) fullPath(key string) string {
	return v.secretPath + "/" + key
}

// Get retrieves a secret from Vault KV v2.
func (v *Vault) Get(ctx context.Context, key string) (string, error) {
	secret, err := v.kv.Get(ctx, v.fullPath(key))
	if err != nil {
		if isVaultNotFound(err) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("%w: %v", ErrBackendError, err)
	}
	if secret == nil || secret.Data == nil {
		return "", ErrNotFound
	}

	value, ok := secret.Data["value"].(string)
	if !ok {
		return "", ErrNotFound
	}

	return value, nil
}

// Set stores a secret in Vault KV v2.
func (v *Vault) Set(ctx context.Context, key, value string) error {
	_, err := v.kv.Put(ctx, v.fullPath(key), map[string]interface{}{
		"value": value,
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBackendError, err)
	}
	return nil
}

// Delete removes a secret (all versions and metadata) from Vault KV v2.
func (v *Vault) Delete(ctx context.Context, key string) error {
	err := v.kv.DeleteMetadata(ctx, v.fullPath(key))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBackendError, err)
	}
	return nil
}

// List returns all secret keys under the configured path.
// The KVv2 helper doesn't expose List, so we use Logical().ListWithContext.
func (v *Vault) List(ctx context.Context) ([]string, error) {
	path := fmt.Sprintf("%s/metadata/%s", v.mountPath, v.secretPath)
	secret, err := v.client.Logical().ListWithContext(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBackendError, err)
	}
	if secret == nil || secret.Data == nil {
		return []string{}, nil
	}

	keysRaw, ok := secret.Data["keys"]
	if !ok {
		return []string{}, nil
	}

	keysList, ok := keysRaw.([]interface{})
	if !ok {
		return []string{}, nil
	}

	keys := make([]string, 0, len(keysList))
	for _, k := range keysList {
		if s, ok := k.(string); ok {
			keys = append(keys, s)
		}
	}

	return keys, nil
}

// Ping checks if Vault is available and the token is authenticated.
func (v *Vault) Ping(ctx context.Context) error {
	_, err := v.client.Auth().Token().LookupSelfWithContext(ctx)
	if err != nil {
		return fmt.Errorf("vault authentication failed: %w", err)
	}
	return nil
}

// isVaultNotFound checks if a Vault API error is a 404 (secret not found).
func isVaultNotFound(err error) bool {
	if err == nil {
		return false
	}
	// The SDK returns a *vaultapi.ResponseError for HTTP errors.
	if respErr, ok := err.(*vaultapi.ResponseError); ok {
		return respErr.StatusCode == 404
	}
	// Fallback: check error message for common patterns
	msg := err.Error()
	return strings.Contains(msg, "secret not found") ||
		strings.Contains(msg, strconv.Itoa(404))
}
