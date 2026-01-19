package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	log "github.com/sirupsen/logrus"
)

// FlexiblePort is a custom type that can unmarshal both strings and integers
type FlexiblePort int16

// UnmarshalText implements the encoding.TextUnmarshaler interface
func (p *FlexiblePort) UnmarshalText(text []byte) error {
	s := strings.TrimSpace(string(text))
	if s == "" {
		*p = 0
		return nil
	}

	val, err := strconv.ParseInt(s, 10, 16)
	if err != nil {
		return fmt.Errorf("invalid port value: %w", err)
	}

	*p = FlexiblePort(val)
	return nil
}

const EnvProd string = "prod"
const EnvDev string = "dev"

const ConfigContextKey string = "config"

// DefaultLogoURL is a simple billing/payment icon (white dollar sign on purple circle)
// SVG: <svg xmlns="http://www.w3.org/2000/svg" width="64" height="64" viewBox="0 0 64 64">
//
//	<circle cx="32" cy="32" r="30" fill="#9945FF"/>
//	<text x="32" y="44" font-family="Arial" font-size="36" font-weight="bold" fill="white" text-anchor="middle">$</text>
//	</svg>
const DefaultLogoURL = "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSI2NCIgaGVpZ2h0PSI2NCIgdmlld0JveD0iMCAwIDY0IDY0Ij48Y2lyY2xlIGN4PSIzMiIgY3k9IjMyIiByPSIzMCIgZmlsbD0iIzk5NDVGRiIvPjx0ZXh0IHg9IjMyIiB5PSI0NCIgZm9udC1mYW1pbHk9IkFyaWFsIiBmb250LXNpemU9IjM2IiBmb250LXdlaWdodD0iYm9sZCIgZmlsbD0id2hpdGUiIHRleHQtYW5jaG9yPSJtaWRkbGUiPiQ8L3RleHQ+PC9zdmc+"

// StoreConfig holds merchant/store branding configuration.
// Used across the system for consistent branding (Solana Pay, emails, etc.)
type StoreConfig struct {
	// Name is the merchant/store name displayed to customers (e.g., in Solana Pay QR codes, emails)
	Name string `koanf:"name"`
	// LogoURL is the URL to the store logo/icon (used in Solana Pay QR codes, etc.)
	// Must be an absolute HTTPS URL to an SVG, PNG, or WebP image
	LogoURL string `koanf:"logo_url"`
	// FromEmail is the sender email address for all outgoing emails (receipts, notifications, etc.)
	// Example: "noreply@mystore.com" or "billing@mystore.com"
	FromEmail string `koanf:"from_email"`
}

type Config struct {
	Env         string       `koanf:"env,omitempty"`
	Port        FlexiblePort `koanf:"port,omitempty"`         // Standalone only: public HTTP port (default 2053)
	PrivatePort FlexiblePort `koanf:"private_port,omitempty"` // Standalone only: internal/service API port (default 8060)
	Host        string       `koanf:"host,omitempty"`         // Standalone only: address to bind to (default 0.0.0.0)
	APIKey      string       `koanf:"api_key,omitempty"`      // Shared secret for service-to-service auth (X-API-KEY header)

	// TestMode controls whether payment processors use sandbox/test environments.
	// When true: NMI uses sandbox.nmi.com, CCBill uses sandbox-api.ccbill.com,
	// Solana uses devnet, Stripe requires sk_test_* key.
	// When false: All processors use production environments (real charges).
	// Defaults to true for safety. Set to false only for production deployments.
	// Note: This is orthogonal to Env - Env controls logging/debug, TestMode controls payments.
	TestMode *bool `koanf:"test_mode,omitempty"`

	// APIURL is the base URL where billing's /v1/* routes are mounted.
	// Used for generating URLs (e.g., Solana Pay transaction_request URLs).
	//
	// Standalone mode: "https://api.mysite.com" (routes at /v1/*)
	// Embedded mode:   "https://api.mysite.com/billing" (routes at /billing/v1/*)
	//
	// Formula: generated_url = APIURL + "/v1/checkout/:id/solana-pay"
	APIURL string       `koanf:"api_url,omitempty"`
	Store  *StoreConfig `koanf:"store,omitempty"`

	// Processors is the unified configuration for all payment processors.
	// Each key is the processor name (e.g., "mobius", "ccbill", "stripe", "solana").
	// Reserved names (ccbill, stripe, solana) don't need explicit "type" field.
	// Non-reserved names (e.g., "mobius", "acme") require "type: nmi".
	//
	// Example:
	//   processors:
	//     mobius:
	//       type: nmi
	//       security_key: "..."
	//     ccbill:
	//       client_acc_num: "..."
	//     stripe:
	//       secret_key: "sk_..."
	//     solana:
	//       recipient_wallet: "..."
	Processors map[string]*ProcessorConfig `koanf:"processors,omitempty"`

	// DEPRECATED: Use Processors["mobius"] (or other NMI provider names) instead.
	// Kept for backwards compatibility during migration.
	NMI *NMIConfig `koanf:"nmi,omitempty"`
	// DEPRECATED: Use Processors["ccbill"] instead.
	CCBill *CCBillConfig `koanf:"ccbill,omitempty"`
	// DEPRECATED: Use Processors["solana"] instead.
	Solana *SolanaConfig `koanf:"solana,omitempty"`
	// DEPRECATED: Use Processors["stripe"] instead.
	Stripe *StripeConfig `koanf:"stripe,omitempty"`

	Webhooks    *WebhookConfig    `koanf:"webhooks,omitempty"`
	DB          *DBConfig         `koanf:"db,omitempty"`
	Redis       *RedisConfig      `koanf:"redis,omitempty"`
	Auth        *AuthConfig       `koanf:"auth,omitempty"`
	ClickHouse  *ClickHouseConfig `koanf:"clickhouse,omitempty"`
	Logger      *LoggerConfig     `koanf:"logger,omitempty"`
	SendGrid    *SendGridConfig   `koanf:"sendgrid,omitempty"`
	CorsOrigins []string          `koanf:"cors_origins,omitempty"`
	RateLimits  *RateLimitsConfig `koanf:"rate_limits,omitempty"`
}

// DBConfig holds database configuration.
// Supports both legacy connection string (URL) and atomic parameters.
// If URL is provided, it takes precedence. Otherwise, connection string
// is built from individual parameters (Host, Port, Username, etc.).
// Database is always PostgreSQL.
type DBConfig struct {
	// Legacy: Full connection string (optional)
	URL string `koanf:"url"`

	// Atomic parameters (preferred for template-based configuration)
	Host     string `koanf:"host"`
	Port     string `koanf:"port"`
	Database string `koanf:"database"`
	Username string `koanf:"username"`
	Password string `koanf:"password"`
	SSLMode  string `koanf:"sslmode"`
}

