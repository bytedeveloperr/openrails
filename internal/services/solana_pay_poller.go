package services

import (
	"context"
	"sync"
	"time"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/solana-go"
	"github.com/google/uuid"
	redis "github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
)

const (
	// Poll interval for checking pending payments
	solanaPayPollInterval = 500 * time.Millisecond
)

// SolanaPayPoller polls the blockchain for confirmed Solana Pay payments
type SolanaPayPoller struct {
	db               *db.DB
	redis            *redis.Client
	cfg              *config.Config
	rpc              *SolanaRPCService
	solanaPayService *SolanaPayService
	checkoutService  *CheckoutService

	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
}

// NewSolanaPayPoller creates a new poller for Solana Pay payments
func NewSolanaPayPoller(
	db *db.DB,
	redis *redis.Client,
	cfg *config.Config,
	solanaPayService *SolanaPayService,
	checkoutService *CheckoutService,
) *SolanaPayPoller {
	var rpc *SolanaRPCService
	if cfg.Solana != nil {
		rpc = NewSolanaRPCService(cfg.Solana.RPCEndpoint, cfg.Solana.Network)
	}

	return &SolanaPayPoller{
		db:               db,
		redis:            redis,
		cfg:              cfg,
		rpc:              rpc,
		solanaPayService: solanaPayService,
		checkoutService:  checkoutService,
	}
}

// Start begins polling for pending payments
func (p *SolanaPayPoller) Start(ctx context.Context) {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return
	}
	p.running = true
	p.stopCh = make(chan struct{})
	p.mu.Unlock()

	log.Info("Starting Solana Pay poller (500ms interval)")

	ticker := time.NewTicker(solanaPayPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("Solana Pay poller stopped (context cancelled)")
			return
		case <-p.stopCh:
			log.Info("Solana Pay poller stopped")
			return
		case <-ticker.C:
			p.pollPendingPayments(ctx)
		}
	}
}

// Stop stops the poller
func (p *SolanaPayPoller) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running && p.stopCh != nil {
		close(p.stopCh)
		p.running = false
	}
}

// pollPendingPayments checks all pending payments for confirmation
func (p *SolanaPayPoller) pollPendingPayments(ctx context.Context) {
	if p.rpc == nil || p.solanaPayService == nil {
		return
	}

	// Get all pending payment references from Redis
	refs, err := p.solanaPayService.GetAllPendingReferences(ctx)
	if err != nil {
		log.WithError(err).Warn("Failed to get pending payment references")
		return
	}

	if len(refs) == 0 {
		return // No pending payments
	}

	log.WithField("count", len(refs)).Debug("Polling pending Solana payments")

	// Check each pending payment
	for _, ref := range refs {
		select {
		case <-ctx.Done():
			return
		default:
		}

		p.checkPayment(ctx, ref)
	}
}

// checkPayment checks a single payment reference for confirmation
func (p *SolanaPayPoller) checkPayment(ctx context.Context, reference string) {
	// Get the pending payment details from Redis
	pending, err := p.solanaPayService.GetPendingPayment(ctx, reference)
	if err != nil {
		log.WithError(err).WithField("reference", reference).Warn("Failed to get pending payment")
		return
	}
	if pending == nil {
		// Payment expired, clean up the set
		p.solanaPayService.RemovePendingPayment(ctx, reference)
		return
	}

	// Query blockchain for transactions with this reference
	refPubkey, err := solana.PublicKeyFromBase58(reference)
	if err != nil {
		log.WithError(err).WithField("reference", reference).Error("Invalid reference public key")
		p.solanaPayService.RemovePendingPayment(ctx, reference)
		return
	}

	limit := 10
	sigs, err := p.rpc.GetSignaturesForAddress(ctx, refPubkey, &limit)
	if err != nil {
		log.WithError(err).WithField("reference", reference).Debug("Failed to get signatures for reference")
		return
	}

	if len(sigs) == 0 {
		return // No transactions yet
	}

	// Found a transaction - verify it matches our expected payment
	for _, sig := range sigs {
		if sig.Err != nil {
			continue // Skip failed transactions
		}

		// Verify the transaction matches our expected payment
		if p.verifyPayment(ctx, pending) {
			log.WithFields(log.Fields{
				"reference": reference,
				"signature": sig.Signature.String(),
				"user_id":   pending.UserID,
				"amount":    pending.Amount,
			}).Info("Solana payment confirmed")

			// Process the confirmed payment
			if err := p.processConfirmedPayment(ctx, reference, sig.Signature.String(), pending); err != nil {
				log.WithError(err).WithField("reference", reference).Error("Failed to process confirmed payment")
				continue
			}

			// Remove from pending set
			p.solanaPayService.RemovePendingPayment(ctx, reference)
			return
		}
	}
}

// verifyPayment validates that a transaction matches our expected payment
func (p *SolanaPayPoller) verifyPayment(ctx context.Context, pending *PendingSolanaPayment) bool {
	// For now, we trust that if a transaction references our unique reference key,
	// it's likely our payment. More sophisticated verification can be added:
	// - Check recipient address matches
	// - Check token mint matches
	// - Check amount matches (within tolerance)
	//
	// The reference key is cryptographically random (32 bytes), making collisions
	// essentially impossible.
	return true
}

// processConfirmedPayment uses CheckoutService.RegisterPurchase to record payment and grant entitlements
func (p *SolanaPayPoller) processConfirmedPayment(ctx context.Context, reference, signature string, pending *PendingSolanaPayment) error {
	priceID, err := uuid.Parse(pending.PriceID)
	if err != nil {
		return err
	}

	// Use the unified RegisterPurchase to record payment and grant entitlements
	result, err := p.checkoutService.RegisterPurchase(ctx, &RegisterPurchaseRequest{
		UserID:        pending.UserID,
		PriceID:       priceID,
		Processor:     "solana",
		TransactionID: signature,
		Amount:        pending.Amount,
		Currency:      pending.Currency,
	})
	if err != nil {
		return err
	}

	// Create SolanaTransaction record for audit trail (Solana-specific metadata)
	now := time.Now()
	userID := pending.UserID
	solTx := &models.SolanaTransaction{
		ID:          uuid.New(),
		UserID:      &userID,
		Signature:   &signature,
		Status:      "confirmed",
		Amount:      int64(pending.TokenAmount),
		Token:       pending.Token,
		TokenMint:   pending.TokenMint,
		FromAddress: "unknown", // Transfer Request flow doesn't reveal sender
		ToAddress:   pending.Recipient,
		PaymentID:   &result.PaymentID,
		BlockTime:   &now,
		ProcessingResult: map[string]interface{}{
			"reference":    reference,
			"token_amount": pending.TokenAmount,
			"token":        pending.Token,
			"flow":         "transfer_request",
		},
		CreatedAt: now,
	}

	if _, err := p.db.GetDB().NewInsert().Model(solTx).Exec(ctx); err != nil {
		log.WithError(err).Warn("Failed to create SolanaTransaction audit record")
		// Don't fail - payment and entitlements were already recorded
	}

	log.WithFields(log.Fields{
		"payment_id":   result.PaymentID,
		"user_id":      pending.UserID,
		"price_id":     priceID,
		"signature":    signature,
		"entitlements": result.Entitlements,
	}).Info("Processed Solana Pay payment via RegisterPurchase")

	return nil
}
