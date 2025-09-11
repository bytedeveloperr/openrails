package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/internal/services"
	"github.com/doujins-org/doujins-billing/internal/utils/solana"
)

type SolanaWalletRequest struct {
	Wallet string `json:"wallet" validate:"required"`
}

type SolanaWalletVerifyRequest struct {
	Wallet    string `json:"wallet" validate:"required"`
	Signature string `json:"signature" validate:"required"`
	Message   string `json:"message" validate:"required"`
}

type SolanaWalletChallengeRequest struct {
	Wallet string `json:"wallet" validate:"required"`
}

// ConnectSolanaWallet adds a wallet to the user's database wallet list
func ConnectSolanaWallet(r *Request) {
	req := new(SolanaWalletRequest)
	if err := r.Bind(req); err != nil {
		r.ErrorJSON(http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := solana.ValidateAddress(req.Wallet); err != nil {
		r.ErrorJSON(http.StatusBadRequest, "Invalid wallet address")
		return
	}

	user := r.GetUser()
	ctx, cancel := context.WithTimeout(r.Request.Context(), 5*time.Second)
	defer cancel()

	wallet, err := r.State.SolanaWalletService.Link(ctx, user.ID, req.Wallet)
	if err != nil {
		log.WithError(err).Error("Failed to link Solana wallet")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to connect wallet")
		return
	}

	r.SuccessJSON(map[string]any{
		"connected":    true,
		"wallet":       wallet.Address,
		"is_verified":  wallet.IsVerified,
		"connected_at": wallet.CreatedAt,
	})
}

// GenerateSolanaWalletChallenge generates a verification challenge for a wallet
func GenerateSolanaWalletChallenge(r *Request) {
	req := new(SolanaWalletChallengeRequest)
	if err := r.Bind(req); err != nil {
		r.ErrorJSON(http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := solana.ValidateAddress(req.Wallet); err != nil {
		r.ErrorJSON(http.StatusBadRequest, "Invalid wallet address")
		return
	}

	user := r.GetUser()
	ctx, cancel := context.WithTimeout(r.Request.Context(), 5*time.Second)
	defer cancel()

	// Get verification service
	var rpcEndpoint, network string
	if r.State.Config.Solana != nil {
		rpcEndpoint = r.State.Config.Solana.RPCEndpoint
		network = r.State.Config.Solana.Network
	} else {
		network = "devnet" // fallback
	}
	rpcService := services.NewSolanaRPCService(rpcEndpoint, network)
	verificationService := services.NewSolanaVerificationService(r.State.DB, rpcService)

	challenge, err := verificationService.GenerateChallenge(ctx, user.ID, req.Wallet)
	if err != nil {
		log.WithError(err).Error("Failed to generate wallet verification challenge")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to generate challenge")
		return
	}

	r.SuccessJSON(map[string]any{
		"challenge": challenge.Message,
		"expires_at": challenge.ExpiresAt.Unix(),
		"wallet": req.Wallet,
	})
}

// VerifySolanaWallet accepts a signature and marks wallet as verified
func VerifySolanaWallet(r *Request) {
	req := new(SolanaWalletVerifyRequest)
	if err := r.Bind(req); err != nil {
		r.ErrorJSON(http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := solana.ValidateAddress(req.Wallet); err != nil {
		r.ErrorJSON(http.StatusBadRequest, "Invalid wallet address")
		return
	}

	user := r.GetUser()
	ctx, cancel := context.WithTimeout(r.Request.Context(), 10*time.Second)
	defer cancel()

	// Get verification service with config
	var rpcEndpoint, network string
	if r.State.Config.Solana != nil {
		rpcEndpoint = r.State.Config.Solana.RPCEndpoint
		network = r.State.Config.Solana.Network
	} else {
		network = "devnet" // fallback
	}
	rpcService := services.NewSolanaRPCService(rpcEndpoint, network)
	verificationService := services.NewSolanaVerificationService(r.State.DB, rpcService)

	// Verify the signature against the challenge
	err := verificationService.VerifySignature(ctx, user.ID, req.Wallet, req.Signature, req.Message)
	if err != nil {
		log.WithError(err).Error("Failed to verify wallet signature")
		r.ErrorJSON(http.StatusBadRequest, fmt.Sprintf("Signature verification failed: %v", err))
		return
	}

	r.SuccessJSON(map[string]any{
		"verified": true,
		"wallet":   req.Wallet,
	})
}

// ListSolanaWallets lists the user's connected wallets from database
func ListSolanaWallets(r *Request) {
	user := r.GetUser()
	ctx, cancel := context.WithTimeout(r.Request.Context(), 5*time.Second)
	defer cancel()

	wallets, err := r.State.SolanaWalletService.List(ctx, user.ID)
	if err != nil {
		log.WithError(err).Error("Failed to list Solana wallets")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to list wallets")
		return
	}

	r.SuccessJSON(map[string]any{"wallets": wallets})
}

// DeleteSolanaWallet removes a wallet from database
func DeleteSolanaWallet(r *Request) {
	user := r.GetUser()
	wallet := r.Query("wallet")
	
	if wallet == "" {
		r.ErrorJSON(http.StatusBadRequest, "wallet parameter required")
		return
	}

	if err := solana.ValidateAddress(wallet); err != nil {
		r.ErrorJSON(http.StatusBadRequest, "Invalid wallet address")
		return
	}

	ctx, cancel := context.WithTimeout(r.Request.Context(), 5*time.Second)
	defer cancel()

	if err := r.State.SolanaWalletService.Delete(ctx, user.ID, wallet); err != nil {
		log.WithError(err).Error("Failed to delete Solana wallet")
		r.ErrorJSON(http.StatusInternalServerError, "Failed to delete wallet")
		return
	}

	r.SuccessJSON(map[string]any{
		"deleted": true,
		"wallet":  wallet,
	})
}