// GetConnectionString returns the database connection string.
// Priority order:
// 1. If URL is set, use it directly
// 2. If all atomic parameters are present, build connection string from them
// 3. Return empty string (caller should use defaults or error based on environment)
func (c *DBConfig) GetConnectionString() string {
	// 1. If legacy URL is provided, use it
	if c.URL != "" {
		return c.URL
	}

	// 2. Build connection string from atomic parameters if all required fields are present
	if c.Host != "" && c.Port != "" && c.Database != "" && c.Username != "" {
		// Format: postgresql://username:password@host:port/database?sslmode=...
		connStr := fmt.Sprintf(
			"postgresql://%s:%s@%s:%s/%s",
			c.Username,
			c.Password,
			c.Host,
			c.Port,
			c.Database,
		)

		// Add query parameters
		params := []string{}

		// Default to sslmode=disable if not specified
		sslMode := c.SSLMode
		if sslMode == "" {
			sslMode = "disable"
		}
		params = append(params, fmt.Sprintf("sslmode=%s", sslMode))

		if len(params) > 0 {
			connStr += "?" + strings.Join(params, "&")
		}

		return connStr
	}

	// 3. No URL and incomplete atomic parameters - return empty (caller handles defaults)
	return ""
}

type NMIConfig struct {
	SecurityKey     string                        `koanf:"security_key"`
	TokenizationKey string                        `koanf:"tokenization_key"`
	WebhookSecret   string                        `koanf:"webhook_secret"`
	TestMode        bool                          `koanf:"test_mode"`
	DirectPostURL   string                        `koanf:"direct_post_url"`
	QueryURL        string                        `koanf:"query_url"`
	Providers       map[string]*NMIProviderConfig `koanf:"providers"`
}

type NMIProviderConfig struct {
	SecurityKey     string `koanf:"security_key"`
	TokenizationKey string `koanf:"tokenization_key"`
	WebhookSecret   string `koanf:"webhook_secret"`
	TestMode        *bool  `koanf:"test_mode"`
	DirectPostURL   string `koanf:"direct_post_url"`
	QueryURL        string `koanf:"query_url"`
}

type StripeConfig struct {
	SecretKey     string `koanf:"secret_key"`
	WebhookSecret string `koanf:"webhook_secret"`
	SuccessURL    string `koanf:"success_url"`
	CancelURL     string `koanf:"cancel_url"`
}

// ProcessorType constants for the unified processor config
const (
	ProcessorTypeNMI    = "nmi"
	ProcessorTypeCCBill = "ccbill"
	ProcessorTypeStripe = "stripe"
	ProcessorTypeSolana = "solana"
)

// ReservedProcessorNames maps processor names that imply their type.
// These names don't require an explicit "type" field in config.
var ReservedProcessorNames = map[string]string{
	"ccbill": ProcessorTypeCCBill,
	"stripe": ProcessorTypeStripe,
	"solana": ProcessorTypeSolana,
}

// ProcessorConfig is the unified configuration for all payment processors.
// The Type field determines which fields are relevant:
//   - type: nmi     → NMI fields (security_key, webhook_secret, etc.)
//   - type: ccbill  → CCBill fields (client_acc_num, client_sub_acc, salt, etc.)
//   - type: stripe  → Stripe fields (secret_key, webhook_secret, success_url, cancel_url)
//   - type: solana  → Solana fields (recipient_wallet, rpc_endpoint, etc.)
//
// Reserved names (ccbill, stripe, solana) don't need explicit type - it's implied.
// Non-reserved names (e.g., "mobius", "acme") require type: nmi.
type ProcessorConfig struct {
	// Type specifies the processor type: "nmi", "ccbill", "stripe", "solana"
	// Required for non-reserved processor names.
	// For reserved names (ccbill, stripe, solana), type is inferred from the name.
	Type string `koanf:"type"`

	// --- NMI fields (type: nmi) ---
	SecurityKey     string `koanf:"security_key"`
	TokenizationKey string `koanf:"tokenization_key"`
	WebhookSecret   string `koanf:"webhook_secret"`
	DirectPostURL   string `koanf:"direct_post_url"`
	QueryURL        string `koanf:"query_url"`

	// --- CCBill fields (type: ccbill) ---
	Salt               string `koanf:"salt"`
	ClientSubAcc       string `koanf:"client_sub_acc"`
	ClientAccNum       string `koanf:"client_acc_num"`
	SubscriptionTypeId string `koanf:"subscription_type_id"`
	DataLinkUsername   string `koanf:"datalink_username"`
	DataLinkPassword   string `koanf:"datalink_password"`

	// --- Stripe fields (type: stripe) ---
	SecretKey  string `koanf:"secret_key"`
	SuccessURL string `koanf:"success_url"`
	CancelURL  string `koanf:"cancel_url"`
	// WebhookSecret is shared with NMI (same field name)

	// --- Solana fields (type: solana) ---
	RPCEndpoint               string                 `koanf:"rpc_endpoint"`
	HeliusAPIKey              string                 `koanf:"helius_api_key"`
	Network                   string                 `koanf:"network"`
	RecipientWallet           string                 `koanf:"recipient_wallet"`
	SupportedTokens           map[string]TokenConfig `koanf:"supported_tokens"`
	EnabledTokens             []string               `koanf:"enabled_tokens"`
	TransactionTimeoutSeconds int                    `koanf:"transaction_timeout_seconds"`
	ConfirmationBlocks        int                    `koanf:"confirmation_blocks"`
	MaxTransactionFee         float64                `koanf:"max_transaction_fee"`
}

// GetEffectiveType returns the processor type, inferring from reserved names if needed.
func (p *ProcessorConfig) GetEffectiveType(name string) string {
	if p.Type != "" {
		return strings.ToLower(p.Type)
	}
	// Check if it's a reserved name
	normalizedName := strings.ToLower(strings.TrimSpace(name))
	if impliedType, ok := ReservedProcessorNames[normalizedName]; ok {
		return impliedType
	}
	return ""
}

// IsNMI returns true if this processor config is for an NMI-backed processor.
func (p *ProcessorConfig) IsNMI(name string) bool {
	return p.GetEffectiveType(name) == ProcessorTypeNMI
}

// IsCCBill returns true if this processor config is for CCBill.
func (p *ProcessorConfig) IsCCBill(name string) bool {
	return p.GetEffectiveType(name) == ProcessorTypeCCBill
}

// IsStripe returns true if this processor config is for Stripe.
func (p *ProcessorConfig) IsStripe(name string) bool {
	return p.GetEffectiveType(name) == ProcessorTypeStripe
}

