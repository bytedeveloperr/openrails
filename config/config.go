package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	log "github.com/sirupsen/logrus"
)

const EnvProd string = "prod"
const EnvDev string = "dev"

const ConfigContextKey string = "config"

type Config struct {
	Env         string            `koanf:"env,omitempty"`
	Port        int16             `koanf:"port,omitempty"`
	Host        string            `koanf:"host,omitempty"`
	NMI         *NMIConfig        `koanf:"nmi,omitempty"`
	CCBill      *CCBillConfig     `koanf:"ccbill,omitempty"`
	Solana      *SolanaConfig     `koanf:"solana,omitempty"`
	DB          *DBConfig         `koanf:"db,omitempty"`
	Redis       *RedisConfig      `koanf:"redis,omitempty"`
	Auth        *AuthConfig       `koanf:"auth,omitempty"`
	ClickHouse  *ClickHouseConfig `koanf:"clickhouse,omitempty"`
	SendGrid    *SendGridConfig   `koanf:"sendgrid,omitempty"`
	CorsOrigins []string          `koanf:"cors_origins,omitempty"`
	RateLimits  *RateLimitConfig  `koanf:"rate_limits,omitempty"`
	Admin       *AdminConfig      `koanf:"admin,omitempty"`
	TLS         *TLSConfig        `koanf:"tls,omitempty"`
}

// DBConfig holds database configuration.
// Supports both legacy connection string (URL) and atomic parameters.
// If URL is provided, it takes precedence. Otherwise, connection string
// is built from individual parameters (Host, Port, Username, etc.).
type DBConfig struct {
	// Legacy: Full connection string (optional)
	URL string `koanf:"url"`

	// Atomic parameters (preferred for template-based configuration)
	Host     string `koanf:"host"`
	Port     string `koanf:"port"`
	Database string `koanf:"database"`
	Username string `koanf:"username"`
	Password string `koanf:"password"`
	Schema   string `koanf:"schema"`
	SSLMode  string `koanf:"sslmode"`

	Dialect string `koanf:"dialect"`
}

// GetConnectionString returns the database connection string.
// If URL is set, returns it directly. Otherwise, builds the connection
// string from atomic parameters.
func (c *DBConfig) GetConnectionString() string {
	// If legacy URL is provided, use it
	if c.URL != "" {
		return c.URL
	}

	// Build connection string from atomic parameters
	// Format: postgresql://username:password@host:port/database?sslmode=...&search_path=...
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
	if c.SSLMode != "" {
		params = append(params, fmt.Sprintf("sslmode=%s", c.SSLMode))
	}
	if c.Schema != "" {
		params = append(params, fmt.Sprintf("search_path=%s", c.Schema))
	}

	if len(params) > 0 {
		connStr += "?" + strings.Join(params, "&")
	}

	return connStr
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
	Language           string `koanf:"language"`
	FormID             string `koanf:"form_id"`
	FormName           string `koanf:"form_name"`
	CurrencyCode       string `koanf:"currency_code"`
	ClientSubAcc       string `koanf:"client_sub_acc"`
	ClientAccNum       string `koanf:"client_acc_num"`
	SubscriptionTypeId string `koanf:"subscription_type_id"`
	TestMode           bool   `koanf:"test_mode"`

	// Webhook secret (optional; typically IP + salt verification is used)
	WebhookSecret string `koanf:"webhook_secret"`

	// FlexForm integration settings
	BaseFlexFormURL string `koanf:"base_flexform_url"`
	IFrameWidth     string `koanf:"iframe_width"`  // Default: "100%"
	IFrameHeight    string `koanf:"iframe_height"` // Default: "600px"

	// Optional success/decline URLs for post-payment navigation
	SuccessURL string `koanf:"success_url"`
	DeclineURL string `koanf:"decline_url"`

	DataLinkURL          string `koanf:"datalink_url"`
	DataLinkUsername     string `koanf:"datalink_username"`
	DataLinkPassword     string `koanf:"datalink_password"`
	DataLinkClientAccNum string `koanf:"datalink_client_acc_num"`

	WebhookIPs []string `koanf:"webhook_ips"`
}

type RedisConfig struct {
	Addr     string `koanf:"host"`
	Password string `koanf:"password"`
	DB       int    `koanf:"db"`
}

