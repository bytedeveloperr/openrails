package handlers

import (
    "net/http"
)

// GetSupportedTokens lists Solana tokens from configuration
func GetSupportedTokens(r *Request) {
    cfg := r.State.Config
    if cfg == nil || cfg.Solana == nil {
        r.ErrorJSON(http.StatusInternalServerError, "Solana configuration missing")
        return
    }

    tokens := make([]TokenInfo, 0, len(cfg.Solana.SupportedTokens))
    for _, t := range cfg.Solana.SupportedTokens {
        name := t.Name
        if name == "" {
            name = t.Symbol
        }
        tokens = append(tokens, TokenInfo{
            Symbol:   t.Symbol,
            Name:     name,
            Mint:     t.Mint,
            Decimals: t.Decimals,
        })
    }

    r.SuccessJSON(SupportedTokensResponse{Tokens: tokens})
}