// IsSolana returns true if this processor config is for Solana.
func (p *ProcessorConfig) IsSolana(name string) bool {
	return p.GetEffectiveType(name) == ProcessorTypeSolana
}

// ToNMIProviderSettings converts the processor config to NMI provider settings.
// Only valid for NMI-type processors.
func (p *ProcessorConfig) ToNMIProviderSettings(name string) *NMIProviderSettings {
	return &NMIProviderSettings{
		Name:            strings.ToLower(strings.TrimSpace(name)),
		SecurityKey:     p.SecurityKey,
		TokenizationKey: p.TokenizationKey,
		WebhookSecret:   p.WebhookSecret,
		DirectPostURL:   p.DirectPostURL,
		QueryURL:        p.QueryURL,
		TestMode:        false, // Will be set by caller based on global test_mode
	}
}

// ToCCBillConfig converts the processor config to CCBillConfig.
// Only valid for CCBill-type processors.
func (p *ProcessorConfig) ToCCBillConfig() *CCBillConfig {
	return &CCBillConfig{
		Salt:               p.Salt,
		ClientSubAcc:       p.ClientSubAcc,
		ClientAccNum:       p.ClientAccNum,
		SubscriptionTypeId: p.SubscriptionTypeId,
		DataLinkUsername:   p.DataLinkUsername,
		DataLinkPassword:   p.DataLinkPassword,
		TestMode:           false, // Will be set by caller based on global test_mode
	}
}

// ToStripeConfig converts the processor config to StripeConfig.
// Only valid for Stripe-type processors.
func (p *ProcessorConfig) ToStripeConfig() *StripeConfig {
	return &StripeConfig{
		SecretKey:     p.SecretKey,
		WebhookSecret: p.WebhookSecret,
		SuccessURL:    p.SuccessURL,
		CancelURL:     p.CancelURL,
	}
}

// ToSolanaConfig converts the processor config to SolanaConfig.
// Only valid for Solana-type processors.
func (p *ProcessorConfig) ToSolanaConfig() *SolanaConfig {
	return &SolanaConfig{
		RPCEndpoint:               p.RPCEndpoint,
		HeliusAPIKey:              p.HeliusAPIKey,
		Network:                   p.Network,
		RecipientWallet:           p.RecipientWallet,
		SupportedTokens:           p.SupportedTokens,
		EnabledTokens:             p.EnabledTokens,
		TransactionTimeoutSeconds: p.TransactionTimeoutSeconds,
		ConfirmationBlocks:        p.ConfirmationBlocks,
		MaxTransactionFee:         p.MaxTransactionFee,
	}
}

type NMIProviderSettings struct {
	Name            string
	SecurityKey     string
	TokenizationKey string
	WebhookSecret   string
	TestMode        bool
	DirectPostURL   string
	QueryURL        string
}

func (cfg *NMIConfig) ProviderSettings(name string) (*NMIProviderSettings, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nmi configuration is missing")
	}

	providerKey := strings.TrimSpace(strings.ToLower(name))
	if providerKey == "" {
		providerKey = "mobius"
	}

	provider, ok := cfg.Providers[providerKey]
	if !ok || provider == nil {
		return nil, fmt.Errorf("nmi provider '%s' is not configured", providerKey)
	}

	settings := &NMIProviderSettings{
		Name:            providerKey,
		SecurityKey:     firstNonEmpty(provider.SecurityKey, cfg.SecurityKey),
		TokenizationKey: firstNonEmpty(provider.TokenizationKey, cfg.TokenizationKey),
		WebhookSecret:   firstNonEmpty(provider.WebhookSecret, cfg.WebhookSecret),
		DirectPostURL:   firstNonEmpty(provider.DirectPostURL, cfg.DirectPostURL),
		QueryURL:        firstNonEmpty(provider.QueryURL, cfg.QueryURL),
		TestMode:        cfg.TestMode,
	}
	if provider.TestMode != nil {
		settings.TestMode = *provider.TestMode
	}

	if settings.SecurityKey == "" {
		return nil, fmt.Errorf("nmi provider '%s' security key is required", providerKey)
	}
	if settings.WebhookSecret == "" {
		log.Warnf("nmi provider '%s' webhook secret is not configured; signature validation will be disabled", providerKey)
	}

	return settings, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

type CCBillConfig struct {
	Salt               string `koanf:"salt"`
	ClientSubAcc       string `koanf:"client_sub_acc"`
	ClientAccNum       string `koanf:"client_acc_num"`
	SubscriptionTypeId string `koanf:"subscription_type_id"`
	TestMode           bool   `koanf:"test_mode"`

	DataLinkUsername string `koanf:"datalink_username"`
	DataLinkPassword string `koanf:"datalink_password"`
}

type RedisConfig struct {
	Addr     string `koanf:"addr"`
	Password string `koanf:"password"`
	DB       int    `koanf:"db"`
}

// AuthConfig holds JWT verification configuration for billing service.
// Billing is a JWT verifier (not issuer) - it validates tokens issued by doujins/hentai0.
type AuthConfig struct {
	Issuers          []string `koanf:"issuers"`           // List of expected token issuers (e.g., ["https://doujins.com", "https://hentai0.com"])
	ExpectedAudience string   `koanf:"expected_audience"` // Accept token only if it contains this audience (e.g., "billing-app")
}

type SolanaConfig struct {
	// RPCEndpoint is a custom RPC endpoint override. If set, it bypasses the fallback chain entirely.
	// Leave empty to use the automatic fallback chain: Helius (if configured) → Ankr → Solana public.
	RPCEndpoint string `koanf:"rpc_endpoint"`

	// HeliusAPIKey enables Helius as the primary RPC provider (recommended for production).
	// Get a free API key at https://helius.dev (100k requests/day on free tier).
	// If not set, falls back to Ankr → Solana public endpoints.
	HeliusAPIKey string `koanf:"helius_api_key"`

	Network         string `koanf:"network"` // mainnet, devnet, testnet
	RecipientWallet string `koanf:"recipient_wallet"`

	SupportedTokens map[string]TokenConfig `koanf:"supported_tokens,omitempty"`

	// EnabledTokens is a simplified token configuration (alternative to SupportedTokens).
	// Use symbol strings for verified tokens (Jupiter lookup): ["SOL", "USDC", "BONK"]
	// If not set, defaults to ["SOL", "USDC", "PYUSD"].
	EnabledTokens []string `koanf:"enabled_tokens,omitempty"`

	TransactionTimeoutSeconds int     `koanf:"transaction_timeout_seconds,omitempty"`
	ConfirmationBlocks        int     `koanf:"confirmation_blocks,omitempty"`
	MaxTransactionFee         float64 `koanf:"max_transaction_fee,omitempty"`
}

