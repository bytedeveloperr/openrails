package workers

import (
	"context"
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/integrations/ccbill"
	"github.com/doujins-org/doujins-billing/internal/integrations/mobius"
	"github.com/doujins-org/doujins-billing/internal/services"
	"github.com/google/uuid"
)

// Manager coordinates and manages all background workers
type Manager struct {
	db                  *db.DB
	mobiusClient        *mobius.MobiusClient
	subscriptionService *services.SubscriptionService

	// Worker control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Individual workers
	ccbillWorker      *CCBillDataLinkWorker
	mobiusWorker      *MobiusRebillWorker
	idempotencyWorker *IdempotencyCleanupWorker
}

// NewManager creates a new worker manager
func NewManager(db *db.DB, mobiusClient *mobius.MobiusClient, ccbillDataLink *ccbill.DataLinkClient, subscriptionService *services.SubscriptionService) *Manager {
	m := &Manager{
		db:                  db,
		mobiusClient:        mobiusClient,
		subscriptionService: subscriptionService,
		idempotencyWorker:   NewIdempotencyCleanupWorker(db),
	}

	if ccbillDataLink != nil {
		m.ccbillWorker = NewCCBillDataLinkWorker(db, ccbillDataLink)
	} else {
		log.Info("CCBill DataLink client not configured; worker disabled")
	}

	if mobiusClient != nil {
		m.mobiusWorker = NewMobiusRebillWorker(db, mobiusClient, subscriptionService)
	} else {
		log.Info("Mobius client not configured; rebill worker disabled")
	}

	return m
}

// Start starts all background workers
func (m *Manager) Start(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)

	log.Info("Starting billing service workers")

	// Start CCBill DataLink reconciliation worker if configured
	if m.ccbillWorker != nil {
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			if err := m.ccbillWorker.Start(m.ctx); err != nil {
				log.WithError(err).Error("CCBill DataLink worker failed")
			}
		}()
	}

	// Start Mobius rebill worker if configured
	if m.mobiusWorker != nil {
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			if err := m.mobiusWorker.Start(m.ctx); err != nil {
				log.WithError(err).Error("Mobius rebill worker failed")
			}
		}()
	}

	// Start idempotency cleanup worker
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		if err := m.idempotencyWorker.Start(m.ctx); err != nil {
			log.WithError(err).Error("Idempotency cleanup worker failed")
		}
	}()

	log.Info("All billing service workers started")
	return nil
}

// Stop stops all background workers
func (m *Manager) Stop(ctx context.Context) error {
	log.Info("Stopping billing service workers")

	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}

	// Wait for all workers to finish or timeout
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()
	m.ctx = nil

	select {
	case <-done:
		log.Info("All billing service workers stopped gracefully")
		return nil
	case <-ctx.Done():
		log.Warn("Timeout waiting for workers to stop")
		return fmt.Errorf("timeout stopping workers")
	}
}

// CCBillDataLinkWorker handles CCBill DataLink reconciliation
type CCBillDataLinkWorker struct {
	db             *db.DB
	dataLinkClient *ccbill.DataLinkClient
}

// NewCCBillDataLinkWorker creates a new CCBill DataLink worker
func NewCCBillDataLinkWorker(db *db.DB, ccbillDataLink *ccbill.DataLinkClient) *CCBillDataLinkWorker {
	return &CCBillDataLinkWorker{
		db:             db,
		dataLinkClient: ccbillDataLink,
	}
}

// Start starts the CCBill DataLink reconciliation worker
func (w *CCBillDataLinkWorker) Start(ctx context.Context) error {
	log.Info("Starting CCBill DataLink reconciliation worker")
	if w.dataLinkClient == nil {
		log.Info("CCBill DataLink client not available; worker will remain idle")
		<-ctx.Done()
		log.Info("CCBill DataLink worker stopping")
		return nil
	}

	ticker := time.NewTicker(6 * time.Hour) // Run every 6 hours
	defer ticker.Stop()

	// Run immediately on startup
	w.reconcile(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Info("CCBill DataLink worker stopping")
			return nil
		case <-ticker.C:
			w.reconcile(ctx)
		}
	}
}

