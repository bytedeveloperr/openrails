package handlers

import (
	"net/http"
	"regexp"
)

var base58rx = regexp.MustCompile(`^[1-9A-HJ-NP-Za-km-z]{32,44}$`)

type SolanaWalletRequest struct {
	Wallet string `json:"wallet" validate:"required"`
}

// ConnectSolanaWallet adds a wallet to the user's in-memory wallet list (scaffold)
func ConnectSolanaWallet(r *Request) {
	req := new(SolanaWalletRequest)
	if err := r.Bind(req); err != nil || req.Wallet == "" || !base58rx.MatchString(req.Wallet) {
		r.ErrorJSON(http.StatusBadRequest, "Invalid wallet address")
		return
	}
	user := r.GetUser()
	r.State.SolanaWalletStore.Add(user.ID, req.Wallet)
	r.SuccessJSON(map[string]any{"connected": true, "wallet": req.Wallet})
}

// VerifySolanaWallet is a stub that accepts a signature and marks wallet as verified
func VerifySolanaWallet(r *Request) {
	// In a full implementation, validate a signature over a challenge.
	// Here, simply acknowledges for UX continuity.
	r.SuccessJSON(map[string]any{"verified": true})
}

// ListSolanaWallets lists the user's connected wallets
func ListSolanaWallets(r *Request) {
	user := r.GetUser()
	wallets := r.State.SolanaWalletStore.List(user.ID)
	r.SuccessJSON(map[string]any{"wallets": wallets})
}

// DeleteSolanaWallet removes a wallet (or all if none provided)
func DeleteSolanaWallet(r *Request) {
	user := r.GetUser()
	wallet := r.Query("wallet")
	if wallet == "" {
		r.State.SolanaWalletStore.Clear(user.ID)
		r.SuccessJSON(map[string]any{"deleted_all": true})
		return
	}
	r.State.SolanaWalletStore.Remove(user.ID, wallet)
	r.SuccessJSON(map[string]any{"deleted": true, "wallet": wallet})
}