// TokenConfig defines configuration for a specific Solana token
type TokenConfig struct {
	Mint        string  `json:"mint" koanf:"mint"`         // Token mint address
	Symbol      string  `json:"symbol" koanf:"symbol"`     // Token symbol (e.g., "SOL", "USDC")
	Name        string  `json:"name" koanf:"name"`         // Token name
	Decimals    int     `json:"decimals" koanf:"decimals"` // Token decimal places
	Price       float64 `json:"price"`                     // Price in USD (fetched from Jupiter at runtime, not loaded from config)
	Enabled     bool    `json:"enabled" koanf:"enabled"`   // Whether this token is enabled
	MainnetMint string  `json:"mainnet_mint,omitempty" koanf:"mainnet_mint"`
}

// RateLimitsConfig is a map of endpoint identifier -> rate limit config
type RateLimitsConfig map[string]*RateLimit

// SendGridConfig holds SendGrid email configuration.
// Sender info (from_email, from_name) comes from StoreConfig.
type SendGridConfig struct {
	APIKey string `koanf:"api_key"`
}

type ClickHouseConfig struct {
	HTTPAddr   string `koanf:"http_addr"`   // HTTP address for queries, e.g., http://clickhouse:8123
	ClientAddr string `koanf:"client_addr"` // Native client address, e.g., clickhouse:9000
	Database   string `koanf:"db"`          // ClickHouse database name (e.g., analytics)
	Username   string `koanf:"user"`        // Optional username for authentication
	Password   string `koanf:"password"`    // Optional password for authentication
	Cluster    string `koanf:"cluster"`     // ClickHouse cluster name (e.g., doujins)
}

// LoggerConfig holds logging configuration
type LoggerConfig struct {
	Level string `koanf:"level"` // debug | info | error
}

// RateLimit defines a rate limit policy.
// All rate limits use a fixed 1-minute window.
type RateLimit struct {
	// RequestsPerMinute is the maximum number of requests allowed per minute.
	RequestsPerMinute int `koanf:"requests_per_minute"`
	// Burst is the maximum burst size (optional, defaults to RequestsPerMinute).
	// Reserved for future use with token bucket algorithms.
	Burst int `koanf:"burst"`
}

// WebhookConfig is kept for backwards compatibility but webhook retry is no longer used.
// Webhook processing is now synchronous-only - payment processors retry on their end.
type WebhookConfig struct {
	// Deprecated: Retry config is no longer used. Webhooks are processed synchronously.
	Retry WebhookRetryConfig `koanf:"retry"`
}

// WebhookRetryConfig is deprecated - webhook retry mechanism has been removed.
// Keeping the struct for backwards compatibility with existing config files.
type WebhookRetryConfig struct {
	MaxAttempts    int           `koanf:"max_attempts"`
	InitialBackoff time.Duration `koanf:"initial_backoff"`
	MaxBackoff     time.Duration `koanf:"max_backoff"`
	BatchSize      int           `koanf:"batch_size"`
}

// Validate validates the billing configuration
func Validate(cfg *Config) error {
	// Skip strict validation in development environments
	isDev := cfg.Env == "development" || cfg.Env == "dev" || cfg.Env == ""

	// Validate new-style Processors map first
	if len(cfg.Processors) > 0 {
		if err := validateProcessors(cfg, isDev); err != nil {
			return fmt.Errorf("processors validation failed: %w", err)
		}
	} else if !isDev {
		// Fall back to legacy validation if no new-style processors
		if err := validateNMI(cfg.NMI); err != nil {
			return fmt.Errorf("nmi config validation failed: %w", err)
		}

		// Validate CCBill configuration
		if err := validateCCBill(cfg.CCBill); err != nil {
			return fmt.Errorf("ccbill config validation failed: %w", err)
		}

		// Validate Stripe configuration
		if err := validateStripe(cfg.Stripe); err != nil {
			return fmt.Errorf("stripe config validation failed: %w", err)
		}
	}

	// Validate Stripe key prefix matches test_mode
	// This runs after processor validation to check the key we'll actually use
	validateStripeKeyForTestMode(cfg)

	// Always validate database configuration
	if err := validateDatabase(cfg.DB); err != nil {
		return fmt.Errorf("database config validation failed: %w", err)
	}

	return nil
}

// validateStripeKeyForTestMode checks if the Stripe API key prefix matches the test_mode setting.
// If there's a mismatch, it logs a warning and clears the key to disable Stripe.
// This prevents accidentally processing real charges in test mode or test charges in production.
func validateStripeKeyForTestMode(cfg *Config) {
	// Check both new Processors map and legacy Stripe config
	var secretKey string

	if stripeProc, ok := cfg.Processors["stripe"]; ok && stripeProc != nil {
		secretKey = strings.TrimSpace(stripeProc.SecretKey)
	} else if cfg.Stripe != nil {
		secretKey = strings.TrimSpace(cfg.Stripe.SecretKey)
	}

	if secretKey == "" {
		return // No key configured, nothing to validate
	}

	isLiveKey := strings.HasPrefix(secretKey, "sk_live_")
	isTestKey := strings.HasPrefix(secretKey, "sk_test_")

	if cfg.IsTestMode() && isLiveKey {
		log.Warn("⚠️  Stripe live key provided but test_mode is enabled - disabling Stripe")
		log.Warn("   Use sk_test_* key when test_mode=true, or set test_mode=false for production")
		// Clear the key to disable Stripe
		if stripeProc, ok := cfg.Processors["stripe"]; ok && stripeProc != nil {
			stripeProc.SecretKey = ""
		}
		if cfg.Stripe != nil {
			cfg.Stripe.SecretKey = ""
		}
	} else if !cfg.IsTestMode() && isTestKey {
		log.Warn("⚠️  Stripe test key provided but test_mode is disabled (production) - disabling Stripe")
		log.Warn("   Use sk_live_* key when test_mode=false, or set test_mode=true for testing")
		// Clear the key to disable Stripe
		if stripeProc, ok := cfg.Processors["stripe"]; ok && stripeProc != nil {
			stripeProc.SecretKey = ""
		}
		if cfg.Stripe != nil {
			cfg.Stripe.SecretKey = ""
		}
	}
}

