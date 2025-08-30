package config

import (
	"fmt"
	"net/url"
)

type Config struct {
	Mobius *MobiusConfig `json:"mobius,omitempty"`
	CCBill *CCBillConfig `json:"ccbill,omitempty"`
	Solana *SolanaConfig `json:"solana,omitempty"`
	DB     *DBConfig     `json:"db,omitempty"`

	RateLimits *RateLimitConfig `json:"rate_limits,omitempty"`
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

type SolanaConfig struct {
	RPCEndpoint       string `json:"rpc_endpoint"`
	Network           string `json:"network"` // mainnet, devnet, testnet
	DestinationWallet string `json:"destination_wallet"`

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

// RateLimit defines a rate limit policy
type RateLimit struct {
	RequestsPerMinute int `json:"requests_per_minute"`
	BurstSize         int `json:"burst_size"`
}

// Validate validates the billing configuration
func Validate(cfg *Config) error {
	if err := validateMobius(cfg.Mobius); err != nil {
		return fmt.Errorf("mobius config validation failed: %w", err)
	}

	// Validate CCBill configuration
	if err := validateCCBill(cfg.CCBill); err != nil {
		return fmt.Errorf("ccbill config validation failed: %w", err)
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

// GetDefaultBillingConfig returns a billing configuration with sensible defaults
func GetDefaultBillingConfig() *Config {
	return &Config{
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
