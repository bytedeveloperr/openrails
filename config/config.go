package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

const EnvProd string = "prod"
const EnvDev string = "dev"

const ConfigContextKey string = "config"

type Config struct {
	Env         string            `json:"env,omitempty"`
	Port        int16             `json:"port,omitempty"`
	Host        string            `json:"host,omitempty"`
	Mobius      *MobiusConfig     `json:"mobius,omitempty"`
	CCBill      *CCBillConfig     `json:"ccbill,omitempty"`
	Solana      *SolanaConfig     `json:"solana,omitempty"`
	DB          *DBConfig         `json:"db,omitempty"`
	ExternalDB  *DBConfig         `json:"external_db,omitempty"` // Client app database for role management
	Redis       *RedisConfig      `json:"redis,omitempty"`
	JWT         *JWTConfig        `json:"jwt,omitempty"`
	ClickHouse  *ClickHouseConfig `json:"clickhouse,omitempty"`
	SendGrid    *SendGridConfig   `json:"sendgrid,omitempty"`
	CorsOrigins []string          `json:"cors_origins,omitempty"`
	RateLimits  *RateLimitConfig  `json:"rate_limits,omitempty"`
	Admin       *AdminConfig      `json:"admin,omitempty"`
	TLS         *TLSConfig        `json:"tls,omitempty"`
}

type DBConfig struct {
	URL     string `koanf:"url"`
	Schema  string `koanf:"schema"`
	Dialect string `koanf:"dialect"`
}

type MobiusConfig struct {
	SecurityKey     string `koanf:"security_key"`
	TokenizationKey string `koanf:"tokenization_key"`
	WebhookSecret   string `koanf:"webhook_secret"`
	TestMode        bool   `koanf:"test_mode"`
}