// validateProcessors validates all processors in the new Processors map
func validateProcessors(cfg *Config, isDev bool) error {
	for name, proc := range cfg.Processors {
		if proc == nil {
			continue
		}

		effectiveType := proc.GetEffectiveType(name)
		switch effectiveType {
		case ProcessorTypeNMI:
			if err := validateNMIProcessor(name, proc, isDev); err != nil {
				return err
			}
		case ProcessorTypeCCBill:
			if err := validateCCBillProcessor(name, proc, isDev); err != nil {
				return err
			}
		case ProcessorTypeStripe:
			if err := validateStripeProcessor(name, proc, isDev); err != nil {
				return err
			}
		case ProcessorTypeSolana:
			if err := validateSolanaProcessor(name, proc, isDev); err != nil {
				return err
			}
		default:
			return fmt.Errorf("processor '%s' has unknown type '%s'", name, effectiveType)
		}
	}
	return nil
}

// validateNMIProcessor validates an NMI-type processor
func validateNMIProcessor(name string, proc *ProcessorConfig, isDev bool) error {
	if isDev {
		return nil // Skip strict validation in dev
	}

	if strings.TrimSpace(proc.SecurityKey) == "" {
		return fmt.Errorf("processor '%s' (nmi): security_key is required", name)
	}

	if strings.TrimSpace(proc.WebhookSecret) == "" {
		log.Warnf("processor '%s' (nmi): webhook_secret not configured; signature verification disabled", name)
	}

	if proc.DirectPostURL != "" {
		if _, err := url.Parse(proc.DirectPostURL); err != nil {
			return fmt.Errorf("processor '%s' (nmi): invalid direct_post_url: %w", name, err)
		}
	}

	if proc.QueryURL != "" {
		if _, err := url.Parse(proc.QueryURL); err != nil {
			return fmt.Errorf("processor '%s' (nmi): invalid query_url: %w", name, err)
		}
	}

	return nil
}

// validateCCBillProcessor validates a CCBill-type processor
func validateCCBillProcessor(name string, proc *ProcessorConfig, isDev bool) error {
	if isDev {
		return nil // Skip strict validation in dev
	}

	if strings.TrimSpace(proc.ClientAccNum) == "" {
		return fmt.Errorf("processor '%s' (ccbill): client_acc_num is required", name)
	}

	if strings.TrimSpace(proc.ClientSubAcc) == "" {
		return fmt.Errorf("processor '%s' (ccbill): client_sub_acc is required", name)
	}

	// DataLink credentials: either both or neither
	hasUsername := strings.TrimSpace(proc.DataLinkUsername) != ""
	hasPassword := strings.TrimSpace(proc.DataLinkPassword) != ""
	if hasUsername != hasPassword {
		return fmt.Errorf("processor '%s' (ccbill): both datalink_username and datalink_password must be provided when configuring DataLink", name)
	}

	return nil
}

// validateStripeProcessor validates a Stripe-type processor
func validateStripeProcessor(name string, proc *ProcessorConfig, isDev bool) error {
	if strings.TrimSpace(proc.SecretKey) == "" {
		log.Warnf("processor '%s' (stripe): secret_key not configured; checkout unavailable", name)
	}

	if strings.TrimSpace(proc.WebhookSecret) == "" {
		log.Warnf("processor '%s' (stripe): webhook_secret not configured; signature verification disabled", name)
	}

	return nil
}

// validateSolanaProcessor validates a Solana-type processor
func validateSolanaProcessor(name string, proc *ProcessorConfig, isDev bool) error {
	if strings.TrimSpace(proc.RecipientWallet) == "" {
		log.Warnf("processor '%s' (solana): recipient_wallet not configured; Solana payments disabled", name)
	}

	return nil
}

func validateStripe(cfg *StripeConfig) error {
	if cfg == nil {
		return nil
	}
	if strings.TrimSpace(cfg.SecretKey) == "" {
		log.Warn("stripe secret key not configured; checkout will be unavailable")
	}
	if strings.TrimSpace(cfg.WebhookSecret) == "" {
		log.Warn("stripe webhook secret not configured; signature verification will be disabled")
	}
	return nil
}

// GetWebhookRetryConfig is deprecated - webhook retry mechanism has been removed.
// Returns empty config. Kept for backwards compatibility.
func (cfg *Config) GetWebhookRetryConfig() WebhookRetryConfig {
	return WebhookRetryConfig{}
}

// GetNMIProcessors returns all NMI-backed processor configs.
// Checks both the new Processors map and legacy NMI config.
func (cfg *Config) GetNMIProcessors() map[string]*ProcessorConfig {
	result := make(map[string]*ProcessorConfig)

	// First, check the new Processors map
	for name, proc := range cfg.Processors {
		if proc != nil && proc.IsNMI(name) {
			result[strings.ToLower(name)] = proc
		}
	}

	// Fall back to legacy NMI config if no new-style processors found
	if len(result) == 0 && cfg.NMI != nil && len(cfg.NMI.Providers) > 0 {
		for name, provider := range cfg.NMI.Providers {
			if provider == nil {
				continue
			}
			result[strings.ToLower(name)] = &ProcessorConfig{
				Type:            ProcessorTypeNMI,
				SecurityKey:     firstNonEmpty(provider.SecurityKey, cfg.NMI.SecurityKey),
				TokenizationKey: firstNonEmpty(provider.TokenizationKey, cfg.NMI.TokenizationKey),
				WebhookSecret:   firstNonEmpty(provider.WebhookSecret, cfg.NMI.WebhookSecret),
				DirectPostURL:   firstNonEmpty(provider.DirectPostURL, cfg.NMI.DirectPostURL),
				QueryURL:        firstNonEmpty(provider.QueryURL, cfg.NMI.QueryURL),
			}
		}
	}

	return result
}

// GetCCBillProcessor returns the CCBill processor config.
// Checks both the new Processors map and legacy CCBill config.
func (cfg *Config) GetCCBillProcessor() *ProcessorConfig {
	// Check new Processors map first
	if proc, ok := cfg.Processors["ccbill"]; ok && proc != nil {
		return proc
	}

	// Fall back to legacy CCBill config
	if cfg.CCBill != nil {
		return &ProcessorConfig{
			Type:               ProcessorTypeCCBill,
			Salt:               cfg.CCBill.Salt,
			ClientSubAcc:       cfg.CCBill.ClientSubAcc,
			ClientAccNum:       cfg.CCBill.ClientAccNum,
			SubscriptionTypeId: cfg.CCBill.SubscriptionTypeId,
			DataLinkUsername:   cfg.CCBill.DataLinkUsername,
			DataLinkPassword:   cfg.CCBill.DataLinkPassword,
		}
	}

	return nil
}

