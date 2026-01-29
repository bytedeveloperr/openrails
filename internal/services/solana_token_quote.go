package services

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/open-rails/openrails/config"
	"github.com/open-rails/openrails/internal/integrations/fx"
	jupiter "github.com/open-rails/openrails/internal/integrations/jupiter"
)

// TokenQuote represents a complete quote for converting fiat to a Solana token.
// It includes all the information needed to audit and verify the quote.
type TokenQuote struct {
	// Units is the token amount in the smallest unit (e.g., lamports for SOL, 10^-6 for USDC).
	Units uint64
	// Decimal is the human-readable token amount (e.g., 9.99 USDC).
	Decimal float64
	// TokenPriceUSD is the token's price in USD at quote time (from Jupiter).
	TokenPriceUSD float64
	// FXRate is the conversion rate from the source currency to USD (1.0 for USD).
	FXRate float64
	// FXCurrency is the source currency code (e.g., "eur", "usd").
	FXCurrency string
	// AmountUSD is the price amount converted to USD (for auditing).
	AmountUSD float64
	// QuotedAt is when this quote was generated.
	QuotedAt time.Time
}

// CalculateTokenQuote converts a fiat amount (in cents) into token units/decimal amount based on live prices.
// It performs FX conversion if the currency is not USD.
//
// Parameters:
//   - ctx: context for cancellation
//   - tokenCfg: the token configuration (symbol, mint, decimals)
//   - amountCents: the price amount in the smallest currency unit (cents)
//   - currency: the currency code (e.g., "usd", "eur")
//   - fxProvider: the FX provider for currency conversion (can be nil for USD-only)
//
// Returns a TokenQuote with full audit information, or an error if quoting fails.
func CalculateTokenQuote(ctx context.Context, tokenCfg config.SolanaToken, amountCents int64, currency string, fxProvider fx.Provider) (*TokenQuote, error) {
	if amountCents <= 0 {
		return &TokenQuote{
			Units:      0,
			Decimal:    0,
			FXRate:     1.0,
			FXCurrency: strings.ToLower(currency),
			QuotedAt:   time.Now(),
		}, nil
	}

	currency = strings.ToLower(strings.TrimSpace(currency))
	if currency == "" {
		currency = "usd"
	}

	quotedAt := time.Now()

	// Step 1: Convert cents to the currency's base unit
	amountInCurrency := float64(amountCents) / 100.0

	// Step 2: Convert to USD if needed
	var amountUSD float64
	var fxRate float64

	if currency == "usd" {
		amountUSD = amountInCurrency
		fxRate = 1.0
	} else {
		// Need FX conversion
		if fxProvider == nil {
			return nil, fmt.Errorf("FX conversion required for currency %s but no FX provider configured", currency)
		}

		fxQuote, err := fxProvider.QuoteToUSD(ctx, currency)
		if err != nil {
			return nil, fmt.Errorf("failed to get FX rate for %s: %w", currency, err)
		}

		fxRate = fxQuote.Rate
		amountUSD = amountInCurrency * fxRate
	}

	// Step 3: Get token mint
	mint := tokenCfg.MainnetMint
	if mint == "" {
		mint = tokenCfg.Mint
	}
	if mint == "" {
		return nil, fmt.Errorf("token %s missing mint configuration", tokenCfg.Symbol)
	}

	// Step 4: Fetch token price in USD from Jupiter
	prices, err := jupiter.FetchJupiterPrices(ctx, []string{mint})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch token price: %w", err)
	}

	tokenPriceUSD, ok := prices[mint]
	if !ok || tokenPriceUSD <= 0 {
		return nil, fmt.Errorf("token price unavailable for %s", tokenCfg.Symbol)
	}

	// Step 4.5: Round stablecoin prices to $1.00 if within tolerance
	// This simplifies quotes and protects against minor depeg noise
	tokenPriceUSD = normalizeStablecoinPrice(tokenCfg.Symbol, tokenPriceUSD)

	// Step 5: Calculate token amount
	// Use ceiling to ensure we don't underpay due to rounding
	scale := math.Pow10(tokenCfg.Decimals)
	tokenAmountFloat := amountUSD / tokenPriceUSD
	tokenUnits := uint64(math.Ceil(tokenAmountFloat * scale))
	tokenDecimal := float64(tokenUnits) / scale

	return &TokenQuote{
		Units:         tokenUnits,
		Decimal:       tokenDecimal,
		TokenPriceUSD: tokenPriceUSD,
		FXRate:        fxRate,
		FXCurrency:    currency,
		AmountUSD:     amountUSD,
		QuotedAt:      quotedAt,
	}, nil
}

// stablecoinSymbols lists tokens that should be treated as USD-pegged stablecoins.
var stablecoinSymbols = map[string]bool{
	"USDC":  true,
	"USDT":  true,
	"PYUSD": true,
	"USDP":  true,
	"BUSD":  true,
	"DAI":   true,
}

// stablecoinTolerance is the maximum deviation from $1.00 before we use the actual price.
// If price is between $0.98 and $1.02, we round to $1.00.
const stablecoinTolerance = 0.02

// normalizeStablecoinPrice rounds stablecoin prices to $1.00 if within tolerance.
// This simplifies quotes and protects against minor depeg noise while still
// respecting significant depegs (e.g., if USDC dropped to $0.95).
func normalizeStablecoinPrice(symbol string, price float64) float64 {
	if !stablecoinSymbols[strings.ToUpper(symbol)] {
		return price
	}

	if price >= (1.0-stablecoinTolerance) && price <= (1.0+stablecoinTolerance) {
		return 1.0
	}

	// Outside tolerance - use actual price (significant depeg)
	return price
}
