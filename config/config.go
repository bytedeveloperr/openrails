package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	email "github.com/doujins-org/doujins-email"
	"github.com/joho/godotenv"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

const EnvProd string = "prod"
const EnvDev string = "dev"

const ConfigContextKey string = "config"

type Config struct {
	Env         string            `koanf:"env,omitempty"`
	Port        int16             `koanf:"port,omitempty"`
	Host        string            `koanf:"host,omitempty"`
	Mobius      *MobiusConfig     `koanf:"mobius,omitempty"`
	CCBill      *CCBillConfig     `koanf:"ccbill,omitempty"`
	Solana      *SolanaConfig     `koanf:"solana,omitempty"`
	DB          *DBConfig         `koanf:"db,omitempty"`
	Redis       *RedisConfig      `koanf:"redis,omitempty"`
	JWT         *JWTConfig        `koanf:"jwt,omitempty"`
	ClickHouse  *ClickHouseConfig `koanf:"clickhouse,omitempty"`
	Email       *email.Config     `koanf:"email,omitempty"`
	CorsOrigins []string          `koanf:"cors_origins,omitempty"`
	RateLimits  *RateLimitConfig  `koanf:"rate_limits,omitempty"`
	Admin       *AdminConfig      `koanf:"admin,omitempty"`
	TLS         *TLSConfig        `koanf:"tls,omitempty"`
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
	Secret   string `koanf:"secret"`
	Issuer   string `koanf:"issuer"`
	Audience string `koanf:"audience"`
	JWKSURL  string `koanf:"jwks_url"`
	// Optional RSA public key PEM for verifying RS256 JWTs. If empty and Issuer is set,
	// the service will attempt OIDC discovery at "{issuer}/.well-known/openid-configuration"
	// and use JWKS for verification.
	PublicKeyPEM         string `koanf:"public_key_pem"`
	SkipExpiryValidation bool   `koanf:"skip_expiry_validation"`
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

type ClickHouseConfig struct {
	ServerURL string `koanf:"server_url"` // HTTP URL for ClickHouse server (e.g., http://localhost:8123)
	Database  string `koanf:"database"`   // ClickHouse database name (e.g., analytics)
	Username  string `koanf:"username"`   // Optional username for authentication
	Password  string `koanf:"password"`   // Optional password for authentication
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
			// Default to host-accessible Postgres so local tests hit docker-compose via localhost
			URL:     "postgres://billing_app:billing_password@localhost:5432/doujins_db?sslmode=disable",
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
			Issuer:   "http://auth:8080",
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

	cfg := &Config{}

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
		"DATABASE_URL": "db.url",
		"REDIS_URL":    "redis.host",
		"ENV":          "env",
		"ENVIRONMENT":  "env",

		// JWT / OIDC
		"JWT_SECRET":        "jwt.secret",   // for HS256
		"JWT_ISSUER":        "jwt.issuer",
		"JWT_AUDIENCE":      "jwt.audience",
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

		// Email configuration
		"EMAIL_PROVIDER":        "email.provider",
		"EMAIL_FROM_ADDRESS":    "email.from_address",
		"EMAIL_FROM":            "email.from_address",
		"EMAIL_FROM_NAME":       "email.from_name",
		"EMAIL_DISABLED":        "email.disabled",
		"SENDGRID_API_KEY":      "email.sendgrid.api_key",
		"SENDGRID_API_HOST":     "email.sendgrid.api_host",
		"SENDGRID_SANDBOX_MODE": "email.sendgrid.sandbox_mode",
		"SENDGRID_FROM_EMAIL":   "email.from_address",
		"SENDGRID_FROM_NAME":    "email.from_name",

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

	if cfg.Solana == nil {
		cfg.Solana = &SolanaConfig{}
	}
	if cfg.Solana.Network == "" {
		cfg.Solana.Network = "mainnet"
	}
	if len(cfg.Solana.SupportedTokens) == 0 {
		cfg.Solana.SupportedTokens = TokensForNetwork(cfg.Solana.Network)
	}

	// Validate the loaded configuration
	if err := Validate(cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}