// AuthConfig holds JWT verification configuration for billing service.
// Billing is a JWT verifier (not issuer) - it validates tokens issued by doujins/hentai0.
type AuthConfig struct {
	Issuer   string `koanf:"issuer"`   // Expected token issuer (e.g., "https://doujins.com")
	Audience string `koanf:"audience"` // Expected audience claim (e.g., "billing-app")
	BaseURL  string `koanf:"base_url"` // Base URL for JWKS endpoint (defaults to issuer if empty)
	// Optional RSA public key PEM for verifying RS256 JWTs. If empty,
	// the service fetches JWKS from "{base_url}/.well-known/jwks.json" or "{issuer}/.well-known/jwks.json".
	PublicKeyPEM         string `koanf:"public_key_pem"`
	SkipExpiryValidation bool   `koanf:"skip_expiry_validation"`
}

// GetJWKSURL returns the JWKS URL for fetching public keys.
// Uses base_url if provided, otherwise falls back to issuer.
func (a *AuthConfig) GetJWKSURL() string {
	base := strings.TrimSpace(a.BaseURL)
	if base == "" {
		base = strings.TrimSpace(a.Issuer)
	}
	base = strings.TrimRight(base, "/")
	return base + "/.well-known/jwks.json"
}

type SolanaConfig struct {
	RPCEndpoint     string `koanf:"rpc_endpoint"`
	Network         string `koanf:"network"` // mainnet, devnet, testnet
	RecipientWallet string `koanf:"recipient_wallet"`

	SupportedTokens map[string]TokenConfig `koanf:"supported_tokens,omitempty"`

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
	Price       float64 `json:"price" koanf:"price"`       // Price in USD (for display)
	Enabled     bool    `json:"enabled" koanf:"enabled"`   // Whether this token is enabled
	MainnetMint string  `json:"mainnet_mint,omitempty" koanf:"mainnet_mint"`
}

// RateLimitConfig defines rate limiting for billing endpoints
type RateLimitConfig struct {
	SubscribeLimit *RateLimit `koanf:"subscribe_limit,omitempty"` // POST /subscriptions/*
	WebhookLimit   *RateLimit `koanf:"webhook_limit,omitempty"`   // POST /webhooks/*
	PaymentLimit   *RateLimit `koanf:"payment_limit,omitempty"`   // Payment method operations
	DefaultLimit   *RateLimit `koanf:"default_limit,omitempty"`   // Default for other endpoints
}

// SendGridConfig holds SendGrid email configuration
type SendGridConfig struct {
	APIKey    string `koanf:"api_key"`
	FromEmail string `koanf:"from_email"`
	FromName  string `koanf:"from_name"`
}

type ClickHouseConfig struct {
	HTTPAddr   string `koanf:"http_addr"`   // Full HTTP address, e.g., http://clickhouse:8123
	ClientAddr string `koanf:"client_addr"` // Native client address, e.g., clickhouse:9000
	Database   string `koanf:"database"`    // ClickHouse database name (e.g., analytics)
	Username   string `koanf:"username"`    // Optional username for authentication
	Password   string `koanf:"password"`    // Optional password for authentication
}

// AdminConfig controls private admin access
type AdminConfig struct {
	// Shared API key required in 'X-API-Key' header for admin routes
	APIKey string `koanf:"api_key"`
}

// TLSConfig controls optional private mTLS listener
type TLSConfig struct {
	Private *PrivateTLSConfig `koanf:"private"`
}

type PrivateTLSConfig struct {
	Enabled           bool   `koanf:"enabled"`
	Addr              string `koanf:"addr"` // default ":8060"
	CertFile          string `koanf:"cert_file"`
	KeyFile           string `koanf:"key_file"`
	ClientCAFile      string `koanf:"client_ca_file"`      // optional client CA
	RequireClientCert bool   `koanf:"require_client_cert"` // enable mTLS if true and ClientCAFile provided
}

// RateLimit defines a rate limit policy
type RateLimit struct {
	RequestsPerMinute int `koanf:"requests_per_minute"`
	BurstSize         int `koanf:"burst_size"`
}