// reconcile performs CCBill DataLink reconciliation
func (w *CCBillDataLinkWorker) reconcile(ctx context.Context) {
	log.Info("Running CCBill DataLink reconciliation")

	if w.dataLinkClient == nil {
		log.Warn("Skipping reconciliation; CCBill DataLink client missing")
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()

	records, err := w.dataLinkClient.FetchActiveMembers(ctx)
	if err != nil {
		log.WithError(err).Error("CCBill DataLink reconciliation failed")
		return
	}

	log.WithField("record_count", len(records)).Info("CCBill DataLink reconciliation fetched active members")

	// TODO: persist reconciliation results once downstream storage and event processing are implemented.

	log.Info("CCBill DataLink reconciliation completed")
}

// MobiusRebillWorker handles Mobius failed payment retries
type MobiusRebillWorker struct {
	db                  *db.DB
	mobiusClient        *mobius.MobiusClient
	subscriptionService *services.SubscriptionService
}

// NewMobiusRebillWorker creates a new Mobius rebill worker
func NewMobiusRebillWorker(db *db.DB, mobiusClient *mobius.MobiusClient, subscriptionService *services.SubscriptionService) *MobiusRebillWorker {
	return &MobiusRebillWorker{
		db:                  db,
		mobiusClient:        mobiusClient,
		subscriptionService: subscriptionService,
	}
}

// Start starts the Mobius rebill worker
func (w *MobiusRebillWorker) Start(ctx context.Context) error {
	log.Info("Starting Mobius rebill worker")

	ticker := time.NewTicker(1 * time.Hour) // Run every hour
	defer ticker.Stop()

	// Run immediately on startup
	w.processRetries(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Info("Mobius rebill worker stopping")
			return nil
		case <-ticker.C:
			w.processRetries(ctx)
		}
	}
}