// GetStripeProcessor returns the Stripe processor config.
// Checks both the new Processors map and legacy Stripe config.
func (cfg *Config) GetStripeProcessor() *ProcessorConfig {
	// Check new Processors map first
	if proc, ok := cfg.Processors["stripe"]; ok && proc != nil {
		return proc
	}

	// Fall back to legacy Stripe config
	if cfg.Stripe != nil {
		return &ProcessorConfig{
			Type:          ProcessorTypeStripe,
			SecretKey:     cfg.Stripe.SecretKey,
			WebhookSecret: cfg.Stripe.WebhookSecret,
			SuccessURL:    cfg.Stripe.SuccessURL,
			CancelURL:     cfg.Stripe.CancelURL,
		}
	}

	return nil
}

// GetSolanaProcessor returns the Solana processor config.
// Checks both the new Processors map and legacy Solana config.
func (cfg *Config) GetSolanaProcessor() *ProcessorConfig {
	// Check new Processors map first
	if proc, ok := cfg.Processors["solana"]; ok && proc != nil {
		return proc
	}

	// Fall back to legacy Solana config
	if cfg.Solana != nil {
		return &ProcessorConfig{
			Type:                      ProcessorTypeSolana,
			RPCEndpoint:               cfg.Solana.RPCEndpoint,
			Network:                   cfg.Solana.Network,
			RecipientWallet:           cfg.Solana.RecipientWallet,
			SupportedTokens:           cfg.Solana.SupportedTokens,
			TransactionTimeoutSeconds: cfg.Solana.TransactionTimeoutSeconds,
			ConfirmationBlocks:        cfg.Solana.ConfirmationBlocks,
			MaxTransactionFee:         cfg.Solana.MaxTransactionFee,
		}
	}

	return nil
}

// GetProcessor returns a processor config by name.
// Checks the new Processors map first, then falls back to legacy configs.
func (cfg *Config) GetProcessor(name string) *ProcessorConfig {
	normalizedName := strings.ToLower(strings.TrimSpace(name))

	// Check new Processors map first
	if proc, ok := cfg.Processors[normalizedName]; ok && proc != nil {
		return proc
	}

	// Fall back to legacy configs based on name
	switch normalizedName {
	case "ccbill":
		return cfg.GetCCBillProcessor()
	case "stripe":
		return cfg.GetStripeProcessor()
	case "solana":
		return cfg.GetSolanaProcessor()
	default:
		// Could be an NMI provider
		nmiProcessors := cfg.GetNMIProcessors()
		if proc, ok := nmiProcessors[normalizedName]; ok {
			return proc
		}
	}

	return nil
}

// GetProcessorType returns the type of a processor by name.
// Returns empty string if processor not found.
func (cfg *Config) GetProcessorType(name string) string {
	proc := cfg.GetProcessor(name)
	if proc == nil {
		return ""
	}
	return proc.GetEffectiveType(name)
}

// IsNMIProcessor returns true if the named processor is NMI-backed.
func (cfg *Config) IsNMIProcessor(name string) bool {
	return cfg.GetProcessorType(name) == ProcessorTypeNMI
}

// IsTestMode returns true if payment processors should use sandbox/test environments.
// This is a simple accessor - TestMode defaults to true for safety.
// Note: This is orthogonal to Env. Env controls logging/debug, TestMode controls payments.
func (cfg *Config) IsTestMode() bool {
	if cfg.TestMode == nil {
		return true // Default to test mode for safety
	}
	return *cfg.TestMode
}

// IsDev returns true if the environment is development.
func (cfg *Config) IsDev() bool {
	return cfg.Env == "" || cfg.Env == "dev" || cfg.Env == "development"
}

// validateNMI validates NMI-specific configuration
func validateNMI(cfg *NMIConfig) error {
	if cfg == nil {
		return fmt.Errorf("nmi configuration is required")
	}

	if cfg.DirectPostURL != "" {
		if _, err := url.Parse(cfg.DirectPostURL); err != nil {
			return fmt.Errorf("invalid nmi direct_post_url: %w", err)
		}
	}

	if cfg.QueryURL != "" {
		if _, err := url.Parse(cfg.QueryURL); err != nil {
			return fmt.Errorf("invalid nmi query_url: %w", err)
		}
	}

	if len(cfg.Providers) == 0 {
		return fmt.Errorf("at least one nmi provider must be configured")
	}

	for name, provider := range cfg.Providers {
		if provider == nil {
			return fmt.Errorf("nmi provider '%s' configuration is missing", name)
		}
		if provider.SecurityKey == "" && cfg.SecurityKey == "" {
			return fmt.Errorf("nmi provider '%s' security key is required", name)
		}
		if provider.WebhookSecret == "" && cfg.WebhookSecret == "" {
			return fmt.Errorf("nmi provider '%s' webhook secret is recommended for security", name)
		}
		if provider.DirectPostURL != "" {
			if _, err := url.Parse(provider.DirectPostURL); err != nil {
				return fmt.Errorf("invalid nmi provider '%s' direct_post_url: %w", name, err)
			}
		}
		if provider.QueryURL != "" {
			if _, err := url.Parse(provider.QueryURL); err != nil {
				return fmt.Errorf("invalid nmi provider '%s' query_url: %w", name, err)
			}
		}
	}

	return nil
}

// validateCCBill validates CCBill-specific configuration
func validateCCBill(cfg *CCBillConfig) error {
	if cfg == nil {
		return fmt.Errorf("ccbill configuration is required")
	}

	// Basic required fields
	if cfg.ClientAccNum == "" {
		return fmt.Errorf("ccbill client account number is required")
	}

	if cfg.ClientSubAcc == "" {
		return fmt.Errorf("ccbill client sub account is required")
	}

	if (cfg.DataLinkUsername == "") != (cfg.DataLinkPassword == "") {
		return fmt.Errorf("both datalink username and password must be provided when configuring DataLink access")
	}

	return nil
}

// assembleDBURL builds the database URL from atomic parameters if not explicitly set
func assembleDBURL(cfg *Config) {
	if cfg.DB == nil {
		return
	}

	// If URL is already explicitly set, nothing to do
	if cfg.DB.URL != "" {
		log.Debug("Using explicitly configured DB_URL")
		return
	}

	// Assemble URL from atomic parameters
	// All parameters have defaults from GetDefaultBillingConfig(), so they should all be present
	connStr := fmt.Sprintf(
		"postgresql://%s:%s@%s:%s/%s?sslmode=%s",
		cfg.DB.Username,
		cfg.DB.Password,
		cfg.DB.Host,
		cfg.DB.Port,
		cfg.DB.Database,
		cfg.DB.SSLMode,
	)

	cfg.DB.URL = connStr

	// Log warnings for critical default values being used
	warnings := []string{}
	if cfg.DB.Host == "localhost" {
		warnings = append(warnings, "DB host")
	}
	if cfg.DB.Username == "admin" {
		warnings = append(warnings, "DB username")
	}
	if cfg.DB.Password == "admin_password" {
		warnings = append(warnings, "DB password")
	}
	if cfg.DB.Database == "doujins_db" {
		warnings = append(warnings, "DB database name")
	}

	if len(warnings) > 0 {
		log.Warnf("Using default values for: %s. Assembled DB URL: %s",
			strings.Join(warnings, ", "), connStr)
	} else {
		log.Debugf("Assembled DB URL from configured parameters: %s", connStr)
	}
}