type CCBillConfig struct {
	Salt               string `koanf:"salt"`
	Language           string `koanf:"language"`
	FormID             string `koanf:"form_id"`
	FormName           string `koanf:"form_name"`
	CurrencyCode       string `koanf:"currency_code"`
	AllowedTypes       string `koanf:"allowed_types"`
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

type JWTConfig struct {
	Secret string `koanf:"secret"`
	Issuer string `koanf:"issuer"`
	// Audience (client ID) to require in the "aud" claim (e.g., Casdoor Application Client ID)
	Audience string `koanf:"audience"`
	// Optional RSA public key PEM for verifying RS256 JWTs. If empty and Issuer is set,
	// the service will attempt OIDC discovery at "{issuer}/.well-known/openid-configuration"
	// and use JWKS for verification.
	PublicKeyPEM string `koanf:"public_key_pem"`
}

type SolanaConfig struct {
	RPCEndpoint       string `json:"rpc_endpoint"`
	Network           string `json:"network"` // mainnet, devnet, testnet
	RecipientWallet   string `json:"recipient_wallet"`
	DestinationWallet string `json:"destination_wallet"` // Alias for RecipientWallet

	SupportedTokens map[string]TokenConfig `json:"supported_tokens,omitempty"`

	TransactionTimeoutSeconds int     `json:"transaction_timeout_seconds,omitempty"`
	ConfirmationBlocks        int     `json:"confirmation_blocks,omitempty"`
	MaxTransactionFee         float64 `json:"max_transaction_fee,omitempty"`
}

// TokenConfig defines configuration for a specific Solana token
type TokenConfig struct {
	Mint     string  `json:"mint"`     // Token mint address
	Symbol   string  `json:"symbol"`   // Token symbol (e.g., "SOL", "USDC")
	Name     string  `json:"name"`     // Token name
	Decimals int     `json:"decimals"` // Token decimal places
	Price    float64 `json:"price"`    // Price in USD (for display)
	Enabled  bool    `json:"enabled"`  // Whether this token is enabled
}

// RateLimitConfig defines rate limiting for billing endpoints
type RateLimitConfig struct {
	SubscribeLimit *RateLimit `json:"subscribe_limit,omitempty"` // POST /subscriptions/*
	WebhookLimit   *RateLimit `json:"webhook_limit,omitempty"`   // POST /webhooks/*
	PaymentLimit   *RateLimit `json:"payment_limit,omitempty"`   // Payment method operations
	DefaultLimit   *RateLimit `json:"default_limit,omitempty"`   // Default for other endpoints
}

type ClickHouseConfig struct {
	ServerURL string `koanf:"server_url"` // HTTP URL for ClickHouse server (e.g., http://localhost:8123)
	Database  string `koanf:"database"`   // ClickHouse database name (e.g., analytics)
	Username  string `koanf:"username"`   // Optional username for authentication
	Password  string `koanf:"password"`   // Optional password for authentication
}

// FeatureFlags control optional integrations and startup gating
// (No feature flags; Redis and ClickHouse behavior is dynamic at runtime.)

type SendGridConfig struct {
	APIKey    string `koanf:"api_key"`
	FromEmail string `koanf:"from_email"`
	FromName  string `koanf:"from_name"`
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
	RequestsPerMinute int `json:"requests_per_minute"`
	BurstSize         int `json:"burst_size"`
}

// Validate validates the billing configuration
func Validate(cfg *Config) error {
	// Skip strict validation in development environments
	isDev := cfg.Env == "development" || cfg.Env == "dev" || cfg.Env == ""

	if !isDev {
		if err := validateMobius(cfg.Mobius); err != nil {
			return fmt.Errorf("mobius config validation failed: %w", err)
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

// validateMobius validates Mobius-specific configuration
func validateMobius(cfg *MobiusConfig) error {
	if cfg == nil {
		return fmt.Errorf("mobius configuration is required")
	}

	if cfg.SecurityKey == "" {
		return fmt.Errorf("mobius security key is required in production")
	}

	// TokenizationKey is not required by the billing service; frontend integrates
	// with Mobius Collect.js directly. Keep optional for backward compatibility.

	if cfg.WebhookSecret == "" {
		return fmt.Errorf("mobius webhook secret is recommended for security")
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

	if cfg.URL == "" {
		return fmt.Errorf("database URL is required")
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
			// Match docker-compose Postgres (service: postgres)
			URL:     "postgres://app_user:app_password@postgres:5432/doujins_db?sslmode=disable",
			Schema:  "billing",
			Dialect: "postgres",
		},
		Redis: &RedisConfig{
			// Match docker-compose Garnet (service: garnet)
			Addr:     "garnet:6379",
			Password: "",
			DB:       0,
		},
		JWT: &JWTConfig{
			Secret:   "", // RS256 by default; no shared secret
			Issuer:   "http://casdoor:8010",
			Audience: "doujins-app",
		},
		// Match docker-compose ClickHouse (service: clickhouse)
		ClickHouse: &ClickHouseConfig{
			ServerURL: "http://clickhouse:8123",
			Database:  "analytics",
			Username:  "analytics_user",
			Password:  "analytics_password",
		},
		Admin: &AdminConfig{
			APIKey: "", // Provide via env BILLING_INTERNAL_API_KEY
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
		Solana: &SolanaConfig{
			Network:                   "mainnet",
			TransactionTimeoutSeconds: 300, // 5 minutes
			ConfirmationBlocks:        1,
			MaxTransactionFee:         0.01, // SOL
			SupportedTokens: map[string]TokenConfig{
				"SOL": {
					Symbol:   "SOL",
					Name:     "Solana",
					Decimals: 9,
					Enabled:  true,
				},
				"USDC": {
					Symbol:   "USDC",
					Name:     "USD Coin",
					Decimals: 6,
					Enabled:  true,
				},
				"PYUSD": {
					Symbol:   "PYUSD",
					Name:     "PayPal USD",
					Decimals: 6,
					Enabled:  true,
				},
			},
		},
	}
}

func Load(configPath string) (*Config, error) {
	k := koanf.New(".")

	// Start with default configuration
	cfg := GetDefaultBillingConfig()

	// Determine config file path

	if configPath == "" {
		// Look for config.yaml in current directory and ./config/
		candidates := []string{
			"config.yaml",
			"config/config.yaml",
			"./config.yaml",
			"./config/config.yaml",
		}

		for _, candidate := range candidates {
			if _, err := os.Stat(candidate); err == nil {
				configPath = candidate
				break
			}
		}
	}

	// Load from YAML file if it exists
	if configPath != "" {
		if _, err := os.Stat(configPath); err == nil {
			if err := k.Load(file.Provider(configPath), yaml.Parser()); err != nil {
				return nil, fmt.Errorf("loading config file %s: %w", configPath, err)
			}
		}
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
		"DATABASE_URL": "db.url",
		"REDIS_URL":    "redis.host",
		"ENV":          "env",
		"ENVIRONMENT":  "env",

		// JWT / Casdoor
		"JWT_SECRET":         "jwt.secret",         // for HS256
		"JWT_ISSUER":         "jwt.issuer",         // legacy name
		"CASDOOR_SERVER_URL": "jwt.issuer",         // preferred name
		"JWT_AUDIENCE":       "jwt.audience",       // legacy
		"JWT_CLIENT_ID":      "jwt.audience",       // legacy
		"CASDOOR_CLIENT_ID":  "jwt.audience",       // preferred
		"JWT_PUBLIC_KEY_PEM": "jwt.public_key_pem", // optional for RS256 if not using JWKS

		// CCBill
		"CCBILL_CLIENT_ACCOUNT":       "ccbill.client_acc_num",
		"CCBILL_CLIENT_SUBACCOUNT":    "ccbill.client_sub_acc",
		"CCBILL_SALT":                 "ccbill.salt",
		"CCBILL_FORM_ID":              "ccbill.form_id",
		"CCBILL_FLEXFORM_ID":          "ccbill.form_id",
		"CCBILL_FORM_NAME":            "ccbill.form_name",
		"CCBILL_LANGUAGE":             "ccbill.language",
		"CCBILL_CURRENCY_CODE":        "ccbill.currency_code",
		"CCBILL_ALLOWED_TYPES":        "ccbill.allowed_types",
		"CCBILL_SUBSCRIPTION_TYPE_ID": "ccbill.subscription_type_id",
		"CCBILL_TEST_MODE":            "ccbill.test_mode",
		"CCBILL_WEBHOOK_SECRET":       "ccbill.webhook_secret",
		"CCBILL_DATALINK_URL":         "ccbill.datalink_url",
		"CCBILL_BASE_FLEXFORM_URL":    "ccbill.base_flexform_url",
		"CCBILL_SUCCESS_URL":          "ccbill.success_url",
		"CCBILL_DECLINE_URL":          "ccbill.decline_url",

		// Mobius
		"MOBIUS_SECURITY_KEY":     "mobius.security_key",
		"MOBIUS_TOKENIZATION_KEY": "mobius.tokenization_key",
		"MOBIUS_WEBHOOK_SECRET":   "mobius.webhook_secret",
		"MOBIUS_TEST_MODE":        "mobius.test_mode",

		// SendGrid
		"SENDGRID_API_KEY":    "sendgrid.api_key",
		"SENDGRID_FROM_EMAIL": "sendgrid.from_email",
		"SENDGRID_FROM_NAME":  "sendgrid.from_name",

		// ClickHouse
		"CLICKHOUSE_URL":      "clickhouse.server_url",
		"CLICKHOUSE_DATABASE": "clickhouse.database",
		"CLICKHOUSE_USERNAME": "clickhouse.username",
		"CLICKHOUSE_PASSWORD": "clickhouse.password",

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

	// Unmarshal into config struct
	if err := k.Unmarshal("", cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	// Set environment if not already set
	if cfg.Env == "" {
		if env := os.Getenv("ENV"); env != "" {
			cfg.Env = env
		} else if env := os.Getenv("ENVIRONMENT"); env != "" {
			cfg.Env = env
		} else {
			cfg.Env = "development"
		}
	}

	// Validate the loaded configuration
	if err := Validate(cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}
