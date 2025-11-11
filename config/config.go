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

type Config struct {
	Env           string            `koanf:"env,omitempty"`
	Port          FlexiblePort      `koanf:"port,omitempty"`
	Host          string            `koanf:"host,omitempty"`
	NMI           *NMIConfig        `koanf:"nmi,omitempty"`
	CCBill        *CCBillConfig     `koanf:"ccbill,omitempty"`
	Solana        *SolanaConfig     `koanf:"solana,omitempty"`
	DB            *DBConfig         `koanf:"db,omitempty"`
	Redis         *RedisConfig      `koanf:"redis,omitempty"`
	Auth          *AuthConfig       `koanf:"auth,omitempty"`
	ClickHouse    *ClickHouseConfig `koanf:"clickhouse,omitempty"`
	Logger        *LoggerConfig     `koanf:"logger,omitempty"`
	SendGrid      *SendGridConfig   `koanf:"sendgrid,omitempty"`
	CorsOrigins   []string          `koanf:"cors_origins,omitempty"`
	RateLimits    *RateLimitsConfig `koanf:"rate_limits,omitempty"`
	BillingAPIKey string            `koanf:"billing_api_key,omitempty"`
	Signing     *SigningConfig    `koanf:"signing,omitempty"`
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
	if c.SSLMode != "" {
		params = append(params, fmt.Sprintf("sslmode=%s", c.SSLMode))
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
	Addr     string `koanf:"addr"`
	Password string `koanf:"password"`
	DB       int    `koanf:"db"`
}

// AuthConfig holds JWT verification configuration for billing service.
// Billing is a JWT verifier (not issuer) - it validates tokens issued by doujins/hentai0.
type AuthConfig struct {
	Issuers          []string `koanf:"issuers"`           // List of expected token issuers (e.g., ["https://doujins.com", "https://hentai0.com"])
	ExpectedAudience string   `koanf:"expected_audience"` // Expected audience claim (e.g., "billing-app")
}

// SigningConfig controls the optional RSA signing used for JWT responses (e.g., /v1/access).
type SigningConfig struct {
	Enabled       bool   `koanf:"enabled"`
	PrivateKeyPEM string `koanf:"private_key_pem"`
	KeyID         string `koanf:"key_id"`
	Issuer        string `koanf:"issuer"`
	Audience      string `koanf:"audience"`
	TTL           string `koanf:"ttl"`
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
	Price       float64 `json:"price"`                     // Price in USD (fetched from Jupiter at runtime, not loaded from config)
	Enabled     bool    `json:"enabled" koanf:"enabled"`   // Whether this token is enabled
	MainnetMint string  `json:"mainnet_mint,omitempty" koanf:"mainnet_mint"`
}

// RateLimitsConfig is a map of endpoint identifier -> rate limit config
type RateLimitsConfig map[string]*RateLimit

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

// LoggerConfig holds logging configuration
type LoggerConfig struct {
	Level string `koanf:"level"` // debug | info | error
}

// RateLimit defines a rate limit policy
type RateLimit struct {
	Limit  int           `koanf:"limit"`
	Window time.Duration `koanf:"window"`
}

// Validate validates the billing configuration
func Validate(cfg *Config) error {
	// Skip strict validation in development environments
	isDev := cfg.Env == "development" || cfg.Env == "dev" || cfg.Env == ""
	prodEnv := !isDev && (strings.EqualFold(cfg.Env, EnvProd) || strings.EqualFold(cfg.Env, "production"))

	if !isDev {
		if err := validateNMI(cfg.NMI); err != nil {
			return fmt.Errorf("nmi config validation failed: %w", err)
		}

		// Validate CCBill configuration
		if err := validateCCBill(cfg.CCBill); err != nil {
			return fmt.Errorf("ccbill config validation failed: %w", err)
		}
	}

	if prodEnv {
		if cfg.CCBill != nil && cfg.CCBill.TestMode {
			return fmt.Errorf("ccbill.test_mode must be false in production")
		}
		if cfg.NMI != nil {
			if cfg.NMI.TestMode {
				return fmt.Errorf("nmi.test_mode must be false in production")
			}
			for name, provider := range cfg.NMI.Providers {
				if provider != nil && provider.TestMode != nil && *provider.TestMode {
					return fmt.Errorf("nmi provider %s test_mode must be false in production", name)
				}
			}
		}
	}

	// Always validate database configuration
	if err := validateDatabase(cfg.DB); err != nil {
		return fmt.Errorf("database config validation failed: %w", err)
	}

	if cfg.Signing != nil && cfg.Signing.Enabled {
		if err := validateSigning(cfg.Signing); err != nil {
			return err
		}
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

func validateSigning(cfg *SigningConfig) error {
	if cfg.KeyID == "" {
		return fmt.Errorf("signing key_id is required")
	}
	if cfg.PrivateKeyPEM == "" {
		return fmt.Errorf("signing private_key_pem is required")
	}
	if cfg.TTL != "" {
		if _, err := time.ParseDuration(cfg.TTL); err != nil {
			return fmt.Errorf("invalid signing ttl: %w", err)
		}
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
			// All apps use the admin superuser. Developers can override via DB_URL.
			URL:     "postgres://admin:admin_password@postgres:5432/doujins_db?sslmode=disable",
			Dialect: "postgres",
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
		// Match docker-compose ClickHouse (service: clickhouse)
		ClickHouse: &ClickHouseConfig{
			HTTPAddr:   "http://clickhouse:8123",
			ClientAddr: "clickhouse:9000",
			Database:   "analytics",
			Username:   "analytics_user",     // Match docker-compose CLICKHOUSE_USERNAME
			Password:   "analytics_password", // Match docker-compose CLICKHOUSE_PASSWORD
		},
		BillingAPIKey: "change-me-in-dev", // Override via env BILLING_API_KEY in prod
		Logger: &LoggerConfig{
			Level: "info", // Default to info level (options: debug, info, warn, error, fatal, panic)
		},
		RateLimits: &RateLimitsConfig{
			"subscribe": &RateLimit{
				Limit:  10, // Very restrictive for payment endpoints
				Window: time.Minute,
			},
			"webhook": &RateLimit{
				Limit:  100, // Higher for webhooks
				Window: time.Minute,
			},
			"payment": &RateLimit{
				Limit:  20,
				Window: time.Minute,
			},
			"default": &RateLimit{
				Limit:  60,
				Window: time.Minute,
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

		// Special case: BILLING_API_KEY stays as billing_api_key (no nesting)
		if s == "billing_api_key" {
			return "billing_api_key"
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