// validateDatabase validates database configuration
func validateDatabase(cfg *DBConfig) error {
	if cfg == nil {
		return fmt.Errorf("database configuration is required")
	}

	// Database is always PostgreSQL
	// After assembleDBURL, cfg.URL should always be set
	if cfg.URL == "" {
		return fmt.Errorf("database URL could not be determined")
	}

	return nil
}

// GetDefaultBillingConfig returns a billing configuration with sensible defaults
func GetDefaultBillingConfig() *Config {
	return &Config{
		Env:         "development",
		Host:        "0.0.0.0",
		Port:        2053,
		PrivatePort: 8060, // Private/service API port (internal only)
		DB: &DBConfig{
			Host:     "localhost",
			Port:     "5432",
			Database: "doujins_db",
			Username: "admin",
			Password: "admin_password",
			SSLMode:  "disable",
		},
		Redis: &RedisConfig{
			// Match docker-compose Garnet (service: garnet)
			Addr:     "garnet:6379",
			Password: "",
			DB:       0,
		},
		Auth: &AuthConfig{
			Issuers:          []string{"http://doujins:2052", "http://doujins:4000"}, // Accept tokens from both doujins and hentai0
			ExpectedAudience: "billing-app",
		},
		ClickHouse: &ClickHouseConfig{
			HTTPAddr:   "http://localhost:8123",
			ClientAddr: "localhost:9000",
			Database:   "analytics",
			Username:   "analytics_user",     // Match docker-compose CLICKHOUSE_USER
			Password:   "analytics_password", // Match docker-compose CLICKHOUSE_PASSWORD
			Cluster:    "doujins",            // Match docker-compose cluster name
		},
		Logger: &LoggerConfig{
			Level: "info", // Default to info level (options: debug, info, warn, error, fatal, panic)
		},
		RateLimits: &RateLimitsConfig{
			"subscribe": &RateLimit{
				RequestsPerMinute: 10, // Very restrictive for payment endpoints
				Burst:             3,
			},
			"checkout": &RateLimit{
				RequestsPerMinute: 5, // Heavy rate limiting for checkout - prevents abuse
				Burst:             2,
			},
			"webhook": &RateLimit{
				RequestsPerMinute: 100, // Higher for webhooks
				Burst:             20,
			},
			"payment": &RateLimit{
				RequestsPerMinute: 20,
				Burst:             5,
			},
			"default": &RateLimit{
				RequestsPerMinute: 60,
				Burst:             10,
			},
		},
		Solana: &SolanaConfig{},
	}
}

func loadConfigIfExists(k *koanf.Koanf, path string) error {
	if path == "" {
		return nil
	}
	candidates := []string{path}
	if !filepath.IsAbs(path) {
		candidates = append(candidates, filepath.Join("config", path))
		candidates = append(candidates, filepath.Join("./config", path))
	}
	visited := make(map[string]struct{})
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, ok := visited[candidate]; ok {
			continue
		}
		visited[candidate] = struct{}{}
		if _, err := os.Stat(candidate); err == nil {
			if err := k.Load(file.Provider(candidate), yaml.Parser()); err != nil {
				return fmt.Errorf("loading config file %s: %w", candidate, err)
			}
			return nil
		}
	}
	return nil
}

