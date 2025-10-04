package config

import "strings"

type SolanaToken = TokenConfig

func DefaultSupportedTokens() map[string]SolanaToken {
	return map[string]SolanaToken{
		"SOL": {
			Decimals:    9,
			Enabled:     true,
			Symbol:      "SOL",
			Name:        "Solana",
			Mint:        "So11111111111111111111111111111111111111112",
			MainnetMint: "So11111111111111111111111111111111111111112",
		},
		"USDC": {
			Decimals:    6,
			Enabled:     true,
			Symbol:      "USDC",
			Name:        "USD Coin",
			Mint:        "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
			MainnetMint: "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
		},
		"PYUSD": {
			Decimals:    6,
			Enabled:     true,
			Symbol:      "PYUSD",
			Name:        "PayPal USD",
			Mint:        "2b1kV6DkPAnxd5ixfnxCpjxmKwqjjaYmCZfHsFu24GXo",
			MainnetMint: "2b1kV6DkPAnxd5ixfnxCpjxmKwqjjaYmCZfHsFu24GXo",
		},
	}
}

func DefaultDevnetTokens() map[string]SolanaToken {
	return map[string]SolanaToken{
		"SOL": {
			Decimals:    9,
			Enabled:     true,
			Symbol:      "SOL",
			Name:        "Solana",
			Mint:        "So11111111111111111111111111111111111111112",
			MainnetMint: "So11111111111111111111111111111111111111112",
		},
		"USDC": {
			Decimals:    6,
			Enabled:     true,
			Symbol:      "USDC",
			Name:        "USD Coin (Devnet)",
			Mint:        "4zMMC9srt5Ri5X14GAgXhaHii3GnPAEERYPJgZJDncDU",
			MainnetMint: "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
		},
		"PYUSD": {
			Decimals:    6,
			Enabled:     true,
			Symbol:      "PYUSD",
			Name:        "PayPal USD (Devnet)",
			Mint:        "CXk2AMBfi3TwaEL2468s6zP8xq9NxTXjp9gjMgzeUynM",
			MainnetMint: "2b1kV6DkPAnxd5ixfnxCpjxmKwqjjaYmCZfHsFu24GXo",
		},
	}
}

func GetTokenBySymbol(symbol string, useDevnet bool) (SolanaToken, bool) {
	var network string
	if useDevnet {
		network = "devnet"
	} else {
		network = "mainnet"
	}
	tokens := TokensForNetwork(network)
	token, exists := tokens[symbol]
	return token, exists
}

func IsValidToken(symbol string) bool {
	_, exists := DefaultSupportedTokens()[symbol]
	return exists
}

func TokensForNetwork(network string) map[string]SolanaToken {
	switch strings.ToLower(network) {
	case "devnet":
		return DefaultDevnetTokens()
	default:
		return DefaultSupportedTokens()
	}
}
