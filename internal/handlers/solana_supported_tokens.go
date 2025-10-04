package handlers

import (
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/services"
	log "github.com/sirupsen/logrus"
)

// GetSupportedTokens lists Solana tokens from configuration and enriches them with live prices.
func GetSupportedTokens(r *Request) {
	cfg := r.State.Config
	if cfg == nil || cfg.Solana == nil {
		r.ErrorJSON(http.StatusInternalServerError, "Solana configuration missing")
		return
	}

	tokenMap := cfg.Solana.SupportedTokens
	if len(tokenMap) == 0 {
		tokenMap = config.TokensForNetwork(cfg.Solana.Network)
	}

	mainnetMintSet := make(map[string]struct{})
	symbols := make([]string, 0, len(tokenMap))
	for symbol, t := range tokenMap {
		symbols = append(symbols, symbol)
		mint := t.MainnetMint
		if mint == "" {
			mint = t.Mint
		}
		if mint != "" {
			mainnetMintSet[mint] = struct{}{}
		}
	}

	mainnetMints := make([]string, 0, len(mainnetMintSet))
	for mint := range mainnetMintSet {
		mainnetMints = append(mainnetMints, mint)
	}

	ctx, cancel := context.WithTimeout(r.Request.Context(), 5*time.Second)
	defer cancel()

	prices, err := services.FetchJupiterPrices(ctx, mainnetMints)
	if err != nil {
		log.WithError(err).Warn("Failed to fetch Solana token prices from Jupiter")
		prices = map[string]float64{}
	}

	sort.Strings(symbols)
	tokens := make([]TokenInfo, 0, len(symbols))
	for _, symbol := range symbols {
		t := tokenMap[symbol]
		name := t.Name
		if name == "" {
			name = t.Symbol
		}
		mainnetMint := t.MainnetMint
		if mainnetMint == "" {
			mainnetMint = t.Mint
		}
		price := prices[mainnetMint]
		tokens = append(tokens, TokenInfo{
			Symbol:   t.Symbol,
			Name:     name,
			Mint:     t.Mint,
			Decimals: t.Decimals,
			Price:    price,
		})
	}

	r.SuccessJSON(SupportedTokensResponse{Tokens: tokens})
}