func Load(configPath string) (*Config, error) {
	k := koanf.New(".")

	// Start from sensible defaults so zero-config works in containers/compose.
	cfg := GetDefaultBillingConfig()

	if err := godotenv.Load(); err != nil {
		var pathErr *os.PathError
		if !errors.As(err, &pathErr) {
			return nil, err
		}
	}

	if configPath == "" {
		if envPath := strings.TrimSpace(os.Getenv("BILLING_CONFIG")); envPath != "" {
			configPath = envPath
		} else {
			configPath = "config.yaml"
		}
	}

	if err := loadConfigIfExists(k, configPath); err != nil {
		return nil, err
	}

	// Load environment variables using koanf's env provider
	// Transform env var names to config keys (e.g., DB_URL -> db.url)
	envCallback := func(s string) string {
		s = strings.ToLower(s)

		// Special case: ENVIRONMENT -> env
		if s == "environment" {
			return "env"
		}

		// Special case: API_URL -> api_url (top-level, not nested api.url)
		if s == "api_url" {
			return "api_url"
		}

		// Special case: BILLING_TEST_MODE or TEST_MODE -> test_mode (top-level)
		if s == "billing_test_mode" || s == "test_mode" {
			return "test_mode"
		}

		// NEW: PROCESSORS_<NAME>_<FIELD> -> processors.<name>.<field>
		// Example: PROCESSORS_MOBIUS_SECURITY_KEY -> processors.mobius.security_key
		// Example: PROCESSORS_CCBILL_CLIENT_ACC_NUM -> processors.ccbill.client_acc_num
		if strings.HasPrefix(s, "processors_") {
			parts := strings.SplitN(s, "_", 3) // ["processors", "mobius", "security_key"]
			if len(parts) == 3 {
				return fmt.Sprintf("processors.%s.%s", parts[1], parts[2])
			}
		}

		// LEGACY: NMI_PROVIDERS_<PROVIDER>_<KEY> -> nmi.providers.<provider>.<key>
		// Example: NMI_PROVIDERS_MOBIUS_SECURITY_KEY -> nmi.providers.mobius.security_key
		// Deprecated: Use PROCESSORS_<NAME>_<FIELD> instead
		if strings.HasPrefix(s, "nmi_providers_") {
			parts := strings.SplitN(s, "_", 4) // ["nmi", "providers", "mobius", "security_key"]
			if len(parts) == 4 {
				return fmt.Sprintf("nmi.providers.%s.%s", parts[2], parts[3])
			}
		}

		// Standard transformation: Replace first underscore with dot
		// DB_URL -> db.url
		// REDIS_ADDR -> redis.addr
		// CLICKHOUSE_HTTP_ADDR -> clickhouse.http_addr
		// CCBILL_SALT -> ccbill.salt
		if !strings.Contains(s, "_") {
			return s // No underscore, return as-is
		}
		return strings.Replace(s, "_", ".", 1)
	}

	if err := k.Load(env.Provider("", ".", envCallback), nil); err != nil {
		return nil, fmt.Errorf("loading environment variables: %w", err)
	}

	// Unmarshal into config struct (overlay onto defaults)
	if err := k.Unmarshal("", cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	// Store defaults (used across the system for branding)
	if cfg.Store == nil {
		cfg.Store = &StoreConfig{}
	}
	if cfg.Store.Name == "" {
		cfg.Store.Name = "My Store"
	}
	if cfg.Store.LogoURL == "" {
		cfg.Store.LogoURL = DefaultLogoURL
	}

	if cfg.Solana == nil {
		cfg.Solana = &SolanaConfig{}
	}
	// Derive Solana network from test_mode (ignores any explicit network setting)
	// test_mode=true → devnet, test_mode=false → mainnet
	if cfg.IsTestMode() {
		cfg.Solana.Network = "devnet"
	} else {
		cfg.Solana.Network = "mainnet"
	}
	if len(cfg.Solana.SupportedTokens) == 0 {
		cfg.Solana.SupportedTokens = TokensForNetwork(cfg.Solana.Network)
	}

	// Warn if Solana is configured but api_url is missing (needed for Solana Pay URLs)
	if cfg.Solana.RecipientWallet != "" && strings.TrimSpace(cfg.APIURL) == "" {
		log.Warn("Solana is configured but api_url is not set; Solana Pay transaction_request URLs will not be generated. " +
			"Set API_URL to your public API endpoint (e.g., https://api.mysite.com or https://api.mysite.com/billing for embedded mode)")
	}

	// Initialize and normalize Processors map
	if cfg.Processors == nil {
		cfg.Processors = make(map[string]*ProcessorConfig)
	}
	if len(cfg.Processors) > 0 {
		normalized := make(map[string]*ProcessorConfig, len(cfg.Processors))
		for name, proc := range cfg.Processors {
			key := strings.TrimSpace(strings.ToLower(name))
			if key == "" {
				log.Warnf("ignoring processor with empty name (original key: %q)", name)
				continue
			}
			if proc == nil {
				log.Warnf("ignoring processor '%s' with nil config", key)
				continue
			}

			if existing, exists := normalized[key]; exists && existing != nil {
				log.Warnf("duplicate processor configuration detected for key '%s'; overriding previous value", key)
			}

			// Validate type for non-reserved names
			effectiveType := proc.GetEffectiveType(key)
			if effectiveType == "" {
				log.Warnf("processor '%s' has no type specified and is not a reserved name (ccbill, stripe, solana); assuming 'nmi'", key)
				proc.Type = ProcessorTypeNMI
			}

			// Warn if reserved name has conflicting type
			if impliedType, isReserved := ReservedProcessorNames[key]; isReserved && proc.Type != "" && proc.Type != impliedType {
				log.Warnf("processor '%s' has type '%s' but '%s' is a reserved name implying type '%s'; using implied type",
					key, proc.Type, key, impliedType)
				proc.Type = impliedType
			}

			normalized[key] = proc
		}
		cfg.Processors = normalized
	}

	// Post-process Solana config from Processors map
	if solanaProc, exists := cfg.Processors["solana"]; exists && solanaProc != nil {
		// If enabled_tokens came from env var as a string, it might need parsing
		// koanf doesn't automatically split comma-separated values for slices
		if len(solanaProc.EnabledTokens) == 1 && strings.Contains(solanaProc.EnabledTokens[0], ",") {
			// Split the single comma-separated string into multiple tokens
			tokens := strings.Split(solanaProc.EnabledTokens[0], ",")
			solanaProc.EnabledTokens = make([]string, 0, len(tokens))
			for _, t := range tokens {
				t = strings.TrimSpace(t)
				if t != "" {
					solanaProc.EnabledTokens = append(solanaProc.EnabledTokens, t)
				}
			}
		}
	}
	// Also handle SolanaConfig's EnabledTokens (for legacy config path)
	if cfg.Solana != nil && len(cfg.Solana.EnabledTokens) == 1 && strings.Contains(cfg.Solana.EnabledTokens[0], ",") {
		tokens := strings.Split(cfg.Solana.EnabledTokens[0], ",")
		cfg.Solana.EnabledTokens = make([]string, 0, len(tokens))
		for _, t := range tokens {
			t = strings.TrimSpace(t)
			if t != "" {
				cfg.Solana.EnabledTokens = append(cfg.Solana.EnabledTokens, t)
			}
		}
	}

	// LEGACY: Initialize NMI config for backwards compatibility
	if cfg.NMI == nil {
		cfg.NMI = &NMIConfig{}
	}
	if cfg.NMI.Providers == nil {
		cfg.NMI.Providers = make(map[string]*NMIProviderConfig)
	}
	if len(cfg.NMI.Providers) > 0 {
		normalized := make(map[string]*NMIProviderConfig, len(cfg.NMI.Providers))
		for name, provider := range cfg.NMI.Providers {
			key := strings.TrimSpace(strings.ToLower(name))
			if key == "" {
				log.Warnf("ignoring NMI provider with empty name (original key: %q)", name)
				continue
			}

			if existing, exists := normalized[key]; exists && existing != nil {
				log.Warnf("duplicate NMI provider configuration detected for key '%s'; overriding previous value", key)
			}

			normalized[key] = provider
		}
		cfg.NMI.Providers = normalized

		// Log deprecation warning if using legacy NMI config
		log.Warn("Using deprecated nmi.providers config format. Migrate to processors map format (see config.example.yaml)")
	}

	// Assemble DB URL from pieces if not explicitly set
	assembleDBURL(cfg)

	// Log test mode status clearly at startup
	logTestModeStatus(cfg)

	// Validate the loaded configuration
	if err := Validate(cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// logTestModeStatus logs the payment processing mode at startup.
// This helps operators confirm whether they're in test or production mode.
func logTestModeStatus(cfg *Config) {
	if cfg.IsTestMode() {
		log.Warn("⚠️  TEST MODE ENABLED - No real charges will be processed")
		log.Info("   Payment providers will use sandbox/test environments:")
		log.Info("   - NMI: sandbox.nmi.com")
		log.Info("   - CCBill: sandbox-api.ccbill.com")
		log.Info("   - Stripe: requires sk_test_* key")
		log.Info("   - Solana: devnet")
	} else {
		log.Warn("🔴 PRODUCTION MODE - Real charges enabled")
		log.Info("   Payment providers will use production environments")

		// Warn if running real charges in dev environment (unusual)
		if cfg.IsDev() {
			log.Warn("⚠️  Real payment processing enabled in dev environment - this is unusual")
			log.Warn("   Set test_mode=true or BILLING_TEST_MODE=true to use sandbox environments")
		}
	}
}
