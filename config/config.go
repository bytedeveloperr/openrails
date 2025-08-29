package config

import (
	"fmt"
	"net/url"
)

// BillingConfig holds billing-specific configuration validation and utilities
type BillingConfig struct {
	Mobius *MobiusConfig `json:"mobius,omitempty"`
	CCBill *CCBillConfig `json:"ccbill,omitempty"`
	Solana *SolanaConfig `json:"solana,omitempty"`

	// Security settings
	RateLimits *RateLimitConfig `json:"rate_limits,omitempty"`
	Security   *SecurityConfig  `json:"security,omitempty"`

	// Network configuration
	Network *NetworkConfig `json:"network,omitempty"`
}

// MobiusConfig extends the main config with billing-specific validation
type MobiusConfig struct {

	// Additional billing-specific settings
	MaxRetries          int  `json:"max_retries,omitempty"`
	RetryBackoffSeconds int  `json:"retry_backoff_seconds,omitempty"`
	ManualRebillEnabled bool `json:"manual_rebill_enabled,omitempty"`
}

// CCBillConfig extends the main config with billing-specific validation
type CCBillConfig struct {
	*config.CCBillConfig

	// DataLink reconciliation settings
	DataLinkEnabled             bool `json:"datalink_enabled,omitempty"`
	DataLinkReconciliationHours int  `json:"datalink_reconciliation_hours,omitempty"`

	// Webhook settings
	WebhookIPWhitelist    []string `json:"webhook_ip_whitelist,omitempty"`
	WebhookTimeoutSeconds int      `json:"webhook_timeout_seconds,omitempty"`
}

// SolanaConfig holds Solana-specific billing configuration
type SolanaConfig struct {
	RPCEndpoint     string `json:"rpc_endpoint"`
	Network         string `json:"network"` // mainnet, devnet, testnet
	RecipientWallet string `json:"recipient_wallet"`

	// Supported tokens and their configurations
	SupportedTokens map[string]TokenConfig `json:"supported_tokens,omitempty"`

	// Transaction settings
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

// SecurityConfig defines security settings for billing
type SecurityConfig struct {
	// Webhook security
	RequireWebhookSignatures bool     `json:"require_webhook_signatures"`
	AllowedWebhookIPs        []string `json:"allowed_webhook_ips,omitempty"`

	// Anti-fraud settings
	MaxDailyTransactions       int     `json:"max_daily_transactions,omitempty"`
	MaxTransactionAmount       float64 `json:"max_transaction_amount,omitempty"`
	RequireAddressVerification bool    `json:"require_address_verification"`

	// Idempotency settings
	IdempotencyTTLHours     int `json:"idempotency_ttl_hours,omitempty"`
	IdempotencyCleanupHours int `json:"idempotency_cleanup_hours,omitempty"`
}

// NetworkConfig defines network-specific settings
type NetworkConfig struct {
	PublicPort     int      `json:"public_port"`
	PrivatePort    int      `json:"private_port"`
	AllowedOrigins []string `json:"allowed_origins,omitempty"`
	TrustedProxies []string `json:"trusted_proxies,omitempty"`

	// Private network restrictions
	PrivateNetworkCIDRs []string `json:"private_network_cidrs,omitempty"`
}

// Validate validates the billing configuration
func Validate(cfg *config.Config) error {
	isProd := cfg.Env == "production" || cfg.Env == "prod"

	// Validate Mobius configuration
	if err := validateMobius(cfg.Mobius, isProd); err != nil {
		return fmt.Errorf("mobius config validation failed: %w", err)
	}

	// Validate CCBill configuration
	if err := validateCCBill(cfg.CCBill, isProd); err != nil {
		return fmt.Errorf("ccbill config validation failed: %w", err)
	}

	// Validate Solana configuration if present
	if cfg.S3 != nil { // Using S3 as a proxy to check if Solana config might be present
		// We'll need to add SolanaConfig to the main config struct later
		// For now, validate basic requirements
	}

	// Validate rate limiting configuration
	if cfg.RateLimiter == nil {
		return fmt.Errorf("rate limiter configuration is required")
	}

	return nil
}

// validateMobius validates Mobius-specific configuration
func validateMobius(cfg *config.MobiusConfig, isProd bool) error {
	if cfg == nil {
		if isProd {
			return fmt.Errorf("mobius configuration is required in production")
		}
		return nil // Optional in development
	}

	// In production, security key is mandatory
	if isProd && cfg.SecurityKey == "" {
		return fmt.Errorf("mobius security key is required in production")
	}

	// Tokenization key is required for frontend integration
	if cfg.TokenizationKey == "" && isProd {
		return fmt.Errorf("mobius tokenization key is required for frontend integration")
	}

	// Validate webhook secret exists (recommended)
	if cfg.WebhookSecret == "" {
		return fmt.Errorf("mobius webhook secret is recommended for security")
	}

	return nil
}

// validateCCBill validates CCBill-specific configuration
func validateCCBill(cfg *config.CCBillConfig, isProd bool) error {
	if cfg == nil {
		if isProd {
			return fmt.Errorf("ccbill configuration is required in production")
		}
		return nil // Optional in development
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
func GetDefaultBillingConfig() *BillingConfig {
	return &BillingConfig{
		Network: &NetworkConfig{
			PublicPort:  2052,
			PrivatePort: 8060,
			PrivateNetworkCIDRs: []string{
				"10.0.0.0/8",     // Private class A
				"172.16.0.0/12",  // Private class B
				"192.168.0.0/16", // Private class C
				"127.0.0.0/8",    // Loopback
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
		Security: &SecurityConfig{
			RequireWebhookSignatures:   true,
			MaxDailyTransactions:       100,
			MaxTransactionAmount:       1000.0, // USD
			RequireAddressVerification: true,
			IdempotencyTTLHours:        24,
			IdempotencyCleanupHours:    168, // 1 week
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

// ExtendWithBillingDefaults extends the main config with billing defaults
func ExtendWithBillingDefaults(cfg *config.Config) *config.Config {
	billingDefaults := GetDefaultBillingConfig()

	// Apply rate limiting defaults if not set
	if cfg.RateLimiter == nil {
		cfg.RateLimiter = &config.RateLimiterConfig{
			Default: &config.RateLimitConfig{
				Limit:  billingDefaults.RateLimits.DefaultLimit.RequestsPerMinute,
				Window: 60, // 1 minute
			},
		}
	}

	return cfg
}