// Validate validates the billing configuration
func Validate(cfg *Config) error {
	// Skip strict validation in development environments
	isDev := cfg.Env == "development" || cfg.Env == "dev" || cfg.Env == ""

	if !isDev {
		if err := validateNMI(cfg.NMI); err != nil {
			return fmt.Errorf("nmi config validation failed: %w", err)
		}

		// Validate CCBill configuration
		if err := validateCCBill(cfg.CCBill); err != nil {
			return fmt.Errorf("ccbill config validation failed: %w", err)
		}
	}

	// Always validate database configuration
	if err := validateDatabase(cfg.DB); err != nil {
		return fmt.Errorf("database config validation failed: %w", err)
	}

	return nil
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

	if cfg.FormName == "" && cfg.FormID == "" {
		return fmt.Errorf("ccbill form name or form ID is required")
	}

	// Validate FlexForm URL
	if cfg.BaseFlexFormURL != "" {
		if _, err := url.Parse(cfg.BaseFlexFormURL); err != nil {
			return fmt.Errorf("invalid ccbill base flexform URL: %w", err)
		}
	}

	// Validate success/decline URLs if provided
	if cfg.SuccessURL != "" {
		if _, err := url.Parse(cfg.SuccessURL); err != nil {
			return fmt.Errorf("invalid ccbill success URL: %w", err)
		}
	}
	if cfg.DeclineURL != "" {
		if _, err := url.Parse(cfg.DeclineURL); err != nil {
			return fmt.Errorf("invalid ccbill decline URL: %w", err)
		}
	}

	// Validate DataLink configuration if provided
	if cfg.DataLinkURL != "" {
		if _, err := url.Parse(cfg.DataLinkURL); err != nil {
			return fmt.Errorf("invalid ccbill datalink URL: %w", err)
		}

		if cfg.DataLinkUsername == "" || cfg.DataLinkPassword == "" {
			return fmt.Errorf("datalink username and password are required when datalink URL is provided")
		}

		if cfg.DataLinkClientAccNum == "" {
			return fmt.Errorf("datalink client account number is required when datalink URL is provided")
		}
	}

	return nil
}

// validateDatabase validates database configuration
func validateDatabase(cfg *DBConfig) error {
	if cfg == nil {
		return fmt.Errorf("database configuration is required")
	}

	if cfg.GetConnectionString() == "" {
		return fmt.Errorf("database configuration is required (DB_URL or DB_HOST/DB_PORT/etc.)")
	}

	return nil
}

