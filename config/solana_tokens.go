package config

// Move-over of token helper logic from doujins-backend, adapted to billing's TokenConfig.

// SolanaToken is an alias to the billing TokenConfig for compatibility with moved helpers.
type SolanaToken = TokenConfig

// DefaultSupportedTokens returns the default list of supported SPL tokens for mainnet.
// SOL (native), USDC and PYUSD are supported for payments.
func DefaultSupportedTokens() map[string]SolanaToken {
	return map[string]SolanaToken{
		"SOL": {
			Symbol:   "SOL",
			Mint:     "So11111111111111111111111111111111111111112", // Native SOL (wrapped)
			Decimals: 9,
			Enabled:  true,
			Name:     "Solana",
		},
		"USDC": {
			Symbol:   "USDC",
			Mint:     "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v", // USDC Mainnet
			Decimals: 6,
			Enabled:  true,
			Name:     "USD Coin",
			Price:    1.0,
		},
		"PYUSD": {
			Symbol:   "PYUSD",
			Mint:     "2b1kV6DkPAnxd5ixfnxCpjxmKwqjjaYmCZfHsFu24GXo", // PYUSD Mainnet
			Decimals: 6,
			Enabled:  true,
			Name:     "PayPal USD",
			Price:    1.0,
		},
	}
}

// DefaultDevnetTokens returns default testnet token addresses for development.
func DefaultDevnetTokens() map[string]SolanaToken {
	return map[string]SolanaToken{
		"SOL": {
			Symbol:   "SOL",
			Mint:     "So11111111111111111111111111111111111111112", // Native SOL (same on devnet)
			Decimals: 9,
			Enabled:  true,
			Name:     "Solana",
		},
		"USDC": {
			Symbol:   "USDC",
			Mint:     "4zMMC9srt5Ri5X14GAgXhaHii3GnPAEERYPJgZJDncDU", // USDC Devnet
			Decimals: 6,
			Enabled:  true,
			Name:     "USD Coin (Devnet)",
			Price:    1.0,
		},
		"PYUSD": {
			Symbol:   "PYUSD",
			Mint:     "CXk2AMBfi3TwaEL2468s6zP8xq9NxTXjp9gjMgzeUynM", // PYUSD Devnet (example)
			Decimals: 6,
			Enabled:  true,
			Name:     "PayPal USD (Devnet)",
			Price:    1.0,
		},
	}
}

// GetTokenBySymbol returns a token configuration by its symbol from the defaults.
func GetTokenBySymbol(symbol string, useDevnet bool) (SolanaToken, bool) {
	var tokens map[string]SolanaToken
	if useDevnet {
		tokens = DefaultDevnetTokens()
	} else {
		tokens = DefaultSupportedTokens()
	}

	token, exists := tokens[symbol]
	return token, exists
}

// IsValidToken checks if a token symbol is supported (from defaults).
func IsValidToken(symbol string) bool {
	_, exists := DefaultSupportedTokens()[symbol]
	return exists
}
