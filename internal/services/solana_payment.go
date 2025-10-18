package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/doujins-org/doujins-billing/config"
	"github.com/doujins-org/doujins-billing/internal/db"
	"github.com/doujins-org/doujins-billing/internal/db/models"
	"github.com/doujins-org/doujins-billing/internal/db/repo"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/uptrace/bun"
)

var (
	ErrInvalidToken  = errors.New("invalid or unsupported token")
	ErrPriceNotFound = errors.New("price not found")
)

type SolanaPaymentService struct {
	db                 *db.DB
	cfg                *config.Config
	priceService       *PriceService
	paymentSvc         *PaymentService
	productService     *ProductService
	entitlementService *EntitlementService
	notificationSvc    *NotificationService
}

func NewSolanaPaymentService(db *db.DB, cfg *config.Config, price *PriceService, payment *PaymentService, product *ProductService, entitlement *EntitlementService, notification *NotificationService) *SolanaPaymentService {
	return &SolanaPaymentService{
		db:                 db,
		cfg:                cfg,
		priceService:       price,
		paymentSvc:         payment,
		productService:     product,
		entitlementService: entitlement,
		notificationSvc:    notification,
	}
}

// SetNotificationService wires the notification service after SolanaPaymentService construction
func (s *SolanaPaymentService) SetNotificationService(notification *NotificationService) {
	s.notificationSvc = notification
}

// Generate returns payment calculation for client-side transaction building.
// userID: OIDC subject (string)
func (s *SolanaPaymentService) Generate(ctx context.Context, userID string, priceID uuid.UUID, tokenSymbol, userWallet string) (amount float64, currency string, tokenAmount uint64, expiresAt time.Time, err error) {
	price, err := s.priceService.GetByID(ctx, priceID)
	if err != nil {
		return 0, "", 0, time.Time{}, fmt.Errorf("%w: %v", ErrPriceNotFound, err)
	}
	if s.cfg.Solana == nil {
		return 0, "", 0, time.Time{}, fmt.Errorf("solana not configured")
	}
	tok, ok := s.cfg.Solana.SupportedTokens[tokenSymbol]
	if !ok || !tok.Enabled {
		return 0, "", 0, time.Time{}, ErrInvalidToken
	}
	mainnetMint := tok.MainnetMint
	if mainnetMint == "" {
		mainnetMint = tok.Mint
	}
	if mainnetMint == "" {
		return 0, "", 0, time.Time{}, fmt.Errorf("token %s missing mint configuration", tokenSymbol)
	}

	tokenUnits, _, err := calculateTokenQuote(ctx, tok, price.Amount)
	if err != nil {
		return 0, "", 0, time.Time{}, err
	}
	exp := time.Now().Add(10 * time.Minute)

	// Disallow one-off if a subscription entitlement is already active (indefinite)
	if userID != "" && s.entitlementService != nil {
		exists, err := s.entitlementService.HasActiveIndefinite(ctx, userID, "premium", time.Now())
		if err != nil {
			return 0, "", 0, time.Time{}, fmt.Errorf("failed entitlement check: %w", err)
		}
		if exists {
			return 0, "", 0, time.Time{}, fmt.Errorf("one-off purchase not allowed while subscription entitlement is active")
		}
	}

	return price.Amount, price.Currency, tokenUnits, exp, nil
}

// Submit records a confirmed payment for the given price and user, and grants associated entitlements.
// This is a pragmatic implementation that skips on-chain signature verification in this codebase.
// userID: OIDC subject (string)
func (s *SolanaPaymentService) Submit(ctx context.Context, userID string, intentID uuid.UUID, priceID uuid.UUID, signature string, userEmail *string) (*models.Payment, error) {
	var payment *models.Payment
	var subscription *models.Subscription
	var lifecycleNotifications []*models.NotificationQueue

	intentRef := intentID.String()
	processorSubscriptionID := &intentRef

	err := s.db.GetDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Create transactional services
		txDB := db.NewWithTx(tx)
		priceService := NewPriceService(txDB)
		paymentService := NewPaymentService(txDB)
		productService := NewProductService(txDB)
		entitlementService := NewEntitlementService(txDB)
		solanaTxnRepo := repo.NewSolanaTransactionRepo(txDB)
		notificationQueueService := NewNotificationQueueService(txDB)
		lifecycleService := NewSubscriptionLifecycleService(txDB, productService, priceService, entitlementService, notificationQueueService)
		lifecycleService.SetNotificationService(s.notificationSvc)

		// Get price information
		price, err := priceService.GetByID(ctx, priceID)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrPriceNotFound, err)
		}

		// Disallow one-off if a subscription entitlement is already active (indefinite)
		if userID != "" {
			exists, err := entitlementService.HasActiveIndefinite(ctx, userID, "premium", time.Now())
			if err != nil {
				return fmt.Errorf("failed entitlement check: %w", err)
			}
			if exists {
				return fmt.Errorf("one-off purchase not allowed while subscription entitlement is active")
			}
		}

		// Create or activate subscription membership through lifecycle service
		lifecycleParams := &CreateMembershipParams{
			UserID:                  userID,
			PriceID:                 price.ID,
			Processor:               models.ProcessorSolana,
			ProcessorSubscriptionID: processorSubscriptionID,
			UserEmail:               userEmail,
		}
		sub, notifications, err := lifecycleService.CreateMembershipTx(ctx, txDB, lifecycleParams)
		if err != nil {
			return fmt.Errorf("failed to create solana membership: %w", err)
		}
		subscription = sub
		lifecycleNotifications = notifications

		// Create canonical payment record
		now := time.Now()
		payment = &models.Payment{
			ID:      uuid.New(),
			UserID:  userID,
			PriceID: price.ID,
			SubscriptionID: func() *uuid.UUID {
				if subscription != nil {
					return &subscription.ID
				}
				return nil
			}(),
			Processor:     models.ProcessorSolana,
			TransactionID: signature,
			Amount:        price.Amount,
			Currency:      price.Currency,
			PurchasedAt:   now,
		}
		if err := paymentService.Create(ctx, payment); err != nil {
			return fmt.Errorf("failed to create payment: %w", err)
		}

		// Mark any pending SolanaTransaction for this user and price as confirmed (best-effort)
		if err := solanaTxnRepo.MarkConfirmedByUserAndAmount(ctx, userID, price.Amount, signature); err != nil {
			log.WithContext(ctx).WithError(err).WithFields(log.Fields{
				"user_id": userID,
				"amount":  price.Amount,
			}).Warn("failed to mark solana transactions as confirmed")
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	if subscription != nil && len(lifecycleNotifications) > 0 {
		lifecycleService := NewSubscriptionLifecycleService(s.db, s.productService, s.priceService, s.entitlementService, NewNotificationQueueService(s.db))
		lifecycleService.SetNotificationService(s.notificationSvc)
		lifecycleService.dispatchNotifications(ctx, lifecycleNotifications)
	}

	return payment, nil
}