// processRetries processes failed payment retries
func (w *MobiusRebillWorker) processRetries(ctx context.Context) {
	log.Info("Processing Mobius payment retries")

	if w.mobiusClient == nil {
		log.Debug("Mobius client not configured; skipping retries")
		return
	}

	// Find Mobius subscriptions in past_due with a retry due now or earlier
	due := []*models.Subscription{}
	if err := w.db.GetDB().NewSelect().
		Model(&due).
		Relation("Price").
		Relation("PaymentMethod").
		Where("processor = ?", models.ProcessorMobius).
		Where("status = ?", models.StatusPastDue).
		Where("next_retry_at IS NOT NULL AND next_retry_at <= NOW()").
		Scan(ctx); err != nil {
		log.WithError(err).Error("failed to query past_due subscriptions for Mobius retries")
		return
	}

	if len(due) == 0 {
		log.Info("No Mobius retries due")
		return
	}

	priceSvc := services.NewPriceService(w.db)
	productSvc := services.NewProductService(w.db)
	entitlementSvc := services.NewEntitlementService(w.db)
	notifQueueSvc := services.NewNotificationQueueService(w.db)
	lifecycle := services.NewSubscriptionLifecycleService(w.db, productSvc, priceSvc, entitlementSvc, notifQueueSvc)
	paymentSvc := services.NewPaymentService(w.db)

	for _, sub := range due {
		// Ensure we have a usable payment method
		pm := sub.PaymentMethod
		if pm == nil || !pm.IsActive || pm.VaultID == "" || pm.BillingID == nil || *pm.BillingID == "" {
			reason := "payment method unavailable for rebill"
			if err := lifecycle.FailMembership(ctx, &services.FailMembershipParams{
				Processor:               models.ProcessorMobius,
				ProcessorSubscriptionID: sub.ProcessorSubscriptionID,
				FailureReason:           &reason,
			}); err != nil {
				log.WithError(err).WithField("subscription_id", sub.ID).Warn("failed to mark membership failed (missing payment method)")
			}
			continue
		}

		// Attempt manual rebill via Mobius vault
		rebillResp, err := w.mobiusClient.AttemptManualRebill(mobius.ManualRebillParams{
			VaultID:        pm.VaultID,
			BillingID:      *pm.BillingID,
			SubscriptionID: sub.ProcessorSubscriptionID,
		})
		if err != nil {
			// Treat as failure and schedule next attempt
			msg := fmt.Sprintf("manual rebill request failed: %v", err)
			if err := lifecycle.FailMembership(ctx, &services.FailMembershipParams{
				Processor:               models.ProcessorMobius,
				ProcessorSubscriptionID: sub.ProcessorSubscriptionID,
				FailureReason:           &msg,
			}); err != nil {
				log.WithError(err).WithField("subscription_id", sub.ID).Warn("failed to record rebill failure")
			}
			continue
		}

		if rebillResp == nil || !rebillResp.Success {
			// Failed charge — increment attempts and schedule NextRetryAt per policy
			reason := "rebill declined"
			if rebillResp != nil && rebillResp.ErrorMessage != "" {
				reason = rebillResp.ErrorMessage
			}
			if err := lifecycle.FailMembership(ctx, &services.FailMembershipParams{
				Processor:               models.ProcessorMobius,
				ProcessorSubscriptionID: sub.ProcessorSubscriptionID,
				FailureReason:           &reason,
			}); err != nil {
				log.WithError(err).WithField("subscription_id", sub.ID).Warn("failed to apply failure policy after declined rebill")
			}
			continue
		}

		// Success: renew membership window and create a payment record
		if err := lifecycle.RenewMembership(ctx, &services.RenewMembershipParams{
			Processor:               models.ProcessorMobius,
			ProcessorSubscriptionID: sub.ProcessorSubscriptionID,
		}); err != nil {
			log.WithError(err).WithField("subscription_id", sub.ID).Error("failed to renew membership after successful rebill")
			// Do not create payment if renewal failed
			continue
		}

		// Create canonical payment record
		amount := 0.0
		currency := "USD"
		if sub.Price != nil {
			amount = sub.Price.Amount
			currency = sub.Price.Currency
		} else if p, err := priceSvc.GetByID(ctx, sub.PriceID); err == nil {
			amount = p.Amount
			currency = p.Currency
		}

		pay := &models.Payment{
			ID:             uuid.New(),
			UserID:         sub.UserID,
			PriceID:        sub.PriceID,
			SubscriptionID: &sub.ID,
			Processor:      models.ProcessorMobius,
			TransactionID:  rebillResp.TransactionID,
			Amount:         amount,
			Currency:       currency,
			PurchasedAt:    time.Now(),
			CreatedAt:      time.Now(),
		}
		if err := paymentSvc.Create(ctx, pay); err != nil {
			log.WithError(err).WithField("subscription_id", sub.ID).Warn("failed to create payment record for rebill")
		}
	}

	log.WithField("count", len(due)).Info("Mobius payment retries processed")
}

// IdempotencyCleanupWorker cleans up expired idempotency records
type IdempotencyCleanupWorker struct {
	db *db.DB
}

// NewIdempotencyCleanupWorker creates a new idempotency cleanup worker
func NewIdempotencyCleanupWorker(db *db.DB) *IdempotencyCleanupWorker {
	return &IdempotencyCleanupWorker{
		db: db,
	}
}

// Start starts the idempotency cleanup worker
func (w *IdempotencyCleanupWorker) Start(ctx context.Context) error {
	log.Info("Starting idempotency cleanup worker")

	ticker := time.NewTicker(1 * time.Hour) // Run every hour
	defer ticker.Stop()

	// Run immediately on startup
	w.cleanup(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Info("Idempotency cleanup worker stopping")
			return nil
		case <-ticker.C:
			w.cleanup(ctx)
		}
	}
}

// cleanup removes expired idempotency records
func (w *IdempotencyCleanupWorker) cleanup(ctx context.Context) {
	log.Info("Running idempotency cleanup")

	// Placeholder implementation
	// In production, this would:
	// 1. Delete idempotency_requests records older than configured retention period
	// 2. Keep successful records longer than failed ones
	// 3. Log cleanup statistics

	log.Info("Idempotency cleanup completed")
}
