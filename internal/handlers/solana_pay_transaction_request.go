package handlers

import (
	"net/http"
	"strings"

	authgin "github.com/PaulFidika/authkit/adapters/gin"
	"github.com/google/uuid"
)

type solanaPayTxnRequest struct {
	Account string `json:"account"`
}

// GetSolanaPayIntent returns metadata for a transaction-request intent.
func GetSolanaPayIntent(r *Request) {
	intentIDStr := strings.TrimSpace(r.GinCtx.Param("intent_id"))
	intentID, err := uuid.Parse(intentIDStr)
	if err != nil {
		r.ErrorJSON(http.StatusBadRequest, "invalid intent id")
		return
	}

	intent, err := r.State.SolanaPayService.GetIntentByID(r.Request.Context(), intentID)
	if err != nil {
		r.ErrorJSON(http.StatusNotFound, "intent not found")
		return
	}
	cl, ok := authgin.ClaimsFromGin(r.GinCtx)
	if !ok || cl.UserID == "" || cl.UserID != intent.UserID {
		r.ErrorJSON(http.StatusForbidden, "intent not authorized for user")
		return
	}
	if intent.ExpiresAt != nil && intent.ExpiresAt.Before(r.Clock.Now()) {
		r.ErrorJSON(http.StatusGone, "intent expired")
		return
	}

	r.SuccessJSON(intent)
}

// CreateSolanaPayTransaction builds an unsigned transaction for a given intent and user account.
func CreateSolanaPayTransaction(r *Request) {
	intentIDStr := strings.TrimSpace(r.GinCtx.Param("intent_id"))
	intentID, err := uuid.Parse(intentIDStr)
	if err != nil {
		r.ErrorJSON(http.StatusBadRequest, "invalid intent id")
		return
	}

	var req solanaPayTxnRequest
	if err := r.GinCtx.ShouldBindJSON(&req); err != nil {
		r.ErrorJSON(http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Account) == "" {
		r.ErrorJSON(http.StatusBadRequest, "account is required")
		return
	}

	cl, ok := authgin.ClaimsFromGin(r.GinCtx)
	if !ok || cl.UserID == "" {
		r.ErrorJSON(http.StatusForbidden, "unauthorized")
		return
	}

	txB64, intent, err := r.State.SolanaPayService.BuildTransactionRequest(r.Request.Context(), intentID, req.Account)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "expired") {
			r.ErrorJSON(http.StatusGone, msg)
			return
		}
		if strings.Contains(msg, "invalid") || strings.Contains(msg, "unsupported") {
			r.ErrorJSON(http.StatusBadRequest, msg)
			return
		}
		if strings.Contains(msg, "blockhash") || strings.Contains(msg, "rpc") {
			r.ErrorJSON(http.StatusServiceUnavailable, "solana rpc unavailable")
			return
		}
		r.ErrorJSON(http.StatusInternalServerError, "failed to build transaction")
		return
	}

	if intent.UserID != cl.UserID {
		r.ErrorJSON(http.StatusForbidden, "intent not authorized for user")
		return
	}

	resp := map[string]any{
		"transaction": txB64,
		"reference":   intent.Reference,
		"recipient":   intent.Recipient,
		"amount":      intent.Amount,
	}
	if intent.TokenMint != nil {
		resp["token_mint"] = *intent.TokenMint
	}
	if intent.Message != nil {
		resp["message"] = *intent.Message
	}
	if intent.ExpiresAt != nil {
		resp["expires_at"] = intent.ExpiresAt
	}

	r.SuccessJSON(resp)
}