// GetDefaultBillingConfig returns a billing configuration with sensible defaults
func GetDefaultBillingConfig() *Config {
	return &Config{
		Env:  "development",
		Host: "0.0.0.0",
		Port: 2053,
		DB: &DBConfig{
			// Defaults align with docker-compose: Postgres service is `postgres` on the compose network.
			// Developers running the binary on the host can override via DB_URL.
			URL:     "postgres://billing_app:billing_password@postgres:5432/doujins_db?sslmode=disable",
			Schema:  "billing",
			Dialect: "postgres",
		},
		Redis: &RedisConfig{
			// Match docker-compose Garnet (service: garnet)
			Addr:     "garnet:6379",
			Password: "",
			DB:       0,
		},
		Auth: &AuthConfig{
			Issuer:   "http://api:2052",
			Audience: "billing-app",
			BaseURL:  "http://api:2052",
		},
		// Match docker-compose ClickHouse (service: clickhouse)
		ClickHouse: &ClickHouseConfig{
			HTTPAddr:   "http://clickhouse:8123",
			ClientAddr: "clickhouse:9000",
			Database:   "analytics",
			Username:   "analytics_user",
			Password:   "analytics_password",
		},
		Admin: &AdminConfig{
			// Default internal admin API key for development. Override via env BILLING_INTERNAL_API_KEY in prod.
			APIKey: "change-me-in-dev",
		},
		TLS: &TLSConfig{
			Private: &PrivateTLSConfig{
				Enabled: false,
				Addr:    ":8060",
			},
		},
		RateLimits: &RateLimitConfig{
			SubscribeLimit: &RateLimit{
				RequestsPerMinute: 10, // Very restrictive for payment endpoints
				BurstSize:         3,
			},
			WebhookLimit: &RateLimit{
				RequestsPerMinute: 100, // Higher for webhooks
				BurstSize:         20,
			},
			PaymentLimit: &RateLimit{
				RequestsPerMinute: 20,
				BurstSize:         5,
			},
			DefaultLimit: &RateLimit{
				RequestsPerMinute: 60,
				BurstSize:         10,
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

	// Load environment variables with prefix
	if err := k.Load(env.Provider("BILLING_", ".", func(s string) string {
		// Convert BILLING_DATABASE_URL to database.url
		s = strings.ToLower(strings.TrimPrefix(s, "BILLING_"))
		s = strings.ReplaceAll(s, "_", ".")
		return s
	}), nil); err != nil {
		return nil, fmt.Errorf("loading environment variables: %w", err)
	}

	// Load common environment variables without prefix
	envMappings := map[string]string{
		"DB_URL":      "db.url",
		"REDIS_URL":   "redis.host",
		"ENV":         "env",
		"ENVIRONMENT": "env",

		// Auth / JWT verification
		"AUTH_ISSUER":         "auth.issuer",
		"AUTH_AUDIENCE":       "auth.audience",
		"AUTH_BASE_URL":       "auth.base_url",
		"AUTH_PUBLIC_KEY_PEM": "auth.public_key_pem", // optional for RS256 if not using JWKS

		// CCBill
		"CCBILL_CLIENT_ACCOUNT":       "ccbill.client_acc_num",
		"CCBILL_CLIENT_SUBACCOUNT":    "ccbill.client_sub_acc",
		"CCBILL_SALT":                 "ccbill.salt",
		"CCBILL_FORM_ID":              "ccbill.form_id",
		"CCBILL_FLEXFORM_ID":          "ccbill.form_id",
		"CCBILL_FORM_NAME":            "ccbill.form_name",
		"CCBILL_LANGUAGE":             "ccbill.language",
		"CCBILL_CURRENCY_CODE":        "ccbill.currency_code",
		"CCBILL_SUBSCRIPTION_TYPE_ID": "ccbill.subscription_type_id",
		"CCBILL_TEST_MODE":            "ccbill.test_mode",
		"CCBILL_WEBHOOK_SECRET":       "ccbill.webhook_secret",
		"CCBILL_DATALINK_URL":         "ccbill.datalink_url",
		"CCBILL_BASE_FLEXFORM_URL":    "ccbill.base_flexform_url",
		"CCBILL_SUCCESS_URL":          "ccbill.success_url",
		"CCBILL_DECLINE_URL":          "ccbill.decline_url",

		// NMI
		"NMI_SECURITY_KEY":        "nmi.security_key",
		"NMI_TOKENIZATION_KEY":    "nmi.tokenization_key",
		"NMI_WEBHOOK_SECRET":      "nmi.webhook_secret",
		"NMI_TEST_MODE":           "nmi.test_mode",
		"NMI_DIRECT_POST_URL":     "nmi.direct_post_url",
		"NMI_QUERY_URL":           "nmi.query_url",
		"MOBIUS_SECURITY_KEY":     "nmi.security_key",
		"MOBIUS_TOKENIZATION_KEY": "nmi.tokenization_key",
		"MOBIUS_WEBHOOK_SECRET":   "nmi.webhook_secret",
		"MOBIUS_TEST_MODE":        "nmi.test_mode",
		"MOBIUS_DIRECT_POST_URL":  "nmi.direct_post_url",
		"MOBIUS_QUERY_URL":        "nmi.query_url",

		// SendGrid email configuration
		"SENDGRID_API_KEY":    "sendgrid.api_key",
		"SENDGRID_FROM_EMAIL": "sendgrid.from_email",
		"SENDGRID_FROM_NAME":  "sendgrid.from_name",

		// ClickHouse
		"CLICKHOUSE_HTTP_ADDR":   "clickhouse.http_addr",
		"CLICKHOUSE_CLIENT_ADDR": "clickhouse.client_addr",
		"CLICKHOUSE_DATABASE":    "clickhouse.database",
		"CLICKHOUSE_USERNAME":    "clickhouse.username",
		"CLICKHOUSE_PASSWORD":    "clickhouse.password",

		// Schema override (compose now uses DB_SCHEMA; keep APP_SCHEMA as legacy)
		"DB_SCHEMA":  "db.schema",
		"APP_SCHEMA": "db.schema",

		// Solana
		"SOLANA_RPC_ENDPOINT":       "solana.rpc_endpoint",
		"SOLANA_NETWORK":            "solana.network",
		"SOLANA_RECIPIENT_WALLET":   "solana.recipient_wallet",
		"SOLANA_DESTINATION_WALLET": "solana.destination_wallet",

		// Admin API key
		"BILLING_INTERNAL_API_KEY": "admin.api_key",
	}

	for envVar, configKey := range envMappings {
		if val := os.Getenv(envVar); val != "" {
			k.Set(configKey, val)
		}
	}

	// Parse Redis URL if provided
	if redisURL := os.Getenv("REDIS_URL"); redisURL != "" {
		if parsedURL, err := url.Parse(redisURL); err == nil {
			k.Set("redis.host", parsedURL.Host)
			if parsedURL.User != nil {
				if password, ok := parsedURL.User.Password(); ok {
					k.Set("redis.password", password)
				}
			}
			// Extract database number from path
			if len(parsedURL.Path) > 1 {
				if db := strings.TrimPrefix(parsedURL.Path, "/"); db != "" {
					k.Set("redis.db", db)
				}
			}
		}
	}

	// Unmarshal into config struct (overlay onto defaults)
	if err := k.Unmarshal("", cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	if cfg.Solana == nil {
		cfg.Solana = &SolanaConfig{}
	}
	if cfg.Solana.Network == "" {
		cfg.Solana.Network = "mainnet"
	}
	if len(cfg.Solana.SupportedTokens) == 0 {
		cfg.Solana.SupportedTokens = TokensForNetwork(cfg.Solana.Network)
	}

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
	}

	// Validate the loaded configuration
	if err := Validate(cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}
