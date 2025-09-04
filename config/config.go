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

	// FlexForm integration settings
	BaseFlexFormURL string `koanf:"base_flexform_url"`
	IFrameWidth     string `koanf:"iframe_width"`  // Default: "100%"
	IFrameHeight    string `koanf:"iframe_height"` // Default: "600px"

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

type SendGridConfig struct {
	APIKey    string `koanf:"api_key"`
	FromEmail string `koanf:"from_email"`
	FromName  string `koanf:"from_name"`
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

	if cfg.TokenizationKey == "" {
		return fmt.Errorf("mobius tokenization key is required for frontend integration")
	}

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
        Port: 2052,
        DB: &DBConfig{
            // Match docker-compose Postgres (service: postgres)
            URL:     "postgres://supabase_admin:password@postgres:5432/supadb?sslmode=disable",
            Schema:  "public",
            Dialect: "postgres",
        },
        Redis: &RedisConfig{
            // Match docker-compose Garnet (service: garnet)
            Addr:     "garnet:6379",
            Password: "",
            DB:       0,
        },
        JWT: &JWTConfig{
            Secret: "dev-jwt-secret-change-in-production",
            Issuer: "doujins-billing",
        },
        // Match docker-compose ClickHouse (service: clickhouse)
        ClickHouse: &ClickHouseConfig{
            ServerURL: "http://clickhouse:8123",
            Database:  "analytics",
            Username:  "analytics_user",
            Password:  "analytics_password",
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
		"EXTERNAL_DATABASE_URL": "external_db.url", // Client app database
		"REDIS_URL":    "redis.host",
		"ENV":          "env",
		"ENVIRONMENT":  "env",

		// JWT
		"JWT_SECRET": "jwt.secret",
		"JWT_ISSUER": "jwt.issuer",

		// CCBill
		"CCBILL_CLIENT_ACCOUNT":    "ccbill.client_acc_num",
		"CCBILL_CLIENT_SUBACCOUNT": "ccbill.client_sub_acc",
		"CCBILL_SALT":              "ccbill.salt",
		"CCBILL_FORM_ID":           "ccbill.form_id",
		"CCBILL_FLEXFORM_ID":       "ccbill.form_id",

		// Mobius
		"MOBIUS_SECURITY_KEY":     "mobius.security_key",
		"MOBIUS_TOKENIZATION_KEY": "mobius.tokenization_key",
		"MOBIUS_WEBHOOK_SECRET":   "mobius.webhook_secret",

		// SendGrid
		"SENDGRID_API_KEY":    "sendgrid.api_key",
		"SENDGRID_FROM_EMAIL": "sendgrid.from_email",
		"SENDGRID_FROM_NAME":  "sendgrid.from_name",

		// ClickHouse
		"CLICKHOUSE_URL":      "clickhouse.server_url",
		"CLICKHOUSE_DATABASE": "clickhouse.database",
		"CLICKHOUSE_USERNAME": "clickhouse.username",
		"CLICKHOUSE_PASSWORD": "clickhouse.password",
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
